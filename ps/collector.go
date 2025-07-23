package ps

import (
	"expvar"
	"fmt"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type Collector struct {
	mts      metric.MultiTimeSeries[*Measure]
	interval time.Duration
	closeCh  chan struct{}
}

func NewCollector(interval time.Duration) *Collector {
	c := &Collector{
		interval: interval,
		closeCh:  make(chan struct{}),
	}
	c.mts = append(c.mts, metric.NewTimeSeries(10*time.Second, 60, avgMeasure))
	c.mts = append(c.mts, metric.NewTimeSeries(5*time.Minute, 60, maxMeasure))
	c.mts = append(c.mts, metric.NewTimeSeries(15*time.Minute, 60, maxMeasure))
	expvar.Publish("metrical:ps:10s", c.mts[0])
	expvar.Publish("metrical:ps:5m", c.mts[1])
	expvar.Publish("metrical:ps:15m", c.mts[2])
	return c
}

func (c *Collector) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				m := c.measure()
				if m == nil {
					continue
				}
				c.mts.AddTime(time.Now(), m)
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

func (c *Collector) Print() {
	times, values := c.mts[0].Values()
	fmt.Print("[")
	for i, t := range times {
		fmt.Printf("{%s:%s}", t.Format(time.TimeOnly), values[i])
	}
	fmt.Println("]")
}

func (c *Collector) measure() *Measure {
	cpuPercent, err := cpu.Percent(c.interval, false)
	if err != nil {
		fmt.Println("Error measure CPU percent:", err)
		return nil
	}
	memStat, err := mem.VirtualMemory()
	if err != nil {
		fmt.Println("Error measure memory percent:", err)
		return nil
	}

	return &Measure{
		CpuPercent: cpuPercent[0],
		MemPercent: memStat.UsedPercent,
	}
}
