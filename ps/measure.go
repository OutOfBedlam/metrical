package ps

import (
	"fmt"

	"github.com/OutOfBedlam/metric"
)

type Measure struct {
	CpuPercent float64
	MemPercent float64
}

func (m *Measure) String() string {
	return fmt.Sprintf("{cpu:%.1f%%,mem:%.1f%%}", m.CpuPercent, m.MemPercent)
}

func avgMeasure(l, r metric.TimePoint[*Measure]) metric.TimePoint[*Measure] {
	ts := r.Time
	if l.Time.After(r.Time) { // Ensure l is after r
		ts = l.Time
	}
	totalCount := l.Count + r.Count
	if totalCount == 0 {
		return metric.TimePoint[*Measure]{Time: ts}
	}
	cpu := ((float64(l.Value.CpuPercent) * float64(l.Count)) + (float64(r.Value.CpuPercent) * float64(r.Count))) / float64(totalCount)
	mem := ((float64(l.Value.MemPercent) * float64(l.Count)) + (float64(r.Value.MemPercent) * float64(r.Count))) / float64(totalCount)

	l.Time = ts
	l.Count = totalCount
	l.Value.CpuPercent = cpu
	l.Value.MemPercent = mem
	return l
}

func maxMeasure(l, r metric.TimePoint[*Measure]) metric.TimePoint[*Measure] {
	ts := r.Time
	if l.Time.After(r.Time) { // Ensure l is after r
		ts = l.Time
	}
	l.Time = ts
	l.Count += r.Count
	l.Value.CpuPercent = max(l.Value.CpuPercent, r.Value.CpuPercent)
	l.Value.MemPercent = max(l.Value.MemPercent, r.Value.MemPercent)
	return l
}
