package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
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
	_ "github.com/OutOfBedlam/metrical/input/opcua"
	_ "github.com/OutOfBedlam/metrical/input/ps"
	"github.com/OutOfBedlam/metrical/middleware/httpstat"
	_ "github.com/OutOfBedlam/metrical/output/ndjson"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/OutOfBedlam/metrical/store/sqlite"
	"github.com/OutOfBedlam/webterm"
	"github.com/OutOfBedlam/webterm/webexec"
	"github.com/OutOfBedlam/webterm/webssh"
	"github.com/OutOfBedlam/webterm/webtail"
	"golang.org/x/crypto/ssh"
)

//go:generate go run main.go -gen-config ./metrical-example.conf

type Metrical struct {
	Log       LogConfig         `toml:"log"`
	Data      DataConfig        `toml:"data"`
	Http      HttpConfig        `toml:"http"`
	Collector *metric.Collector `toml:"-"`
	Storage   metric.Storage    `toml:"-"`

	instantiatedInputs []string
}

type LogConfig struct {
	Filename string   `toml:"filename"`
	Stdout   bool     `toml:"stdout"`
	Level    LogLevel `toml:"level"`
}

type LogLevel string

var _ flag.Value = (*LogLevel)(nil)

func (ll *LogLevel) String() string {
	return string(*ll)
}

func (ll *LogLevel) Set(value string) error {
	*ll = LogLevel(strings.ToUpper(value))
	return nil
}

func (ll *LogLevel) Level() *slog.LevelVar {
	var logLevel = new(slog.LevelVar)
	switch string(*ll) {
	case "DEBUG":
		logLevel.Set(slog.LevelDebug)
	case "WARN":
		logLevel.Set(slog.LevelWarn)
	case "ERROR":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}
	return logLevel
}

type HttpConfig struct {
	Listen    string            `toml:"listen"`
	AdvAddr   string            `toml:"adv_addr"`
	Dashboard []DashboardConfig `toml:"dashboard"`
	Tails     []WebTailConfig   `toml:"tail"`
	Terms     []WebTermConfig   `toml:"term"`
	SSHs      []WebSSHConfig    `toml:"ssh"`
}

type DashboardConfig struct {
	Path string `toml:"path"`
}

type WebTailConfig struct {
	Path  string        `toml:"path"`
	Files []WebTailFile `toml:"file"`
}

type WebTailFile struct {
	Filename   string   `toml:"filename"`
	Label      string   `toml:"label"`
	Highlights []string `toml:"highlights"`
}

