package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/export"
	"github.com/OutOfBedlam/metrical/export/svg"
	"github.com/OutOfBedlam/metrical/input/gostat"
	"github.com/OutOfBedlam/metrical/input/httpstat"
	"github.com/OutOfBedlam/metrical/input/ps"
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
		metric.WithSamplingInterval(1*time.Second),
		metric.WithSeries("15m", 10*time.Second, 90),
		metric.WithSeries("3h", 60*time.Second, 180),
		metric.WithSeries("30h", 10*time.Minute, 180),
		metric.WithSeries("3d", 30*time.Minute, 144),
		metric.WithPrefix("metrical"),
		metric.WithInputBuffer(100),
		metric.WithStorage(metric.NewFileStorage(storeDir)),
	)
	collector.AddInputFunc(gostat.Runtime{}.Collect)
	collector.AddInputFunc(ps.PS{}.Collect)
	collector.AddInputFunc(ps.NetStat{}.Collect)
	psNet := &ps.Net{Interfaces: []string{"eth*"}}
	collector.AddInputFunc(psNet.Collect)
	// collector.AddOutputFunc(
	// 	metric.DenyNameFilter(ndjson.Output{DestUrl: ""}.Export,
	// 		"netstat:tcp_last_ack", "netstat:tcp_none", "netstat:tcp_time_wait", "netstat:tcp_closing",
	// 	),
	// )
	collector.Start()
	defer collector.Stop()

	if exportDir != "" {
		exporter := export.NewExporter(1*time.Second, collector.PublishNames())
		exporter.AddOutput(&svg.SVGOutput{DstDir: exportDir}, nil)
		exporter.Start()
		defer exporter.Stop()
	}

	// http server
	if httpAddr != "" {
		netstatFilter := metric.MustCompile([]string{"metrical:netstat:tcp_*", "metrical:netstat:udp_*"}, ':')
		lastOnlyFilter := metric.MustCompile([]string{"*(last)"})
		avgOnlyFilter := metric.MustCompile([]string{"*(avg)"})
		httpStatusFilter := metric.MustCompile([]string{"metrical:http:status_[1-5]xx"}, ':')

		dash := metric.NewDashboard(collector)
		dash.AddChart(metric.Chart{Title: "CPU Usage", MetricNames: []string{"metrical:ps:cpu_percent"}})
		dash.AddChart(metric.Chart{Title: "MEM Usage", MetricNames: []string{"metrical:ps:mem_percent"}})
		dash.AddChart(metric.Chart{Title: "Go Routines", MetricNames: []string{"metrical:runtime:goroutines"}})
		dash.AddChart(metric.Chart{Title: "Go Heap In Use", MetricNames: []string{"metrical:runtime:heap_inuse"}, ValueSelector: avgOnlyFilter})
		dash.AddChart(metric.Chart{Title: "Network I/O", MetricNames: []string{"metrical:net:bytes_recv", "metrical:net:bytes_sent"}, Type: "line"})
		dash.AddChart(metric.Chart{Title: "Network Packets", MetricNames: []string{"metrical:net:packets_recv", "metrical:net:packets_sent"}, Type: "line"})
		dash.AddChart(metric.Chart{Title: "Network Errors", MetricNames: []string{"metrical:net:drop_in", "metrical:net:drop_out", "metrical:net:err_in", "metrical:net:err_out"}, Type: "bar-stack"})
		dash.AddChart(metric.Chart{Title: "Netstat", MetricNameFilter: netstatFilter, ValueSelector: lastOnlyFilter})
		dash.AddChart(metric.Chart{Title: "HTTP Latency", MetricNames: []string{"metrical:http:latency"}})
		dash.AddChart(metric.Chart{Title: "HTTP I/O", MetricNames: []string{"metrical:http:bytes_recv", "metrical:http:bytes_sent"}, Type: "line"})
		dash.AddChart(metric.Chart{Title: "HTTP Status", MetricNameFilter: httpStatusFilter, Type: "bar-stack"})
		dash.ShowRemains = true

		mux := http.NewServeMux()
		mux.Handle("/dashboard", dash)
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
