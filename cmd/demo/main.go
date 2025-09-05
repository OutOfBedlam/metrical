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
	"github.com/OutOfBedlam/metrical/export/charts"
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
		mux := http.NewServeMux()
		mux.HandleFunc("/dashboard", charts.HandleDashboard(collector.PublishNames, collector.SeriesNames()))
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
