package svg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OutOfBedlam/metric"
)

type SVGOutput struct {
	DstDir string
}

func (s *SVGOutput) Export(metricName string, ss *metric.Snapshot) error {
	dstFile := filepath.Join(s.DstDir, fmt.Sprintf("%s.svg", strings.ReplaceAll(metricName, ":", "_")))
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	canvas := CanvasWithSnapshot(ss)
	canvas.XMLHeader = true
	if err := canvas.Export(out); err != nil {
		panic(fmt.Errorf("failed to generate SVG: %v", err))
	}
	out.Close()
	return nil
}

func CanvasWithSnapshot(ss *metric.Snapshot) *Canvas {
	values := make([]float64, len(ss.Values))
	minValues := make([]float64, len(ss.Values))
	maxValues := make([]float64, len(ss.Values))
	last := ss.Values[len(ss.Values)-1]
	lastValue := 0.0
	if c, ok := last.(*metric.CounterValue); ok {
		lastValue = c.Value
		for i, v := range ss.Values {
			if v == nil {
				continue
			}
			values[i] = v.(*metric.CounterValue).Value
		}
	} else if g, ok := last.(*metric.GaugeValue); ok {
		lastValue = g.Value
		for i, v := range ss.Values {
			if v == nil {
				continue
			}
			values[i] = v.(*metric.GaugeValue).Value
		}
	} else if m, ok := last.(*metric.MeterValue); ok {
		lastValue = m.Last
		for i, val := range ss.Values {
			if val == nil {
				continue
			}
			v := val.(*metric.MeterValue)
			if v.Samples > 0 {
				values[i] = v.Sum / float64(v.Samples)
			}
			minValues[i] = v.Min
			maxValues[i] = v.Max
		}
	} else if h, ok := last.(*metric.HistogramValue); ok {
		lastValue = h.Values[0]
		for i, val := range ss.Values {
			if val == nil {
				continue
			}
			v := val.(*metric.HistogramValue)
			values[i] = v.Values[len(v.Values)/2]
			minValues[i] = v.Values[0]
			maxValues[i] = v.Values[len(v.Values)-1]
		}
	}
	svg := NewCanvas(200, 80)
	if meta, ok := ss.Meta.(metric.FieldInfo); ok {
		svg.Title = fmt.Sprintf("%s %s", meta.Name, meta.Series)
		svg.Value = meta.Unit.Format(lastValue, 1)
		if meta.Unit == metric.UnitPercent {
			svg.GridYMin = 0
			svg.GridYMax = 100
		} else {
			svg.GridYMargin = 0.5
		}
		svg.ValueUnit = meta.Unit
	}
	svg.GridXInterval = ss.Interval
	svg.GridXMaxCount = ss.MaxCount
	svg.ShowXAxisLabels = true
	svg.ShowYAxisLabels = true
	svg.Times = ss.Times
	svg.Values = values
	svg.MinValues = minValues
	svg.MaxValues = maxValues

	return svg
}
