package collect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/OutOfBedlam/metric"
)

type Measure struct {
	Fields map[string]float64
}

func NewMeasure() *Measure {
	return &Measure{
		Fields: make(map[string]float64),
	}
}

func (m Measure) Set(name string, value float64) {
	if m.Fields == nil {
		m.Fields = make(map[string]float64)
	}
	m.Fields[name] = value
}

func (m Measure) Get(name string) (float64, bool) {
	value, exists := m.Fields[name]
	return value, exists
}

func (m *Measure) String() string {
	names := make([]string, 0, len(m.Fields))
	for name := range m.Fields {
		names = append(names, name)
	}
	slices.Sort(names)
	sb := &strings.Builder{}
	sb.WriteString("{")
	for i, name := range names {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s:%v", name, m.Fields[name]))
	}
	sb.WriteString("}")
	return sb.String()
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
	names := make([]string, 0, len(l.Value.Fields))
	for name := range l.Value.Fields {
		names = append(names, name)
	}
	for name := range r.Value.Fields {
		if _, exists := l.Value.Fields[name]; !exists {
			names = append(names, name)
		}
	}

	for _, name := range names {
		lValue, lExists := l.Value.Fields[name]
		rValue, rExists := r.Value.Fields[name]
		if !lExists {
			l.Value.Set(name, rValue)
		}
		if !rExists {
			continue
		}
		avgValue := ((lValue * float64(l.Count)) + (rValue * float64(r.Count))) / float64(totalCount)
		l.Value.Set(name, avgValue)
	}
	l.Time = ts
	l.Count = totalCount
	return l
}

func maxMeasure(l, r metric.TimePoint[*Measure]) metric.TimePoint[*Measure] {
	ts := r.Time
	if l.Time.After(r.Time) { // Ensure l is after r
		ts = l.Time
	}
	l.Time = ts
	l.Count += r.Count

	names := make([]string, 0, len(l.Value.Fields))
	for name := range l.Value.Fields {
		names = append(names, name)
	}
	for name := range r.Value.Fields {
		if _, exists := l.Value.Fields[name]; !exists {
			names = append(names, name)
		}
	}
	for _, name := range names {
		lValue, lExists := l.Value.Fields[name]
		rValue, rExists := r.Value.Fields[name]
		if !lExists {
			l.Value.Set(name, rValue)
		}
		if !rExists {
			continue
		}
		// Use max for the value
		if lValue < rValue {
			l.Value.Set(name, rValue)
		}
	}
	return l
}
