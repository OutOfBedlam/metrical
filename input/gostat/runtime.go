package gostat

import (
	_ "embed"
	"runtime"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
)

func init() {
	registry.Register("input.go_runtime", (*GoRoutines)(nil))
}

//go:embed "runtime.toml"
var runtimeSampleConfig string

func (n *GoRoutines) SampleConfig() string {
	return runtimeSampleConfig
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

func (gr *GoRoutines) Gather(g *metric.Gather) error {
	gorutine := runtime.NumGoroutine()
	g.Add("go_runtime:goroutines", float64(gorutine), gr.metricType)
	return nil
}
