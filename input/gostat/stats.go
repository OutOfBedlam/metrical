package gostat

import (
	_ "embed"
	"runtime"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
)

func init() {
	registry.Register("input.go_mem_stats", (*HeapInuse)(nil))
	registry.Register("input.go_runtime", (*GoRoutines)(nil))
}

//go:embed "runtime.toml"
var runtimeSampleConfig string

func (n *HeapInuse) SampleConfig() string {
	return runtimeSampleConfig
}

type HeapInuse struct {
	Type       string      `toml:"type"` // e.g. "meter", "gauge"(default)
	metricType metric.Type `toml:"-"`
}

func (hi *HeapInuse) Init() error {
	switch hi.Type {
	case "meter":
		hi.metricType = metric.MeterType(metric.UnitBytes)
	default:
		hi.metricType = metric.GaugeType(metric.UnitBytes)
	}
	return nil
}

func (hi *HeapInuse) Gather(g *metric.Gather) {
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	g.Add("go_mem_stats:heap_inuse", float64(memStats.HeapInuse), hi.metricType)
}

type GoRoutines struct {
	Type       string      `toml:"type"` // e.g. "gauge", "meter"(default)
	metricType metric.Type `toml:"-"`
}

func (gr *GoRoutines) Init() error {
	switch gr.Type {
	case "meter":
		gr.metricType = metric.MeterType(metric.UnitShort)
	default:
		gr.metricType = metric.GaugeType(metric.UnitShort)
	}
	return nil
}

func (gr *GoRoutines) Gather(g *metric.Gather) {
	gorutine := runtime.NumGoroutine()
	g.Add("go_runtime:goroutines", float64(gorutine), gr.metricType)
}
