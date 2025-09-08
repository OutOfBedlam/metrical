package gostat

import (
	"runtime"

	"github.com/OutOfBedlam/metric"
)

type Runtime struct {
	HeapInuse struct {
		Enabled    bool        `toml:"enable"`
		Type       string      `toml:"type"` // e.g. "meter", "gauge"(default)
		metricType metric.Type `toml:"-"`
	} `toml:"heap_inuse"`
	GoRoutines struct {
		Enabled    bool        `toml:"enable"`
		Type       string      `toml:"type"` // e.g. "gauge", "meter"(default)
		metricType metric.Type `toml:"-"`
	} `toml:"goroutines"`
}

var _ metric.Input = (*Runtime)(nil)

func (gr *Runtime) Init() error {
	gr.HeapInuse.Enabled = true
	gr.HeapInuse.Type = "gauge"
	gr.GoRoutines.Enabled = true
	gr.GoRoutines.Type = "meter"
	return nil
}

const RegisterName = "runtime"
const HeapInuse = "heap_inuse"
const GoRoutines = "goroutines"

func (gr *Runtime) Gather(g metric.Gather) {
	m := metric.Measurement{Name: RegisterName}

	if gr.HeapInuse.Enabled {
		if gr.HeapInuse.metricType.Empty() {
			switch gr.HeapInuse.Type {
			case "meter":
				gr.HeapInuse.metricType = metric.MeterType(metric.UnitBytes)
			default:
				gr.HeapInuse.metricType = metric.GaugeType(metric.UnitBytes)
			}
		}
		memStats := runtime.MemStats{}
		runtime.ReadMemStats(&memStats)
		m.AddField(metric.Field{
			Name:  HeapInuse,
			Value: float64(memStats.HeapInuse),
			Type:  gr.HeapInuse.metricType,
		})
	}

	if gr.GoRoutines.Enabled {
		if gr.GoRoutines.metricType.Empty() {
			switch gr.GoRoutines.Type {
			case "gauge":
				gr.GoRoutines.metricType = metric.GaugeType(metric.UnitShort)
			default:
				gr.GoRoutines.metricType = metric.MeterType(metric.UnitShort)
			}
		}
		gorutine := runtime.NumGoroutine()
		m.AddField(metric.Field{
			Name:  GoRoutines,
			Value: float64(gorutine),
			Type:  gr.GoRoutines.metricType,
		})
	}
	g.AddMeasurement(m)
}
