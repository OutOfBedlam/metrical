package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/OutOfBedlam/metric"
	_ "github.com/OutOfBedlam/metrical/input/disk"
	_ "github.com/OutOfBedlam/metrical/input/diskio"
	_ "github.com/OutOfBedlam/metrical/input/gostat"
	_ "github.com/OutOfBedlam/metrical/input/ps"
	"github.com/OutOfBedlam/metrical/middleware/httpstat"
	_ "github.com/OutOfBedlam/metrical/output/ndjson"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/OutOfBedlam/metrical/store/sqlite"
)

//go:generate go run main.go -gen-config ./metrical-example.conf

type Metrical struct {
	Data      DataConfig        `toml:"data"`
	Http      HttpConfig        `toml:"http"`
	Collector *metric.Collector `toml:"-"`
	Storage   metric.Storage    `toml:"-"`

	instantiatedInputs []string
}

type HttpConfig struct {
	Listen        string `toml:"listen"`
	AdvAddr       string `toml:"adv_addr"`
	DashboardPath string `toml:"dashboard"`
}

type DataConfig struct {
	SamplingInterval time.Duration      `toml:"sampling_interval"`
	InputBuffer      int                `toml:"input_buffer"`
	Prefix           string             `toml:"prefix"`
	Store            string             `toml:"store"`
	Filter           FilterConfig       `toml:"filter"`
	Timeseries       []TimeseriesConfig `toml:"timeseries"`
}

type TimeseriesConfig struct {
	Name     string        `toml:"name"`
	Interval time.Duration `toml:"interval"`
	MaxCount int           `toml:"length"`
}

type FilterConfig struct {
	Includes []string `toml:"includes"`
	Excludes []string `toml:"excludes"`
}

//go:embed "metrical.toml"
var configContent string

