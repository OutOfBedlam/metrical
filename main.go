package main

import (
	"expvar"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/ps"
)

func main() {
	psCollector := ps.NewCollector(1 * time.Second)
	psCollector.Start()
	defer psCollector.Stop()

	exporter := &SvgExporter{}
	exporter.Start()
	defer exporter.Stop()

	// wait signal ^C
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh
}

type SvgExporter struct {
	closeCh chan struct{}
}

func (s *SvgExporter) Start() {
	s.closeCh = make(chan struct{})
	go func() {
		for {
			select {
			case <-s.closeCh:
				return
			case <-time.After(1 * time.Second):
				ts := expvar.Get("metrical:ps:10s").(*metric.TimeSeries[*ps.Measure])
				interval := ts.Interval()
				maxCount := ts.MaxCount()
				times, measures := ts.Values()
				values := make([]float64, len(measures))
				if len(measures) == 0 {
					continue
				}
				for i, m := range measures {
					values[i] = m.CpuPercent
				}

				out, err := os.OpenFile("svg_test.svg", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					panic(err)
				}

				svg := NewSvg(200, 80)
				svg.Title = fmt.Sprintf("CPU Usage %s - %.f%%", interval, values[len(values)-1])
				svg.StrokeWidth = 1.5
				svg.GridYMin = 0
				svg.GridYMax = 100
				svg.GridMaxCount = maxCount
				if err := svg.Export(out, times, values); err != nil {
					panic(fmt.Errorf("failed to generate SVG: %v", err))
				}
				out.Close()
			}
		}
	}()
}

func (s *SvgExporter) Stop() {
	close(s.closeCh)
}
