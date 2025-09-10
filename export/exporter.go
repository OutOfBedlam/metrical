package export

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/OutOfBedlam/metric"
)

type Output interface {
	Export(name string, times []time.Time, values []metric.Value, fnfo metric.SeriesInfo, interval time.Duration, maxCount int) error
}

type OutputWrapper struct {
	output Output
	filter func(string) bool
}

type Exporter struct {
	sync.Mutex
	ows       []OutputWrapper
	metrics   []string
	interval  time.Duration
	closeCh   chan struct{}
	latestErr error
}

// Example:
//
// exporter := export.NewExporter(1*time.Second, mc.Collector.PublishNames())
// exporter.AddOutput(&svg.SVGOutput{DstDir: exportDir}, nil)
// exporter.Start()
// defer exporter.Stop()
func NewExporter(interval time.Duration, metrics []string) *Exporter {
	return &Exporter{
		interval: interval,
		metrics:  metrics,
		closeCh:  make(chan struct{}),
	}
}

func (s *Exporter) AddOutput(output Output, filter any) {
	s.Lock()
	defer s.Unlock()
	ow := OutputWrapper{
		output: output,
		filter: func(string) bool { return true }, // Default filter allows all metrics
	}
	s.ows = append(s.ows, ow)
}

func (s *Exporter) Start() {
	ticker := time.NewTicker(s.interval)
	go func() {
		for {
			select {
			case <-s.closeCh:
				ticker.Stop()
				return
			case <-ticker.C:
				if err := s.exportAll(0); err != nil {
					s.latestErr = err
				}
			}
		}
	}()
}

func (s *Exporter) Stop() {
	if s.closeCh == nil {
		return
	}
	close(s.closeCh)
	s.closeCh = nil
}

// Err returns the latest error encountered during export.
// If no error has occurred, it returns nil.
func (s *Exporter) Err() error {
	return s.latestErr
}

func (s *Exporter) exportAll(tsIdx int) error {
	for _, metricName := range s.metrics {
		if err := s.Export(metricName, tsIdx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Exporter) Export(metricName string, tsIdx int) error {
	for _, ow := range s.ows {
		if !ow.filter(metricName) {
			continue
		}
		ts, err := getTimeseries(metricName, tsIdx)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("%s:%d", metricName, tsIdx)
		times, values := ts.LastN(0)
		if len(times) == 0 {
			// If the metric is nil or has no values, skip
			continue
		}
		interval := ts.Interval()
		maxCount := ts.MaxCount()
		fnfo := ts.Meta().(metric.SeriesInfo)
		// Export the data using the output
		if err := ow.output.Export(name, times, values, fnfo, interval, maxCount); err != nil {
			return err
		}
	}
	return nil
}

func getTimeseries(metricName string, idx int) (*metric.TimeSeries, error) {
	if ev := expvar.Get(metricName); ev != nil {
		mts, ok := ev.(metric.MultiTimeSeries)
		if !ok {
			return nil, fmt.Errorf("metric %s is not a Metric, but %T", metricName, ev)
		}
		if idx < 0 || idx >= len(mts) {
			return nil, fmt.Errorf("index %d out of range for metric %s with %d time series",
				idx, metricName, len(mts))
		}
		return mts[idx], nil
	}
	return nil, metric.ErrMetricNotFound
}
