package svg

import (
	"fmt"
	"io"
	"strings"
	textTemplate "text/template"
	"time"
)

func NewCanvas(width, height int) *Canvas {
	return &Canvas{
		Width:           width,
		Height:          height,
		BackgroundColor: "white",
		StrokeColor:     "rgba(0,0,0,0.33)",
		StrokeWidth:     1.5,
		StrokeLineCap:   "round",
		GridWidth:       width - 2,
		GridHeight:      height - 2,
	}
}

type Canvas struct {
	Title           string
	Width           int
	Height          int
	BackgroundColor string
	StrokeColor     string
	StrokeWidth     float64
	StrokeLineCap   string
	GridWidth       int
	GridHeight      int
	GridYMin        float64
	GridYMax        float64
	GridMaxCount    int
	Times           []time.Time
	Values          []float64
}

func (s Canvas) Export(w io.Writer, times []time.Time, values []float64) error {
	c := s
	c.Times = times
	c.Values = values
	return svgTmpl.ExecuteTemplate(w, "svg", c)
}

func svgPath(times []time.Time, values []float64, maxCount int, width int, height int, gridYMin float64, gridYMax float64) string {
	if len(times) == 0 || len(values) == 0 {
		return ""
	}
	if len(times) != len(values) {
		return ""
	}

	min, max := gridYMin, gridYMax
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	sb := &strings.Builder{}
	xOffset := 0.0
	xWidth := float64(width)
	if maxCount > 0 && len(values) < maxCount {
		xOffset = (float64(width) / float64(maxCount)) * float64(maxCount-len(values))
		xWidth = xWidth - xOffset
	}
	for i, v := range values {
		x := float64(i+1) / float64(len(values))
		y := (v - min) / (max - min)
		if max == min {
			y = 0
		}
		if i == 0 || times[i].IsZero() {
			sb.WriteString(fmt.Sprintf("M%f %f", xOffset+x*xWidth, (1-y)*float64(height+1)))
		}
		sb.WriteString(fmt.Sprintf(" L%f %f", xOffset+x*xWidth, (1-y)*float64(height+1)))
	}
	return sb.String()
}

var svgTmpl = textTemplate.Must(textTemplate.New("svg").
	Funcs(textTemplate.FuncMap{
		"svgPath": svgPath,
	}).
	Parse(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN"
"http://www.w3c.org/Graphics/SVG/1.1/DTD/svg1.1dtd">
<svg width="{{ .Width }}" height="{{ .Height }}" xmlns="http://www.w3.org/2000/svg">
  <rect x="0" y="0" width="{{ .Width }}" height="{{ .Height }}" rx="5" ry="5" fill="{{ .BackgroundColor }}"/>
  <text x="100" y="2" text-anchor="middle" alignment-baseline="hanging" font-size="0.8em" stroke-width="1px" font-family="'Courier New',monospace" stroke="{{ .StrokeColor }}">{{ .Title }}</text>
  <path d="{{svgPath .Times .Values .GridMaxCount .GridWidth .GridHeight .GridYMin .GridYMax }}" fill="none"
		stroke="{{ .StrokeColor }}" stroke-width="{{ .StrokeWidth }}" stroke-linecap="{{ .StrokeLineCap }}"/>
</svg>`))
