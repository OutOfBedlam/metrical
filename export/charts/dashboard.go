package charts

import (
	_ "embed"
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/OutOfBedlam/metric"
)

func HandleDashboard(nameProvider func() []string, series []string) func(w http.ResponseWriter, r *http.Request) {
	data := Dashboard{
		nameProvider: nameProvider,
		Series:       series,
		Options: Options{
			JsSrc: []string{
				"https://cdn.jsdelivr.net/npm/echarts@6.0.0/dist/echarts.min.js",
			},
			Style: []CSSStyle{
				{
					Selector: "body",
					Styles: map[string]string{
						"background": "rgb(38,40,49)",
					},
				},
				{
					Selector: ".container",
					Styles: map[string]string{
						"display":         "flex",       // Enables Flexbox
						"flex-wrap":       "wrap",       // Allows wrapping to the next line
						"gap":             "10px",       // Adds spacing between items
						"justify-content": "flex-start", // Aligns items to the left
						// "justify-content": "space-between" // Distributes items evenly with space between
					},
				},
				{
					Selector: ".item",
					Styles: map[string]string{
						"flex":          "0 0 400px", // Each item takes up 400px width
						"height":        "300px",     // Fixed height for each item
						"border-radius": "4px",
						"padding":       "0px",
						"box-shadow":    "2px 2px 5px rgba(0,0,0,0.1)",
					},
				},
				{
					Selector: ".series-tabs",
					Styles: map[string]string{
						"display":       "flex",
						"gap":           "4px",
						"margin-bottom": "0.2em",
					},
				},
				{
					Selector: ".series-tabs .tab",
					Styles: map[string]string{
						"padding":         "6px 16px",
						"border":          "1px solid #888",
						"border-radius":   "6px 6px 0 0",
						"background":      "#222",
						"color":           "#eee",
						"text-decoration": "none",
						"cursor":          "pointer",
						"transition":      "background 0.2s",
					},
				},
				{
					Selector: ".series-tabs .tab.active",
					Styles: map[string]string{
						"background":    "#444",
						"font-weight":   "bold",
						"border-bottom": "2px solid #fff",
					},
				},
				{
					Selector: ".series-tabs .tab:hover",
					Styles: map[string]string{
						"background": "#333",
					},
				},
			},
		},
	}
	return data.Handle
}

type Dashboard struct {
	Options      Options
	Series       []string
	SeriesIdx    int
	nameProvider func() []string
}

func (d Dashboard) MetricNames() []string {
	lst := d.nameProvider()
	slices.Sort(lst)
	return lst
}

type Options struct {
	BasePath string
	JsSrc    []string
	Style    []CSSStyle
}

func (opt Options) StyleCSS() template.CSS {
	var sb strings.Builder
	for _, style := range opt.Style {
		sb.WriteString(style.Selector)
		sb.WriteString(" {")
		for k, v := range style.Styles {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("; ")
		}
		sb.WriteString("}\n")
	}
	return template.CSS(sb.String())
}

type CSSStyle struct {
	Selector string
	Styles   map[string]string
}

func (d Dashboard) Handle(w http.ResponseWriter, r *http.Request) {
	reqMetric := r.URL.Query().Get("metric")
	if reqMetric == "" {
		d.HandleIndex(w, r)
	} else {
		d.HandleData(w, r)
	}
}