type WebTermConfig struct {
	Path    string   `toml:"path"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Dir     string   `toml:"dir"`
}

type WebSSHConfig struct {
	Path     string      `toml:"path"`
	Host     string      `toml:"host"`
	Port     int         `toml:"port"`
	User     string      `toml:"user"`
	Password string      `toml:"password"`
	Keyfile  string      `toml:"keyfile"`
	Via      []WebSSHVia `toml:"via"`
	Command  string      `toml:"command"`
}

type WebSSHVia struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Keyfile  string `toml:"keyfile"`
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
	ID       string        `toml:"id"`
	Title    string        `toml:"title"`
	Interval time.Duration `toml:"interval"`
	MaxCount int           `toml:"length"`
}

type FilterConfig struct {
	Includes []string `toml:"includes"`
	Excludes []string `toml:"excludes"`
}

//go:embed "metrical.toml"
var configContent string

//go:embed static/*
var staticFS embed.FS

func main() {
	var configFilename string
	var genConfigFilename string

	flag.StringVar(&configFilename, "config", "", "metrical config file path")
	flag.StringVar(&genConfigFilename, "gen-config", "", "Generates default config to the given filename")
	flag.Parse()

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

	logWriter := io.Discard
	if mc.Log.Stdout {
		logWriter = os.Stdout
	}
	if mc.Log.Filename != "" {
		logFile, err := os.OpenFile(mc.Log.Filename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(fmt.Sprintf("Failed to open log file %s: %v", mc.Log.Filename, err))
		}
		if mc.Log.Stdout {
			logWriter = io.MultiWriter(logWriter, logFile)
		} else {
			logWriter = logFile
		}
	}

	logHandler := slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: mc.Log.Level.Level()})
	slog.SetDefault(slog.New(logHandler))

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
		fileSvrFS := http.FileServerFS(staticFS)
		mux := http.NewServeMux()
		for _, cfg := range mc.Http.Dashboard {
			if path := cfg.Path; path != "" {
				path = strings.TrimSuffix(path, "/") + "/"
				mux.Handle(path, mc.makeDashboard())
				slog.Info("- Dashboard " + mc.Http.AdvAddr + path)
			}
		}
		for _, cfg := range mc.Http.Tails {
			path := strings.TrimSuffix(cfg.Path, "/") + "/"
			for i, v := range cfg.Files {
				v.Filename = strings.ReplaceAll(v.Filename, "~", os.Getenv("HOME"))
				v.Filename = strings.ReplaceAll(v.Filename, "${log-filename}", mc.Log.Filename)
				cfg.Files[i] = v
			}
			mux.Handle(path, mc.makeTail(path, cfg.Files))
			slog.Info("- Tail " + mc.Http.AdvAddr + path)
		}
		for _, cfg := range mc.Http.Terms {
			path := strings.TrimSuffix(cfg.Path, "/") + "/"
			mux.Handle(path, mc.makeTerminal(path, cfg.Command, cfg.Args, cfg.Dir))
			slog.Info("- Term " + mc.Http.AdvAddr + path)
		}
		for _, cfg := range mc.Http.SSHs {
			path := strings.TrimSuffix(cfg.Path, "/") + "/"
			via := append(cfg.Via, WebSSHVia{
				Host:     cfg.Host,
				Port:     cfg.Port,
				User:     cfg.User,
				Password: cfg.Password,
				Keyfile:  cfg.Keyfile,
			})
			mux.Handle(path, mc.makeSSH(path, via, cfg.Command))
			slog.Info("- SSH " + mc.Http.AdvAddr + path)
		}
		mux.Handle("/static/", fileSvrFS)
		mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "static/favicon.ico")
		})
		mux.Handle("/debug/pprof", pprof.Handler("/debug/pprof"))
		svr := &http.Server{
			Addr:      mc.Http.Listen,
			Handler:   httpstat.NewHandler(mc.Collector.C, mux),
			ConnState: connState,
		}
		defer svr.Close()
		go func() {
			slog.Info("Starting HTTP server " + mc.Http.AdvAddr + " ...")
			if err := svr.ListenAndServe(); err != nil {
				if err == http.ErrServerClosed {
					slog.Info("HTTP server closed")
				} else {
					slog.Error("Error starting HTTP server", "error", err)
				}
			}
		}()
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
	fmt.Fprintln(fd, "# This is the sample configuration file for metrical.")
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
		seriesID, err := metric.NewSeriesID(ts.ID, ts.Title, ts.Interval, ts.MaxCount)
		if err != nil {
			return fmt.Errorf("invalid series ID %q: %w", ts.ID, err)
		}
		options = append(options, metric.WithSeries(seriesID))
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

func (mc *Metrical) makeDashboard() *metric.Dashboard {
	dash := metric.NewDashboard(mc.Collector)
	dash.PageTitle = "Metrical - Demo"
	dash.ShowRemains = false
	dash.Option.JsSrc = []string{"/static/js/echarts.min.js"}
	dash.SetTheme("light")
	dash.SetPanelHeight("280px")   // default
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
	if mc.HasInput("net") {
		dash.AddChart(metric.Chart{Title: "Network I/O", MetricNames: []string{"net:*:bytes_recv", "net:*:bytes_sent"}, FieldNames: []string{"abs_diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Packets", MetricNames: []string{"net:*:packets_recv", "net:*:packets_sent"}, FieldNames: []string{"non_negative_diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Network Errors", MetricNames: []string{"net:*:drop_in", "net:*:drop_out", "net:*:err_in", "net:*:err_out"}, FieldNames: []string{"non_negative_diff"}, Type: metric.ChartTypeScatter, ShowSymbol: true})
	}
	if mc.HasInput("netstat") {
		dash.AddChart(metric.Chart{Title: "Netstat", MetricNames: []string{"netstat:tcp_*", "netstat:udp_*"}, FieldNames: []string{"last"}})
	}
	if mc.HasInput("diskio") {
		dash.AddChart(metric.Chart{Title: "Disk I/O Bytes", MetricNames: []string{"diskio:*:read_bytes", "diskio:*:write_bytes"}, FieldNames: []string{"non_negative_diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Disk I/O Count", MetricNames: []string{"diskio:*:read_count", "diskio:*:write_count"}, FieldNames: []string{"non_negative_diff"}, Type: metric.ChartTypeLine})
		dash.AddChart(metric.Chart{Title: "Disk I/O Time", MetricNames: []string{"diskio:*:read_time", "diskio:*:write_time", "diskio:*:io_time", "diskio:*:weighted_io_time"}, FieldNames: []string{"non_negative_diff"}, Type: metric.ChartTypeLine})
	}
	dash.AddChart(metric.Chart{Title: "HTTP Latency", MetricNames: []string{"http:latency"}, FieldNames: []string{"p50", "p90", "p99"}})
	dash.AddChart(metric.Chart{Title: "HTTP I/O", MetricNames: []string{"http:bytes_recv", "http:bytes_sent"}, Type: metric.ChartTypeLine, ShowSymbol: false})
	dash.AddChart(metric.Chart{Title: "HTTP Status", MetricNames: []string{"http:status_[1-5]xx"}, Type: metric.ChartTypeBarStack})
	return dash
}

func (mc *Metrical) makeTail(cutPrefix string, files []WebTailFile) http.Handler {
	tc := []webtail.TailConfig{}
	for _, f := range files {
		tc = append(tc, webtail.TailConfig{
			Filename:   f.Filename,
			Label:      f.Label,
			Highlights: f.Highlights,
		})
	}

	return webterm.New(
		&webtail.WebTail{Tails: tc},
		webterm.WithCutPrefix(cutPrefix),
		webterm.WithTheme(webterm.ThemeDefault),
		webterm.WithFontSize(11),
		webterm.WithLocalization(map[string]string{
			"Log Viewer":           "Metrical Logs",
			"All Logs":             "All Log Files",
			"No Logs":              "No Log Files",
			"Enter filter text...": "Filter text...",
			"Apply":                "Apply",
			"Clear":                "Reset",
		}),
	)
}

func (mc *Metrical) makeTerminal(cutPrefix string, cmd string, args []string, dir string) http.Handler {
	term := webterm.New(
		&webexec.WebExec{
			Command: cmd,
			Args:    args,
			Dir:     dir,
		},
		webterm.WithCutPrefix(cutPrefix),
		webterm.WithFontSize(11),
		webterm.WithTheme(webterm.ThemeDracula),
	)
	return term
}

func (mc *Metrical) makeSSH(cutPrefix string, vai []WebSSHVia, cmd string) http.Handler {
	hops := webssh.Hops{}
	for _, v := range vai {
		auth := []ssh.AuthMethod{}
		if v.Password != "" {
			auth = append(auth, webssh.AuthPassword(v.Password))
		}
		if v.Keyfile != "" {
			key, _ := os.ReadFile(v.Keyfile)
			auth = append(auth, webssh.AuthPrivateKey(key))
		}
		hop := webssh.Hop{
			Host: v.Host,
			Port: v.Port,
			User: v.User,
			Auth: auth,
		}
		hops = append(hops, hop)
	}

	term := webterm.New(
		&webssh.WebSSH{
			Hops:     hops,
			TermType: "xterm-256color",
			Command:  cmd,
		},
		webterm.WithCutPrefix(cutPrefix),
		webterm.WithFontSize(11),
		webterm.WithTheme(webterm.ThemeMolokai),
	)
	return term
}
