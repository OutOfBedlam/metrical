package gostat

import (
	"runtime"

	"github.com/OutOfBedlam/metric"
)

const RegisterName = "runtime"

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

func (hi *HeapInuse) Gather(g metric.Gather) {
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	m := metric.Measurement{Name: RegisterName}
	m.AddField(metric.Field{
		Name:  "heap_inuse",
		Value: float64(memStats.HeapInuse),
		Type:  hi.metricType,
	})
	g.AddMeasurement(m)
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

func (gr *GoRoutines) Gather(g metric.Gather) {
	gorutine := runtime.NumGoroutine()
	m := metric.Measurement{Name: RegisterName}
	m.AddField(metric.Field{
		Name:  "goroutines",
		Value: float64(gorutine),
		Type:  gr.metricType,
	})
	g.AddMeasurement(m)
}