func (d Dashboard) HandleIndex(w http.ResponseWriter, r *http.Request) {
	tsIdxStr := r.URL.Query().Get("tsIdx")
	if _, err := fmt.Sscanf(tsIdxStr, "%d", &d.SeriesIdx); err != nil {
		d.SeriesIdx = 0
	}
	d.Options.BasePath = r.URL.Path
	w.Header().Set("Content-Type", "text/html")
	err := tmplIndex.Execute(w, d)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (d Dashboard) HandleData(w http.ResponseWriter, r *http.Request) {
	metricName := r.URL.Query().Get("metric")
	tsIdxStr := r.URL.Query().Get("tsIdx")

	var tsIdx int
	if _, err := fmt.Sscanf(tsIdxStr, "%d", &tsIdx); err != nil {
		tsIdx = 0
	}
	ss, _ := getSnapshot(metricName, tsIdx)
	series := ss.Series()

	var seriesSingleOrArray any
	if len(series) == 1 {
		seriesSingleOrArray = series[0]
	} else {
		seriesSingleOrArray = series
	}

	subText := ss.Meta.Series + " | " + ss.Meta.Period.String()
	if strings.HasSuffix(subText, "m0s") {
		subText = strings.TrimSuffix(subText, "0s")
	}
	if strings.HasSuffix(subText, "h0m") {
		subText = strings.TrimSuffix(subText, "0m")
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(H{
		"chartOption": H{
			"series": seriesSingleOrArray,
			"title": H{
				"text":    metricName,
				"subtext": subText,
			},
			"legend": H{},
			"tooltip": H{
				"trigger": "axis",
			},
			"xAxis": H{
				"type": "time",
			},
			"yAxis":     H{},
			"animation": false,
		},
		"interval": ss.Interval.Milliseconds(),
		"maxCount": ss.MaxCount,
		"meta": H{
			"measure": ss.Meta.Measure,
			"field":   ss.Meta.Name,
			"series":  ss.Meta.Series,
			"unit":    ss.Meta.Unit,
			"type":    ss.Meta.Type,
		},
	})
	if err != nil {
		http.Error(w, "Error encoding JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

type H map[string]any

type Series struct {
	Name       string         `json:"name"`
	Data       []Item         `json:"data"`
	Type       string         `json:"type"`                 // e.g. 'line',
	Dimensions []string       `json:"dimensions,omitempty"` // ['time', 'value']
	Smooth     bool           `json:"smooth"`               //  true,
	ShowSymbol bool           `json:"showSymbol"`           // showSymbol: true,
	AreaStyle  map[string]any `json:"areaStyle,omitempty"`  // {}
}

type Item struct {
	Time  int64
	Value any
}

func (itm Item) MarshalJSON() ([]byte, error) {
	if arr, ok := itm.Value.([]any); ok {
		return json.Marshal(append([]any{itm.Time}, arr...))
	}
	return json.Marshal([2]any{itm.Time, itm.Value})
}

func (ss Snapshot) Series() []Series {
	var series []Series
	switch ss.Meta.Type {
	case "counter":
		series = []Series{
			{
				Name:       ss.Meta.Name,
				Data:       make([]Item, len(ss.Times)),
				Type:       "line",
				Smooth:     true,
				ShowSymbol: false,
				AreaStyle: H{
					"opacity": 0.5,
				},
			},
		}
		for i, t := range ss.Times {
			series[0].Data[i] = Item{t.UnixMilli(), nil}
		}
		for i := range ss.Times {
			v, ok := ss.Values[i].(*metric.CounterValue)
			if !ok || v.Samples == 0 {
				continue
			}
			series[0].Data[i].Value = v.Value
		}
	case "gauge":
		series = []Series{
			{
				Name:       ss.Meta.Name + "(avg)",
				Data:       make([]Item, len(ss.Times)),
				Type:       "line",
				Smooth:     true,
				ShowSymbol: false,
			},
			{
				Name:       ss.Meta.Name + "(last)",
				Data:       make([]Item, len(ss.Times)),
				Type:       "line",
				Smooth:     true,
				ShowSymbol: false,
			},
		}
		for i, t := range ss.Times {
			series[0].Data[i] = Item{t.UnixMilli(), nil}
			series[1].Data[i] = Item{t.UnixMilli(), nil}
		}
		for i := range ss.Times {
			v, ok := ss.Values[i].(*metric.GaugeValue)
			if !ok || v.Samples == 0 {
				continue
			}
			series[0].Data[i].Value = v.Sum / float64(v.Samples)
			series[1].Data[i].Value = v.Value
		}
	case "meter":
		series = []Series{
			{
				Name:       ss.Meta.Name,
				Data:       make([]Item, len(ss.Times)),
				Type:       "candlestick",
				Smooth:     true,
				ShowSymbol: false,
			},
		}
		for i, t := range ss.Times {
			series[0].Data[i] = Item{t.UnixMilli(), nil}
		}
		for i := range ss.Times {
			v, ok := ss.Values[i].(*metric.MeterValue)
			if !ok || v.Samples == 0 {
				continue
			}
			// data order [open, close, lowest, highest]
			series[0].Data[i].Value = []any{v.First, v.Last, v.Min, v.Max}
		}
	case "histogram":
		last := ss.Values[len(ss.Values)-1].(*metric.HistogramValue)
		for idx, p := range last.P {
			pName := fmt.Sprintf("p%d", int(p*1000))
			if pName[len(pName)-1] == '0' {
				pName = pName[:len(pName)-1]
			}
			series = append(series, Series{
				Name:       ss.Meta.Name + "(" + pName + ")",
				Data:       make([]Item, len(ss.Times)),
				Type:       "line",
				Smooth:     true,
				ShowSymbol: false,
				AreaStyle: H{
					"opacity": 0.3,
				},
			})
			for i, t := range ss.Times {
				series[idx].Data[i] = Item{t.UnixMilli(), nil}
			}
		}

		for i, t := range ss.Times {
			v, ok := ss.Values[i].(*metric.HistogramValue)
			if !ok || v.Samples == 0 {
				continue
			}
			for pIdx, pVal := range v.Values {
				series[pIdx].Data[i] = Item{t.UnixMilli(), pVal}
			}
		}
	}
	return series
}

//go:embed dashboard.tmpl
var tmplIndexHtml string

var tmplIndex = template.Must(template.New("index").Funcs(tmplFuncMap).Parse(tmplIndexHtml))

var tmplFuncMap = template.FuncMap{
	"sub": func(a, b int) int {
		return a - b
	},
}

type Snapshot struct {
	Times    []time.Time
	Values   []metric.Value
	Interval time.Duration
	MaxCount int
	Meta     metric.FieldInfo
}

func getSnapshot(expvarKey string, tsIdx int) (Snapshot, bool) {
	var ret Snapshot
	if g := expvar.Get(expvarKey); g != nil {
		mts := g.(metric.MultiTimeSeries)
		if tsIdx < 0 || tsIdx >= len(mts) {
			return ret, false
		}
		ts := mts[tsIdx]
		times, values := ts.All()
		if len(times) > 0 {
			ret = Snapshot{
				Times:    times,
				Values:   values,
				Interval: ts.Interval(),
				MaxCount: ts.MaxCount(),
				Meta:     ts.Meta().(metric.FieldInfo),
			}
		}
		return ret, true
	}
	return ret, false
}
