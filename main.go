package main

import (
	"encoding/json"
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
	"github.com/OutOfBedlam/metrical/input/httpstat"
	"github.com/OutOfBedlam/metrical/input/netstat"
	"github.com/OutOfBedlam/metrical/input/ps"
	"github.com/OutOfBedlam/metrical/input/runtime"
	"github.com/OutOfBedlam/metrical/output/svg"
)

func main() {
	var httpAddr string
	var outputDir string

	flag.StringVar(&outputDir, "out", "./tmp", "Output directory for SVG files")
	flag.StringVar(&httpAddr, "http", ":3000", "HTTP server address (e.g., :3000)")
	flag.Parse()

	collector := metric.NewCollector(
		metric.WithCollectInterval(1*time.Second),
		//metric.WithSeriesListener("5 min.", 5*time.Second, 60, onProduct),
		metric.WithSeries("5 min.", 5*time.Second, 60),
		metric.WithSeries("5 hr.", 5*time.Minute, 60),
		metric.WithSeries("15 hr.", 15*time.Minute, 60),
		metric.WithExpvarPrefix("metrical"),
		metric.WithReceiverSize(100),
		metric.WithStorage(metric.NewFileStorage(outputDir)),
	)
	collector.AddInputFunc(ps.Collect)
	collector.AddInputFunc(runtime.Collect)
	collector.AddInputFunc(netstat.Collect)
	collector.Start()
	defer collector.Stop()

	if outputDir != "" {
		exporter := metric.NewExporter(1*time.Second, collector.Names())
		exporter.AddOutput(&svg.SVGOutput{DstDir: outputDir}, nil)
		exporter.Start()
		defer exporter.Stop()
	}

	// http server
	if httpAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", handleMetrics(collector))
		svr := &http.Server{
			Addr:      httpAddr,
			Handler:   httpstat.NewHandler(collector.C, mux),
			ConnState: connState,
		}
		go func() {
			fmt.Println("Starting HTTP server on http://127.0.0.1:3000/metrics")
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

func handleMetrics(c *metric.Collector) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		metricNames := c.Names()
		slices.Sort(metricNames)

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
			data.Snapshot = getSnapshot(name, idx)
			if data.Snapshot == nil {
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

func getSnapshot(name string, idx int) *metric.Snapshot {
	if g := expvar.Get(name); g != nil {
		mts := g.(metric.MultiTimeSeries)
		if len(mts) > 0 {
			return mts[idx].Snapshot()
		}
	}
	return nil
}

type Data struct {
	MetricNames []string
	Snapshot    *metric.Snapshot
}

var tmplFuncMap = template.FuncMap{
	"snapshotAll":        SnapshotAll,
	"snapshotField":      SnapshotField,
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
{{ if .Snapshot }}
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
 	{{ $ss := .Snapshot }}
	{{ $field := snapshotField $ss }}
	<h2>{{ $field.Name }} ({{ $ss | productKind }})</h2>
	<table>
		<tr>
			<th>Time</th>
			<th>Value</th>
			<th>JSON</th>
		</tr>
		{{ range $idx, $val := $ss.Values }}
		<tr>
		<td>{{ index $ss.Times $idx | formatTime }}</td>
		<td>{{ productValueString $val $field.Unit }}</td>
		<td>{{ $val }}</td>
		</tr>
		{{end}}
	</table>
{{ end }}
`))

func MiniGraph(ss *metric.Snapshot) template.HTML {
	canvas := svg.CanvasWithSnapshot(ss)
	buff := &strings.Builder{}
	canvas.Export(buff)
	return template.HTML(buff.String())
}

func SnapshotAll(name string) []*metric.Snapshot {
	ret := make([]*metric.Snapshot, 0)
	if g := expvar.Get(name); g != nil {
		mts := g.(metric.MultiTimeSeries)
		for _, ts := range mts {
			snapshot := ts.Snapshot()
			if snapshot != nil {
				ret = append(ret, snapshot)
			}
		}
	}
	return ret
}

func SnapshotField(ss *metric.Snapshot) metric.FieldInfo {
	f, _ := ss.Field()
	return f
}

func ProductValueString(p metric.Product, unit metric.Unit) string {
	if p == nil {
		return "null"
	}
	return unit.Format(ProductValue(p), 2)
}

func ProductValue(p metric.Product) float64 {
	switch v := p.(type) {
	case *metric.CounterProduct:
		return v.Value
	case *metric.GaugeProduct:
		return v.Value
	case *metric.MeterProduct:
		if v.Samples > 0 {
			return v.Sum / float64(v.Samples)
		}
		return 0
	case *metric.HistogramProduct:
		if len(v.Values) > 0 {
			return v.Values[len(v.Values)/2]
		}
		return 0
	default:
		return 0
	}
}

func ProductKind(ss *metric.Snapshot) string {
	p := ss.Values[0]
	switch p.(type) {
	case *metric.CounterProduct:
		return "Counter"
	case *metric.GaugeProduct:
		return "Gauge"
	case *metric.MeterProduct:
		return "Meter"
	case *metric.HistogramProduct:
		return "Histogram"
	default:
		return "Unknown"
	}
}

func onProduct(pd metric.ProductData) {
	m := map[string]any{
		"NAME": fmt.Sprintf("%s:%s", pd.Measure, pd.Field),
		"TIME": pd.Time.UnixNano(),
	}
	switch p := pd.Value.(type) {
	case *metric.CounterProduct:
		m["VALUE"] = p.Value
		m["COUNT"] = p.Samples
	case *metric.GaugeProduct:
		m["VALUE"] = p.Value
		m["COUNT"] = p.Samples
		m["SUM"] = p.Sum
	case *metric.MeterProduct:
		if p.Samples > 0 {
			m["VALUE"] = p.Sum / float64(p.Samples)
		} else {
			m["VALUE"] = 0
		}
		m["COUNT"] = p.Samples
		m["SUM"] = p.Sum
		m["LAST"] = p.Last
		m["FIRST"] = p.First
		m["MIN"] = p.Min
		m["MAX"] = p.Max
	case *metric.HistogramProduct:
		for i, x := range p.P {
			if x == 0.5 {
				m["VALUE"] = p.Values[i]
			}
			m[fmt.Sprintf("P%d", int(x*100))] = p.Values[i]
		}
		if _, exist := m["VALUE"]; !exist {
			m["VALUE"] = 0
		}
		m["COUNT"] = p.Samples
	default:
		fmt.Printf("Unknown product type: %T\n", p)
		return
	}
	n, err := json.Marshal(m)
	if err != nil {
		fmt.Printf("Error marshaling product: %v\n", err)
		return
	}
	rsp, err := http.DefaultClient.Post(
		"http://127.0.0.1:5654/db/write/EXAMPLE",
		"application/x-ndjson",
		strings.NewReader(string(n)))
	if err != nil {
		fmt.Printf("Error sending product: %v\n", err)
		return
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		fmt.Printf("Error response from server: %s\n", rsp.Status)
		return
	}
}
