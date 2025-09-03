package gostat

import (
	"runtime"

	"github.com/OutOfBedlam/metric"
)

type Runtime struct {
}

const HeapInuse = "heap_inuse"
const GoRoutines = "goroutines"

func (gr Runtime) Collect() (metric.Measurement, error) {
	m := metric.Measurement{Name: "runtime"}

	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	gorutine := runtime.NumGoroutine()
	m.Fields = []metric.Field{
		{
			Name:  HeapInuse,
			Value: float64(memStats.HeapInuse),
			Type:  metric.GaugeType(metric.UnitBytes),
		},
		{
			Name:  GoRoutines,
			Value: float64(gorutine),
			Type:  metric.MeterType(metric.UnitShort),
		},
	}
	return m, nil
}
