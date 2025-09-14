package svg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OutOfBedlam/metric"
)

type SVGOutput struct {
	DstDir string
}

type MetaProvider interface {
	Meta() any
	Interval() time.Duration
	MaxCount() int
}

func (s *SVGOutput) Export(metricName string, times []time.Time, values []metric.Value, fnfo metric.SeriesInfo, interval time.Duration, maxCount int) error {
	dstFile := filepath.Join(s.DstDir, fmt.Sprintf("%s.svg", strings.ReplaceAll(metricName, ":", "_")))
	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	canvas := CanvasWithSnapshot(times, values, fnfo, interval, maxCount)
	canvas.XMLHeader = true
	if err := canvas.Export(out); err != nil {
		panic(fmt.Errorf("failed to generate SVG: %v", err))
	}
	out.Close()
	return nil
}

func CanvasWithSnapshot(times []time.Time, values []metric.Value, field metric.SeriesInfo, interval time.Duration, maxCount int) *Canvas {
	numValues := make([]float64, len(values))
	minValues := make([]float64, len(values))
	maxValues := make([]float64, len(values))
	last := values[len(values)-1]
	lastValue := 0.0
	if c, ok := last.(*metric.CounterValue); ok {
		lastValue = c.Value
		for i, v := range values {
			if v == nil {
				continue
			}
			numValues[i] = v.(*metric.CounterValue).Value
		}
	} else if g, ok := last.(*metric.GaugeValue); ok {
		lastValue = g.Value
		for i, v := range values {
			if v == nil {
				continue
			}
			numValues[i] = v.(*metric.GaugeValue).Value
		}
	} else if m, ok := last.(*metric.MeterValue); ok {
		lastValue = m.Last
		for i, val := range values {
			if val == nil {
				continue
			}
			v := val.(*metric.MeterValue)
			if v.Samples > 0 {
				numValues[i] = v.Sum / float64(v.Samples)
			}
			minValues[i] = v.Min
			maxValues[i] = v.Max
		}
	} else if h, ok := last.(*metric.HistogramValue); ok {
		lastValue = h.Values[0]
		for i, val := range values {
			if val == nil {
				continue
			}
			v := val.(*metric.HistogramValue)
			numValues[i] = v.Values[len(v.Values)/2]
			minValues[i] = v.Values[0]
			maxValues[i] = v.Values[len(v.Values)-1]
		}
	}
	svg := NewCanvas(200, 80)
	svg.Title = fmt.Sprintf("%s %s", field.MeasureName, field.SeriesID.Title())
	svg.Value = field.MeasureType.Unit().Format(lastValue, 1)
	if field.MeasureType.Unit() == metric.UnitPercent {
		svg.GridYMin = 0
		svg.GridYMax = 100
	} else {
		svg.GridYMargin = 0.5
	}
	svg.ValueUnit = field.MeasureType.Unit()

	svg.GridXInterval = interval
	svg.GridXMaxCount = maxCount
	svg.ShowXAxisLabels = true
	svg.ShowYAxisLabels = true
	svg.Times = times
	svg.Values = numValues
	svg.MinValues = maxValues
	svg.MaxValues = maxValues

	return svg
}
