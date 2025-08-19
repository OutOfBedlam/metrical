package svg

import (
	"fmt"
	"io"
	"slices"
	"strings"
	textTemplate "text/template"
	"time"

	"github.com/OutOfBedlam/metric"
)

func NewCanvas(width, height int) *Canvas {
	return &Canvas{
		Width:           width,
		Height:          height,
		BackgroundColor: "white",
		StrokeColor:     "rgba(0,0,0,0.33)",
		StrokeWidth:     1.5,
		StrokeLineCap:   "round",
	}
}

type Canvas struct {
	XMLHeader       bool
	Title           string
	Value           string
	Width           int
	Height          int
	BackgroundColor string
	StrokeColor     string
	StrokeWidth     float64
	StrokeLineCap   string
	GridYMin        float64
	GridYMax        float64
	GridYMargin     float64
	GridXInterval   time.Duration
	GridXMaxCount   int
	Times           []time.Time
	Values          []float64
	MinValues       []float64
	MaxValues       []float64
	ShowXAxisLabels bool
	ShowYAxisLabels bool
	ValueUnit       metric.Unit

	gridOrgX     float64
	gridOrgY     float64
	gridWidth    float64
	gridHeight   float64
	gridOrgTime  time.Time
	timePerPoint time.Duration
}

func (c Canvas) Export(w io.Writer) error {
	if len(c.Times) == 0 || len(c.Values) == 0 {
		return fmt.Errorf("no data to export")
	}

	lastTime := c.Times[len(c.Times)-1]
	if c.GridXInterval > 0 && c.GridXMaxCount > 0 {
		c.gridOrgTime = lastTime.Add(-c.GridXInterval * time.Duration(c.GridXMaxCount-1))
	} else {
		c.gridOrgTime = c.Times[0]
	}

	leftMargin := float64(c.Width) * 0.04 // 4% for left margin
	if c.ShowYAxisLabels {
		leftMargin = float64(c.Width) * 0.14 // more left margin for y-axis labels
	}
	rightMargin := float64(c.Width) * 0.03  // 3% for right margin
	topMargin := float64(12)                // 12px for top margin
	bottomMargin := float64(c.Height) * 0.1 // 10% for bottom margin
	if c.ShowXAxisLabels {
		bottomMargin = float64(c.Height) * 0.18 // 18% for bottom margin with x-axis labels
	}

	c.gridOrgX = leftMargin
	c.gridOrgY = float64(c.Height) - bottomMargin
	c.gridWidth = float64(c.Width) - (leftMargin + rightMargin)
	c.gridHeight = c.gridOrgY - topMargin
	c.timePerPoint = lastTime.Sub(c.gridOrgTime) / time.Duration(int(c.gridWidth))

	if c.timePerPoint <= 0 {
		return fmt.Errorf("invalid time per point: %v", c.timePerPoint)
	}

	min, max := c.GridYMin, c.GridYMax
	for i, v := range c.Values {
		if i == 0 && c.GridYMin == c.GridYMax {
			min = v
			max = v
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	for _, v := range c.MinValues {
		if v < min {
			min = v
		}
	}

	for _, v := range c.MaxValues {
		if v > max {
			max = v
		}
	}

	c.GridYMin = min - c.GridYMargin*(max-min)
	c.GridYMax = max + c.GridYMargin*(max-min)

	return svgTmpl.ExecuteTemplate(w, "svg", c)
}

func (c Canvas) x(t time.Time) float64 {
	var x float64 = 0
	if c.timePerPoint > 0 {
		x = float64(t.Sub(c.gridOrgTime)) / float64(c.timePerPoint)
	}
	return c.gridOrgX + x
}

func (c Canvas) y(v float64) float64 {
	var y float64 = 0
	if c.GridYMax > c.GridYMin {
		y = ((v - c.GridYMin) / (c.GridYMax - c.GridYMin)) * c.gridHeight
	}
	return c.gridOrgY - y
}

func (c Canvas) xy(t time.Time, v float64) (float64, float64) {
	return c.x(t), c.y(v)
}

type SvgAxis struct {
	Path       string
	TickLabels []string
}

// svgAxes generates the SVG path for the axes and ticks.
// It returns axes paths and tick labels as a string.
func svgAxes(c Canvas) SvgAxis {
	sb := &strings.Builder{}
	// x axis
	sb.WriteString(fmt.Sprintf("M%.f %.f h %.f", c.gridOrgX, c.gridOrgY, c.gridWidth))
	// y axis
	sb.WriteString(fmt.Sprintf("M%.f %.f v %.f", c.gridOrgX, c.gridOrgY, -c.gridHeight))

	// x ticks
	tickLabels := []string{}
	xLimit := c.gridOrgTime.Add(c.GridXInterval * time.Duration(c.GridXMaxCount))
	for xt := c.gridOrgTime; xt.Before(xLimit); xt = xt.Add(c.GridXInterval) {
		x := c.x(xt)
		n := 2
		if (c.timePerPoint < 30*time.Second && xt.Second() == 0) ||
			(c.timePerPoint < 5*time.Minute && xt.Minute() == 0 && xt.Second() == 0) ||
			(c.timePerPoint < 30*time.Minute && xt.Hour() == 0 && xt.Minute() == 0 && xt.Second() == 0) ||
			(c.timePerPoint < 24*time.Hour && xt.Hour()%3 == 0 && xt.Minute() == 0 && xt.Second() == 0) {
			n = 4
			label := xt.Format("15:04")
			if c.ShowXAxisLabels {
				tickLabels = append(tickLabels,
					fmt.Sprintf(`<text x="%.f" y="%.f" `+
						`text-anchor="middle" `+
						`alignment-baseline="hanging" `+
						`font-size="0.5em" `+
						`stroke-width="1px" `+
						`stroke="{{ .StrokeColor }}">%s</text>`,
						x, c.gridOrgY+4, label))
			}
		}
		sb.WriteString(fmt.Sprintf(" M%.f %.f v %d", x, c.gridOrgY, n))
	}
	// y ticks
	for i := 0; i <= 10; i++ {
		v := (c.GridYMax-c.GridYMin)*float64(i)/10 + c.GridYMin
		y := c.y(v)
		n := 2
		if i == 0 || i == 5 || i == 10 {
			alignmentBaseline := "middle"
			if i == 0 {
				alignmentBaseline = "bottom"
			}
			if c.ShowYAxisLabels {
				n = 4
				tickLabels = append(tickLabels,
					fmt.Sprintf(`<text x="%.f" y="%.f" `+
						`text-anchor="end" `+
						`alignment-baseline="%s" `+
						`font-size="0.5em" `+
						`stroke-width="1px" `+
						`stroke="{{ .StrokeColor }}">%s</text>`,
						c.gridOrgX-5, y,
						alignmentBaseline,
						c.ValueUnit.Format(v, 0)))
			}
		}
		sb.WriteString(fmt.Sprintf(" M%.f %.f h -%d", c.gridOrgX, y, n))
	}
	return SvgAxis{Path: sb.String(), TickLabels: tickLabels}
}

func svgPath(c Canvas) string {
	sb := &strings.Builder{}
	for i, v := range c.Values {
		x, y := c.xy(c.Times[i], v)
		if i == 0 || c.Times[i].IsZero() {
			sb.WriteString(fmt.Sprintf("M%f %f", x, y))
		} else {
			sb.WriteString(fmt.Sprintf(" L%f %f", x, y))
		}
	}
	return sb.String()
}

func svgRange(c Canvas) string {
	var times []time.Time
	var values []float64

	times = append(times, c.Times...)
	slices.Reverse(times)
	times = append(times, c.Times...)

	values = append(values, c.MinValues...)
	slices.Reverse(values)
	values = append(values, c.MaxValues...)

	sb := &strings.Builder{}
	for i, v := range values {
		x, y := c.xy(times[i], v)
		if i == 0 || times[i].IsZero() {
			sb.WriteString(fmt.Sprintf("M%f %f", x, y))
		} else {
			sb.WriteString(fmt.Sprintf(" L%f %f", x, y))
		}
	}
	return sb.String()
}

var svgTmpl = textTemplate.Must(textTemplate.New("svg").
	Funcs(textTemplate.FuncMap{
		"svgPath":  svgPath,
		"svgRange": svgRange,
		"svgAxes":  svgAxes,
	}).
	Parse(`{{- if .XMLHeader -}}
<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3c.org/Graphics/SVG/1.1/DTD/svg1.1dtd">
{{ end -}}
<svg width="{{ .Width }}" height="{{ .Height }}" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <style type="text/css">
	  text {
	    font-family: Courier New,monospace;
	  }
	</style>
  </defs>
  <rect x="0" y="0" width="{{ .Width }}" height="{{ .Height }}" rx="5" ry="5" fill="{{ .BackgroundColor }}"/>
  <text x="100" y="2" text-anchor="middle" alignment-baseline="hanging" font-size="0.6em" stroke-width="1px" font-family="'Courier New',monospace" stroke="{{ .StrokeColor }}">{{ .Title }}</text>
  <text x="196" y="2" text-anchor="end" alignment-baseline="hanging" font-size="0.6em" stroke-width="1px" font-family="'Courier New',monospace" stroke="{{ .StrokeColor }}">{{ .Value }}</text>
  {{ $axes := svgAxes . }}
  <path d="{{ $axes.Path }}" stroke="rgba(0,0,0,0.50)" stroke-width="1.0" fill="none"/>
  {{ range $axes.TickLabels }}
    {{ . }}
  {{ end }}
  <path d="{{ svgRange . }} Z" fill="rgba(0,0,0,0.15)" stroke="none" stroke-width="0.9"/>
  <path d="{{ svgPath . }}" fill="none" stroke="{{ .StrokeColor }}" stroke-width="{{ .StrokeWidth }}" stroke-linecap="{{ .StrokeLineCap }}"/>
</svg>`))
