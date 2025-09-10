package main

import (
	_ "embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/OutOfBedlam/metric"
	_ "github.com/OutOfBedlam/metrical/input/gostat"
	"github.com/OutOfBedlam/metrical/input/httpstat"
	_ "github.com/OutOfBedlam/metrical/input/ps"
	_ "github.com/OutOfBedlam/metrical/output/ndjson"
	"github.com/OutOfBedlam/metrical/registry"
)

//go:generate go run main.go -gen-config ./metrical-default.conf

type Metrical struct {
	Data      DataConfig        `toml:"data"`
	Http      HttpConfig        `toml:"http"`
	Collector *metric.Collector `toml:"-"`
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

//go:embed "metrical-default.conf"
var configContent string

func main() {
	var configFilename string
	var genConfigFilename string

	flag.StringVar(&configFilename, "config", "", "metrical config file path")
	flag.StringVar(&genConfigFilename, "gen-config", "", "Generates default config to the given filename")
	flag.Parse()

	mc := Metrical{
		Http: HttpConfig{
			Listen:        ":3000",
			AdvAddr:       "http://localhost:3000",
			DashboardPath: "/dashboard",
		},
		Data: DataConfig{
			SamplingInterval: time.Second,
			InputBuffer:      100,
			Store:            "./tmp/store/",
			Filter: FilterConfig{
				Includes: []string{},
				Excludes: []string{},
			},
			Timeseries: []TimeseriesConfig{
				{Name: "15m", Interval: 10 * time.Second, MaxCount: 90},
				{Name: "1h30m", Interval: time.Minute, MaxCount: 90},
				{Name: "2d", Interval: 30 * time.Minute, MaxCount: 96},
			},
		},
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
	if err := mc.loadConfig(configContent); err != nil {
		panic(err)
	}

	mc.Collector.Start()
	defer mc.Collector.Stop()

	// http server
	if mc.Http.Listen != "" {
		netstatFilter := metric.MustCompile([]string{"netstat:tcp_*", "netstat:udp_*"}, ':')
		lastOnlyFilter := metric.MustCompile([]string{"*(last)"})
		avgOnlyFilter := metric.MustCompile([]string{"*(avg)"})
		httpStatusFilter := metric.MustCompile([]string{"http:status_[1-5]xx"}, ':')

		dash := metric.NewDashboard(mc.Collector)
		dash.PageTitle = "Metrical - Demo"
		dash.ShowRemains = true
		dash.SetTheme("light")
		dash.SetPanelHeight(300)
		dash.SetPanelMinWidth(400)
		dash.SetPanelMaxWidth(600)
		dash.AddChart(metric.Chart{Title: "CPU Usage", MetricNameFilter: metric.MustCompile([]string{"cpu:cpu_*"}, ':')})
		dash.AddChart(metric.Chart{Title: "MEM Usage", MetricNames: []string{"mem:percent"}})
		dash.AddChart(metric.Chart{Title: "Go Routines", MetricNames: []string{"go_runtime:goroutines"}, ValueSelector: avgOnlyFilter})
		dash.AddChart(metric.Chart{Title: "Go Heap In Use", MetricNames: []string{"go_mem:heap_inuse"}, ValueSelector: avgOnlyFilter})
		dash.AddChart(metric.Chart{Title: "Network I/O", MetricNames: []string{"net:bytes_recv", "net:bytes_sent"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Packets", MetricNames: []string{"net:packets_recv", "net:packets_sent"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Errors", MetricNames: []string{"net:drop_in", "net:drop_out", "net:err_in", "net:err_out"}, Type: metric.ChartTypeBarStack})
		dash.AddChart(metric.Chart{Title: "Netstat", MetricNameFilter: netstatFilter, ValueSelector: lastOnlyFilter})
		dash.AddChart(metric.Chart{Title: "HTTP Latency", MetricNames: []string{"http:latency"}})
		dash.AddChart(metric.Chart{Title: "HTTP I/O", MetricNames: []string{"http:bytes_recv", "http:bytes_sent"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "HTTP Status", MetricNameFilter: httpStatusFilter, Type: metric.ChartTypeBarStack})

		mux := http.NewServeMux()
		mux.Handle(mc.Http.DashboardPath, dash)
		svr := &http.Server{
			Addr:      mc.Http.Listen,
			Handler:   httpstat.NewHandler(mc.Collector.C, mux),
			ConnState: connState,
		}
		go func() {
			fmt.Printf("Starting HTTP server on %s%s\n",
				mc.Http.AdvAddr, mc.Http.DashboardPath)
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
			fmt.Println("Error open", filename, err.Error())
			return
		}
		defer fd.Close()
	}
	enc := toml.NewEncoder(fd)
	enc.Encode(mc)
	fmt.Fprintln(fd)
	registry.GenerateSampleConfig(fd)
}

func (mc *Metrical) loadConfig(content string) error {
	if _, err := toml.Decode(content, mc); err != nil {
		return err
	}
	if mc.Data.SamplingInterval < time.Second {
		mc.Data.SamplingInterval = time.Second
	}
	options := []metric.CollectorOption{
		metric.WithSamplingInterval(mc.Data.SamplingInterval),
		metric.WithInputBuffer(mc.Data.InputBuffer),
		metric.WithPrefix(mc.Data.Prefix),
		metric.WithStorage(metric.NewFileStorage(mc.Data.Store)),
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
	if err := registry.LoadConfig(mc.Collector, content); err != nil {
		return err
	}
	return nil
}
