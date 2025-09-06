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

func NewDashboard(c *metric.Collector) *Dashboard {
	d := &Dashboard{
		Option:           DefaultDashboardOption(),
		Timeseries:       c.Series(),
		SamplingInterval: c.SamplingInterval(),
		nameProvider:     c.PublishNames,
	}
	return d
}

type Dashboard struct {
	Option           DashboardOption
	PanelOptions     []PanelOption
	Timeseries       []metric.CollectorSeries
	SeriesIdx        int
	ShowAllMetrics   bool
	SamplingInterval time.Duration
	nameProvider     func() []string
}

type PanelOption struct {
	MetricNames      []string
	MetricNameFilter metric.Filter // pattern to match metric names, e.g., "net:*"
	FieldNameFilter  metric.Filter // pattern to match field names, e.g., "*(avg)"
	ID               string
	Title            string
	SubTitle         string
	Type             string // e.g., line, bar
}

// multiple metric names can be added in one place to group them together
// those series are shown together in one graph
func (d *Dashboard) AddPanelOption(co ...PanelOption) {
	for _, c := range co {
		if c.ID == "" {
			c.ID = fmt.Sprintf("@%d", len(d.PanelOptions)+1)
		}
		if c.Title == "" {
			c.Title = strings.Join(c.MetricNames, ", ")
		}
		d.PanelOptions = append(d.PanelOptions, c)
	}
}

func (d Dashboard) Panels() []PanelOption {
	lst := d.nameProvider()
	slices.Sort(lst)

	ret := []PanelOption{}
	for idx := range d.PanelOptions {
		po := &d.PanelOptions[idx]
		if po.MetricNameFilter != nil {
			d.refreshPanel(po)
			// remove matched names from lst
			for _, name := range po.MetricNames {
				if i := slices.Index(lst, name); i >= 0 {
					lst = append(lst[:i], lst[i+1:]...)
				}
			}
		} else if len(po.MetricNames) > 0 {
			for _, name := range po.MetricNames {
				if i := slices.Index(lst, name); i >= 0 {
					lst = append(lst[:i], lst[i+1:]...)
				}
			}
		}
		ret = append(ret, *po)
	}
	if d.ShowAllMetrics {
		for _, name := range lst {
			ret = append(ret, PanelOption{ID: name, Title: name})
		}
	}
	return ret
}

func (d Dashboard) refreshPanel(po *PanelOption) {
	if po.MetricNameFilter == nil {
		return
	}
	lst := d.nameProvider()
	slices.Sort(lst)
	for _, name := range lst {
		if po.MetricNameFilter.Match(name) {
			if !slices.Contains(po.MetricNames, name) {
				po.MetricNames = append(po.MetricNames, name)
			}
		}
	}
}

type DashboardOption struct {
	BasePath string
	JsSrc    []string
	Style    []CSSStyle
}

func DefaultDashboardOption() DashboardOption {
	return DashboardOption{
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
					"gap":             "10px",       // Adds spacing between panels
					"justify-content": "flex-start", // Aligns panels to the left
					// "justify-content": "space-between" // Distributes panels evenly with space between
				},
			},
			{
				Selector: ".panel",
				Styles: map[string]string{
					"flex":          "0 0 400px", // Each panel takes up 400px width
					"height":        "300px",     // Fixed height for each panel
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
	}
}

