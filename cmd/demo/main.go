package main

import (
	"expvar"
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/export"
	"github.com/OutOfBedlam/metrical/export/svg"
	"github.com/OutOfBedlam/metrical/input/gostat"
	"github.com/OutOfBedlam/metrical/input/httpstat"
	"github.com/OutOfBedlam/metrical/input/ps"
	"github.com/OutOfBedlam/metrical/output/ndjson"
)

func main() {
	var httpAddr string
	var storeDir string
	var exportDir string

	flag.StringVar(&httpAddr, "http", "127.0.0.1:3000", "HTTP server address (e.g., :3000)")
	flag.StringVar(&storeDir, "store", "./tmp", "storage directory for metrics")
	flag.StringVar(&exportDir, "export", "", "Export directory for SVG files")
	flag.Parse()

	collector := metric.NewCollector(
		metric.WithInterval(1*time.Second),
		metric.WithSeries("5 min.", 5*time.Second, 60),
		metric.WithSeries("5 hr.", 5*time.Minute, 60),
		metric.WithSeries("15 hr.", 15*time.Minute, 60),
		metric.WithPrefix("metrical"),
		metric.WithInputBuffer(100),
		metric.WithStorage(metric.NewFileStorage(storeDir)),
	)
	collector.AddInputFunc(gostat.Runtime{}.Collect)
	collector.AddInputFunc(ps.PS{}.Collect)
	collector.AddInputFunc(ps.NetStat{}.Collect)
	collector.AddOutputFunc(
		metric.DenyNameFilter(ndjson.Output{DestUrl: ""}.Export,
			"netstat:tcp_last_ack", "netstat:tcp_none", "netstat:tcp_time_wait", "netstat:tcp_closing",
		),
	)
	collector.Start()
	defer collector.Stop()

	if exportDir != "" {
		exporter := export.NewExporter(1*time.Second, collector.Names())
		exporter.AddOutput(&svg.SVGOutput{DstDir: exportDir}, nil)
		exporter.Start()
		defer exporter.Stop()
	}

	// http server
	if httpAddr != "" {
		metricNames := collector.Names()
		slices.Sort(metricNames)

		mux := http.NewServeMux()
		mux.HandleFunc("/dashboard", handleDashboard(metricNames))
		svr := &http.Server{
			Addr:      httpAddr,
			Handler:   httpstat.NewHandler(collector.C, mux),
			ConnState: connState,
		}
		go func() {
			addr := httpAddr
			if strings.HasPrefix(addr, ":") {
				addr = "127.0.0.1" + addr
			}
			fmt.Printf("Starting HTTP server on http://%s/dashboard\n", addr)
			if err := svr.ListenAndServe(); err != nil {
				if err == http.ErrServerClosed {
					fmt.Println("HTTP server closed")
				} else {
					fmt.Println("Error starting HTTP server:", err)
				}
			}
		}()
		defer svr.Close()
	}

	// wait signal ^C
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh
}

func connState(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		if c, ok := conn.(*net.TCPConn); ok {
			c.SetLinger(0)
		}
	}
}

