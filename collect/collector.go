package collect

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/OutOfBedlam/metric"
)

type Input interface {
	Name() string
	Field(field string) (string, string)
	Collect() (map[string]float64, error)
}

type InputWrapper struct {
	input         Input
	mtsFields     map[string]metric.MultiTimeSeries[float64]
	publishedName string
}

type Collector struct {
	iwsMutex sync.Mutex
	iws      []InputWrapper
	interval time.Duration
	closeCh  chan struct{}
}

func NewCollector(interval time.Duration) *Collector {
	c := &Collector{
		interval: interval,
		closeCh:  make(chan struct{}),
	}
	return c
}

func (c *Collector) AddInput(input Input) {
	c.iwsMutex.Lock()
	defer c.iwsMutex.Unlock()
	iw := InputWrapper{
		input:     input,
		mtsFields: make(map[string]metric.MultiTimeSeries[float64]),
	}
	c.iws = append(c.iws, iw)
}

func (c *Collector) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		for {
			select {
			case tm := <-ticker.C:
				c.runMeasures(tm)
			case <-c.closeCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (c *Collector) Stop() {
	close(c.closeCh)
}

func (c *Collector) runMeasures(tm time.Time) {
	c.iwsMutex.Lock()
	defer c.iwsMutex.Unlock()

	for _, iw := range c.iws {
		measure, err := iw.input.Collect()
		if err != nil {
			fmt.Printf("Error measuring: %v\n", err)
			continue
		}
		inputName := iw.input.Name()
		for field, value := range measure {
			var mts metric.MultiTimeSeries[float64]
			if m, exists := iw.mtsFields[field]; exists {
				mts = m
			} else {
				title, unit := iw.input.Field(field)
				if title == "" {
					title = field
				}
				mts = metric.MultiTimeSeries[float64]{
					metric.NewTimeSeries[float64](2*time.Second, 60, metric.AVG),
					metric.NewTimeSeries[float64](5*time.Minute, 60, metric.AVG),
					metric.NewTimeSeries[float64](15*time.Minute, 60, metric.AVG),
				}
				mts[0].SetMeta(&metric.TimeSeriesMeta{Title: fmt.Sprintf("%s 2 min.", title), Unit: unit})
				mts[1].SetMeta(&metric.TimeSeriesMeta{Title: fmt.Sprintf("%s 5 hours", title), Unit: unit})
				mts[2].SetMeta(&metric.TimeSeriesMeta{Title: fmt.Sprintf("%s 15 hours", title), Unit: unit})
				iw.mtsFields[field] = mts
				iw.publishedName = fmt.Sprintf("metrical:%s:%s", inputName, field)
				expvar.Publish(iw.publishedName, mts)
			}
			mts.AddTime(tm, value)
		}
	}
}