func (opt DashboardOption) StyleCSS() template.CSS {
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
	if id := r.URL.Query().Get("id"); id == "" {
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
	d.Option.BasePath = r.URL.Path
	w.Header().Set("Content-Type", "text/html")
	err := tmplIndex.Execute(w, d)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (d Dashboard) HandleData(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query.Get("id")
	tsIdxStr := query.Get("tsIdx")

	var tsIdx int
	if _, err := fmt.Sscanf(tsIdxStr, "%d", &tsIdx); err != nil {
		tsIdx = 0
	}
	var panelOpt PanelOption
	for _, po := range d.PanelOptions {
		if po.ID == id {
			panelOpt = po
			break
		}
	}
	if panelOpt.ID == "" {
		// id not found, which means it is a single metric name panel
		panelOpt = PanelOption{
			ID:          id,
			MetricNames: []string{id},
		}
	}
	if panelOpt.MetricNameFilter != nil {
		d.refreshPanel(&panelOpt)
	}

	var series []Series
	var meta *metric.FieldInfo
	var seriesMaxCount int
	var seriesInterval time.Duration

	for _, metricName := range panelOpt.MetricNames {
		ss, ssExists := getSnapshot(metricName, tsIdx)

		if !ssExists {
			panelOpt.SubTitle = "Metric not found"
			continue
		}
		series = append(series, ss.Series(panelOpt)...)

		if meta == nil {
			meta = &ss.Meta
			seriesInterval = ss.Interval
			seriesMaxCount = ss.MaxCount
		}
		if panelOpt.Title == "" {
			panelOpt.Title = ss.PublishName
		}
	}

	roundTime := func(t time.Time, interval time.Duration) time.Time {
		return t.Add(interval / 2).Round(interval)
	}
	xAxisMin := roundTime(time.Now().Add(-seriesInterval*time.Duration(seriesMaxCount)), seriesInterval).UnixMilli()
	xAxisMax := roundTime(time.Now(), seriesInterval).UnixMilli()
	var seriesSingleOrArray any
	if len(series) == 1 {
		seriesSingleOrArray = series[0]
	} else {
		seriesSingleOrArray = series
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(H{
		"chartOption": H{
			"series": seriesSingleOrArray,
			"title": H{
				"text":    panelOpt.Title,
				"subtext": panelOpt.SubTitle,
			},
			"legend": H{},
			"tooltip": H{
				"trigger": "axis",
			},
			"xAxis": H{
				"type": "time",
				"min":  xAxisMin,
				"max":  xAxisMax,
			},
			"yAxis":     H{},
			"animation": false,
		},
		"interval": seriesInterval.Milliseconds(),
		"maxCount": seriesMaxCount,
		"meta": H{
			"measure": meta.Measure,
			"field":   meta.Name,
			"series":  meta.Series,
			"unit":    meta.Unit,
			"type":    meta.Type,
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

type Series struct {
	Name       string         `json:"name"`
	Data       []Item         `json:"data"`
	Type       string         `json:"type"`                // e.g. 'line',
	Stack      any            `json:"stack,omitempty"`     // nil or stack-name
	Smooth     bool           `json:"smooth"`              //  true,
	ShowSymbol bool           `json:"showSymbol"`          // showSymbol: true,
	AreaStyle  map[string]any `json:"areaStyle,omitempty"` // {}
}

func (ss Snapshot) Series(opt PanelOption) []Series {
	var series []Series
	switch ss.Meta.Type {
	case "counter":
		var typ = "bar"
		var stack any
		switch opt.Type {
		case "line-stack":
			typ = "line"
			stack = "total"
		case "bar-stack":
			typ = "bar"
			stack = "total"
		default:
			if opt.Type != "" {
				typ = opt.Type
			}
		}
		series = []Series{
			{
				Name:       ss.Meta.Name,
				Data:       make([]Item, len(ss.Times)),
				Type:       typ,
				Stack:      stack,
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
		// for gauge, show avg and last value
		seriesNames := []string{ss.Meta.Name + "(avg)", ss.Meta.Name + "(last)"}
		seriesFlags := map[string]int{}
		for i, seriesName := range seriesNames {
			if opt.FieldNameFilter != nil && !opt.FieldNameFilter.Match(seriesName) {
				continue
			}
			switch i {
			case 0:
				seriesFlags["avg"] = len(series)
			case 1:
				seriesFlags["last"] = len(series)
			}
			series = append(series, Series{
				Name:       seriesName,
				Data:       make([]Item, len(ss.Times)),
				Type:       "line",
				Smooth:     true,
				ShowSymbol: false,
			})
		}
		for seriesIdx := range series {
			for i, t := range ss.Times {
				series[seriesIdx].Data[i] = Item{t.UnixMilli(), nil}
			}
		}
		for i := range ss.Times {
			v, ok := ss.Values[i].(*metric.GaugeValue)
			if !ok || v.Samples == 0 {
				continue
			}
			if idx, ok := seriesFlags["avg"]; ok {
				series[idx].Data[i].Value = v.Sum / float64(v.Samples)
			}
			if idx, ok := seriesFlags["last"]; ok {
				series[idx].Data[i].Value = v.Value
			}
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
	"seriesTitle": func(s metric.CollectorSeries) string {
		title := s.Name + " | " + s.Period.String()
		if strings.HasSuffix(title, "m0s") {
			title = strings.TrimSuffix(title, "0s")
		}
		if strings.HasSuffix(title, "h0m") {
			title = strings.TrimSuffix(title, "0m")
		}
		return title
	},
}

type Snapshot struct {
	PublishName string
	Times       []time.Time
	Values      []metric.Value
	Interval    time.Duration
	MaxCount    int
	Meta        metric.FieldInfo
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
				PublishName: expvarKey,
				Times:       times,
				Values:      values,
				Interval:    ts.Interval(),
				MaxCount:    ts.MaxCount(),
				Meta:        ts.Meta().(metric.FieldInfo),
			}
		}
		return ret, true
	}
	return ret, false
}
