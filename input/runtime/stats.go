package runtime

import (
	goruntime "runtime"

	"github.com/OutOfBedlam/metric"
)

var _ metric.InputFunc = Collect

const HeapInuse = "heap_inuse"
const GoRoutines = "goroutines"

func Collect() (metric.Measurement, error) {
	m := metric.Measurement{Name: "runtime"}

	memStats := goruntime.MemStats{}
	goruntime.ReadMemStats(&memStats)
	gorutine := goruntime.NumGoroutine()
	m.Fields = []metric.Field{
		{
			Name:  HeapInuse,
			Value: float64(memStats.HeapInuse),
			Unit:  metric.UnitBytes,
			Type:  metric.FieldTypeGauge,
		},
		{
			Name:  GoRoutines,
			Value: float64(gorutine),
			Unit:  metric.UnitShort,
			Type:  metric.FieldTypeMeter,
		},
	}
	return m, nil
}
