package collect

import (
	"errors"
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/OutOfBedlam/metric"
)

type ExportReq struct {
	Name  string
	Title string
	Unit  string
	Data  *metric.TimeSeriesSnapshot[float64]
}

type Output interface {
	Export(ExportReq) error
}

type OutputWrapper struct {
	output Output
	filter func(string) bool
}

type Exporter struct {
	owsMutex sync.Mutex
	ows      []OutputWrapper
	metrics  []string
	interval time.Duration
	closeCh  chan struct{}
}

func NewExporter(interval time.Duration, metrics []string) *Exporter {
	return &Exporter{
		interval: interval,
		metrics:  metrics,
		closeCh:  make(chan struct{}),
	}
}

func (s *Exporter) AddOutput(output Output, filter any) {
	s.owsMutex.Lock()
	defer s.owsMutex.Unlock()
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
				s.exportAll(0)
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

func (s *Exporter) exportAll(tsIdx int) {
	for _, metricName := range s.metrics {
		s.Export(metricName, tsIdx)
	}
}

func (s *Exporter) Export(metricName string, tsIdx int) error {
	var ss *metric.TimeSeriesSnapshot[float64]
	var meta *metric.TimeSeriesMeta
	var req ExportReq
	for _, ow := range s.ows {
		if !ow.filter(metricName) {
			continue
		}
		if ss == nil {
			var err error
			ss, meta, err = snapshot(metricName, tsIdx)
			if err != nil {
				return err
			}
			if ss == nil || len(ss.Values) == 0 {
				// If the metric is nil or has no values, skip
				break
			}
			req.Name = fmt.Sprintf("%s:%d", metricName, tsIdx)
			req.Data = ss
			if meta != nil {
				req.Title = meta.Title
				req.Unit = meta.Unit
			}
		}
		if err := ow.output.Export(req); err != nil {
			return err
		}
	}
	return nil
}

func snapshot(metricName string, idx int) (*metric.TimeSeriesSnapshot[float64], *metric.TimeSeriesMeta, error) {
	if ev := expvar.Get(metricName); ev != nil {
		mts, ok := ev.(metric.MultiTimeSeries[float64])
		if !ok {
			return nil, nil, fmt.Errorf("metric %s is not a Metric, but %T", metricName, ev)
		}
		if idx < 0 || idx >= len(mts) {
			return nil, nil, fmt.Errorf("index %d out of range for metric %s with %d time series",
				idx, metricName, len(mts))
		}
		return mts[idx].Snapshot(nil), mts[idx].Meta(), nil
	}
	return nil, nil, MetricNotFoundError
}

var MetricNotFoundError = errors.New("metric not found")