func main() {
	var configFilename string
	var genConfigFilename string
	var logLevelStr string = "INFO"

	flag.StringVar(&configFilename, "config", "", "metrical config file path")
	flag.StringVar(&genConfigFilename, "gen-config", "", "Generates default config to the given filename")
	flag.StringVar(&logLevelStr, "log-level", logLevelStr, "log level [DEBUG, INFO, WARN, ERROR]")
	flag.Parse()

	var logLevel = new(slog.LevelVar)
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	slog.SetDefault(slog.New(logHandler))
	switch strings.ToUpper(logLevelStr) {
	case "DEBUG":
		logLevel.Set(slog.LevelDebug)
	case "WARN":
		logLevel.Set(slog.LevelWarn)
	case "ERROR":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}

	mc := Metrical{}
	_, err := toml.Decode(configContent, &mc)
	if err != nil {
		panic(err)
	}

	if genConfigFilename != "" {
		mc.genConfig(genConfigFilename)
		return
	}
	if configFilename != "" {
		if b, err := os.ReadFile(configFilename); err != nil {
			panic(err)
		} else {
			configContent = string(b)
		}
	}
	if _, err := toml.Decode(configContent, &mc); err != nil {
		panic(err)
	}
	if mc.Data.Store != "" {
		if strings.HasPrefix(mc.Data.Store, "sqlite:") {
			path := strings.TrimPrefix(mc.Data.Store, "sqlite:")
			if storage, err := sqlite.NewStorage(path, mc.Data.InputBuffer); err != nil {
				panic(err)
			} else {
				mc.Storage = storage
			}
		} else { // default to file storage
			mc.Storage = metric.NewFileStorage(mc.Data.Store, mc.Data.InputBuffer)
		}
		if mc.Storage != nil {
			if opener, ok := mc.Storage.(interface{ Open() error }); ok {
				if err := opener.Open(); err != nil {
					panic(err)
				}
			}
		}
	}
	// load registry and inputs/outputs,
	// it requires mc.Storage to restore the previous timeseries
	if err := mc.loadCollector(configContent); err != nil {
		panic(err)
	}
	mc.Collector.Start()
	defer func() {
		mc.Collector.Stop()
		if closer, ok := mc.Storage.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	// http server
	if mc.Http.Listen != "" {
		dash := metric.NewDashboard(mc.Collector)
		dash.PageTitle = "Metrical - Demo"
		dash.ShowRemains = false
		dash.SetTheme("light")
		dash.SetPanelHeight("300px")   // default
		dash.SetPanelMinWidth("400px") // default
		dash.SetPanelMaxWidth("1fr")   // default
		if mc.HasInput("load") {
			dash.AddChart(metric.Chart{Title: "Load Average", MetricNames: []string{"load:load1", "load:load5", "load:load15"}, FieldNames: []string{"avg"}, Type: metric.ChartTypeLine})
		}
		if mc.HasInput("cpu") {
			dash.AddChart(metric.Chart{Title: "CPU Usage", MetricNames: []string{"cpu:cpu_*"}, FieldNames: []string{"ohlc", "avg"}})
		}
		if mc.HasInput("mem") {
			dash.AddChart(metric.Chart{Title: "MEM Usage", MetricNames: []string{"mem:percent"}, FieldNames: []string{"max"}})
		}
		if mc.HasInput("disk") {
			dash.AddChart(metric.Chart{Title: "Disk Usage", MetricNames: []string{"disk:*:used_percent"}, FieldNames: []string{"last"}, Type: metric.ChartTypeLine})
		}
		if mc.HasInput("go_runtime") {
			dash.AddChart(metric.Chart{Title: "Go Routines", MetricNames: []string{"go:runtime:goroutines"}, FieldNames: []string{"max", "min"}})
		}
		dash.AddChart(metric.Chart{Title: "Go Heap In Use", MetricNames: []string{"go:mem:heap_inuse"}, FieldNames: []string{"max", "min"}})
		dash.AddChart(metric.Chart{Title: "Network I/O", MetricNames: []string{"net:*:bytes_recv", "net:*:bytes_sent"}, FieldNames: []string{"abs-diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Packets", MetricNames: []string{"net:*:packets_recv", "net:*:packets_sent"}, FieldNames: []string{"non-negative-diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Errors", MetricNames: []string{"net:*:drop_in", "net:*:drop_out", "net:*:err_in", "net:*:err_out"}, Type: metric.ChartTypeScatter, ShowSymbol: true})
		dash.AddChart(metric.Chart{Title: "Netstat", MetricNames: []string{"netstat:tcp_*", "netstat:udp_*"}, FieldNames: []string{"last"}})
		dash.AddChart(metric.Chart{Title: "HTTP Latency", MetricNames: []string{"http:latency"}, FieldNames: []string{"p50", "p99"}})
		dash.AddChart(metric.Chart{Title: "HTTP I/O", MetricNames: []string{"http:bytes_recv", "http:bytes_sent"}, Type: metric.ChartTypeLine, ShowSymbol: true})
		dash.AddChart(metric.Chart{Title: "HTTP Status", MetricNames: []string{"http:status_[1-5]xx"}, Type: metric.ChartTypeBarStack})
		if mc.HasInput("diskio") {
			dash.AddChart(metric.Chart{Title: "Disk I/O Bytes", MetricNames: []string{"diskio:*:read_bytes", "diskio:*:write_bytes"}, FieldNames: []string{"non-negative-diff"}, Type: metric.ChartTypeLine})
			dash.AddChart(metric.Chart{Title: "Disk I/O Count", MetricNames: []string{"diskio:*:read_count", "diskio:*:write_count"}, FieldNames: []string{"non-negative-diff"}, Type: metric.ChartTypeLine})
			dash.AddChart(metric.Chart{Title: "Disk I/O Time", MetricNames: []string{"diskio:*:read_time", "diskio:*:write_time", "diskio:*:io_time", "diskio:*:weighted_io_time"}, FieldNames: []string{"non-negative-diff"}, Type: metric.ChartTypeLine})
		}
		mux := http.NewServeMux()
		mux.Handle(mc.Http.DashboardPath, dash)
		mux.Handle("/debug/pprof", pprof.Handler("/debug/pprof"))
		svr := &http.Server{
			Addr:      mc.Http.Listen,
			Handler:   httpstat.NewHandler(mc.Collector.C, mux),
			ConnState: connState,
		}
		go func() {
			slog.Info("Starting HTTP server on " + mc.Http.Listen + mc.Http.DashboardPath)
			if err := svr.ListenAndServe(); err != nil {
				if err == http.ErrServerClosed {
					slog.Info("HTTP server closed")
				} else {
					slog.Error("Error starting HTTP server", "error", err)
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

func (mc Metrical) HasInput(name string) bool {
	for _, n := range mc.instantiatedInputs {
		if n == name {
			return true
		}
	}
	return false
}

func (mc Metrical) genConfig(filename string) {
	if filename == "" {
		return
	}
	var err error
	var fd *os.File
	if filename == "-" {
		fd = os.Stdout
	} else {
		fd, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			slog.Error("Error open config file for writing", "file", filename, "error", err)
			return
		}
		defer fd.Close()
	}
	fmt.Fprintln(fd, "# This is the default configuration file for metrical.")
	fmt.Fprintln(fd, configContent)
	fmt.Fprintln(fd)
	registry.GenerateSampleConfig(fd)
}

func (mc *Metrical) loadCollector(content string) error {
	if mc.Data.SamplingInterval < time.Second {
		mc.Data.SamplingInterval = time.Second
	}
	options := []metric.CollectorOption{
		metric.WithSamplingInterval(mc.Data.SamplingInterval),
		metric.WithInputBuffer(mc.Data.InputBuffer),
		metric.WithPrefix(mc.Data.Prefix),
		metric.WithStorage(mc.Storage),
	}
	for _, ts := range mc.Data.Timeseries {
		if ts.Interval < time.Second {
			continue
		}
		if ts.MaxCount <= 1 {
			continue
		}
		options = append(options, metric.WithSeries(ts.Name, ts.Interval, ts.MaxCount))
	}
	if len(mc.Data.Filter.Includes) > 0 || len(mc.Data.Filter.Excludes) > 0 {
		filter, err := metric.CompileIncludeAndExclude(mc.Data.Filter.Includes, mc.Data.Filter.Excludes, ':')
		if err != nil {
			return fmt.Errorf("error compiling filter %v: %w", mc.Data.Filter, err)
		}
		options = append(options, metric.WithTimeseriesFilter(filter))
	}
	mc.Collector = metric.NewCollector(options...)
	if inputs, outputs, err := registry.LoadConfig(mc.Collector, content); err != nil {
		return err
	} else {
		mc.instantiatedInputs = inputs
		_ = outputs
	}
	return nil
}
