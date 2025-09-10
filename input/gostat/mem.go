package gostat

import (
	_ "embed"
	"runtime"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
)

func init() {
	registry.Register("input.go_mem", (*HeapInuse)(nil))
}

//go:embed "mem.toml"
var go_memSampleConfig string

func (n *HeapInuse) SampleConfig() string {
	return go_memSampleConfig
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

func (hi *HeapInuse) Gather(g *metric.Gather) error {
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	g.Add("go:mem:heap_inuse", float64(memStats.HeapInuse), hi.metricType)
	return nil
}
