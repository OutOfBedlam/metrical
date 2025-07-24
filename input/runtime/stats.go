package runtime

import goruntime "runtime"

type Stats struct {
}

func (m *Stats) Name() string {
	return "runtime"
}

const HeapInuse = "heap_inuse"
const GoRoutines = "goroutines"

func (m *Stats) Field(field string) (string, string) {
	switch field {
	case HeapInuse:
		return "HeapInUse", ""
	case GoRoutines:
		return "Go Routines", ""
	default:
		return "", ""
	}
}

func (m *Stats) Collect() (map[string]float64, error) {
	s := goruntime.MemStats{}
	goruntime.ReadMemStats(&s)
	gorutine := goruntime.NumGoroutine()
	return map[string]float64{
		HeapInuse:  float64(s.HeapInuse), // title: "Heap Inuse", unit: "bytes",
		GoRoutines: float64(gorutine),    // title: "Goroutines", unit: "short",
	}, nil
}