func handleDashboard(metricNames []string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		q := r.URL.Query()
		name := q.Get("n")
		idx := 0
		if name != "" {
			if str := q.Get("i"); str != "" {
				fmt.Sscanf(str, "%d", &idx)
			}
		}
		if str := q.Get("r"); str != "" {
			if refresh, err := fmt.Sscanf(str, "%d", &idx); err == nil {
				if refresh > 0 {
					w.Header().Set("Refresh", fmt.Sprintf("%d", refresh))
				}
			}
		}
		var err error
		var data = Data{}
		if name == "" {
			data.MetricNames = metricNames
		} else {
			data.MetricNames = []string{name}
			data.Times, data.Values, data.Meta = getSnapshot(name, idx)
			data.Detail = true
			if len(data.Times) == 0 {
				http.Error(w, "Metric not found", http.StatusNotFound)
				return
			}
		}
		err = tmplIndex.Execute(w, data)
		if err != nil {
			http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func getSnapshot(name string, idx int) ([]time.Time, []metric.Value, metric.FieldInfo) {
	if g := expvar.Get(name); g != nil {
		mts := g.(metric.MultiTimeSeries)
		if idx >= 0 && idx < len(mts) {
			ts := mts[idx]
			meta := ts.Meta().(metric.FieldInfo)
			times, values := ts.All()
			return times, values, meta
		}
	}
	return nil, nil, metric.FieldInfo{}
}

type Data struct {
	// for index view
	MetricNames []string
	// for detail view
	Detail bool
	Meta   metric.FieldInfo
	Times  []time.Time
	Values []metric.Value
}

var tmplFuncMap = template.FuncMap{
	"snapshotAll":        SnapshotAll,
	"productValueString": ProductValueString,
	"productKind":        ProductKind,
	"miniGraph":          MiniGraph,
	"formatTime":         func(t time.Time) string { return t.Format(time.TimeOnly) },
}

var tmplIndex = template.Must(template.New("index").Funcs(tmplFuncMap).
	Parse(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<meta http-equiv="cache-control" content="no-cache, no-store, must-revalidate">
	<meta http-equiv="cache-control" content="max-age=0">
	<meta http-equiv="pragma" content="no-cache">
	<meta http-equiv="expires" content="0">
	<title>Metrics</title>
	<style>
		body { font-family: Arial, sans-serif; }
		table { width: 100%; border-collapse: collapse; }
		th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }
		th { background-color: #f2f2f2; }
		tr:hover { background-color: #f1f1f1; }
		.graphRow {
			display: flex;
			justify-content: flex-start;
			flex-direction: row;
			flex-wrap: wrap;
		}
		.graph {
			flex: 0;
			margin-left: 10px;
			margin-right: 20px;
		}
	</style>
</head>
<body>
<h1>Metrics</h1>
{{ if .Detail }}
	{{ template "doDetail" . }}
{{ else }}
	{{ template "doMiniGraph" . }}
{{end}}
</body>
</html>

{{ define "doMiniGraph" }}
 	{{range $n, $name := .MetricNames}}
		<h2>{{$name}}</h2>
		<div class="graphRow">
		{{ range $idx, $ss := snapshotAll $name }}
		 <div class="graph">
			<a href="?n={{ $name }}&i={{ $idx }}">{{ $ss | miniGraph }}</a>
		</div>
		{{ end }}
		 </div>
	{{end}}
{{ end }}

{{ define "doDetail" }}
	{{ $meta := .Meta }}
	{{ $times := .Times }}
	{{ $values := .Values }}
	<h2>{{ $meta.Name }} ({{ index $values 0 | productKind }})</h2>
	<table>
		<tr>
			<th>Time</th>
			<th>Value</th>
			<th>JSON</th>
		</tr>
		{{ range $idx, $val := .Values }}
		<tr>
		<td>{{ index $times $idx | formatTime }}</td>
		<td>{{ productValueString $val $meta.Unit }}</td>
		<td>{{ $val }}</td>
		</tr>
		{{end}}
	</table>
{{ end }}
`))

type Snapshot struct {
	Times    []time.Time
	Values   []metric.Value
	Interval time.Duration
	MaxCount int
	Meta     metric.FieldInfo
}

func MiniGraph(ss Snapshot) template.HTML {
	canvas := svg.CanvasWithSnapshot(ss.Times, ss.Values, ss.Meta, ss.Interval, ss.MaxCount)
	buff := &strings.Builder{}
	canvas.Export(buff)
	return template.HTML(buff.String())
}

func SnapshotAll(name string) []Snapshot {
	ret := make([]Snapshot, 0)
	if g := expvar.Get(name); g != nil {
		mts := g.(metric.MultiTimeSeries)
		for _, ts := range mts {
			times, values := ts.All()
			if len(times) > 0 {
				ret = append(ret, Snapshot{
					Times:    times,
					Values:   values,
					Interval: ts.Interval(),
					MaxCount: ts.MaxCount(),
					Meta:     ts.Meta().(metric.FieldInfo),
				})
			}
		}
	}
	return ret
}

func ProductValueString(p metric.Value, unit metric.Unit) string {
	if p == nil {
		return "null"
	}
	return unit.Format(ProductValue(p), 2)
}

func ProductValue(p metric.Value) float64 {
	switch v := p.(type) {
	case *metric.CounterValue:
		return v.Value
	case *metric.GaugeValue:
		return v.Value
	case *metric.MeterValue:
		if v.Samples > 0 {
			return v.Sum / float64(v.Samples)
		}
		return 0
	case *metric.HistogramValue:
		if len(v.Values) > 0 {
			return v.Values[len(v.Values)/2]
		}
		return 0
	default:
		return 0
	}
}

func ProductKind(value metric.Value) string {
	switch value.(type) {
	case *metric.CounterValue:
		return "Counter"
	case *metric.GaugeValue:
		return "Gauge"
	case *metric.MeterValue:
		return "Meter"
	case *metric.HistogramValue:
		return "Histogram"
	default:
		return "Unknown"
	}
}
