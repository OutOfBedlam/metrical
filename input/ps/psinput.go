package ps

import (
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type CPU struct {
	Type       string      `toml:"type"` // e.g. "gauge", "meter"(default)
	metricType metric.Type `toml:"-"`
}

func (c *CPU) Init() error {
	switch c.Type {
	case "meter":
		c.metricType = metric.MeterType(metric.UnitPercent)
	default:
		c.metricType = metric.GaugeType(metric.UnitPercent)
	}
	return nil
}

func (c *CPU) Gather(g metric.Gather) {
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		g.AddError(fmt.Errorf("error collecting CPU percent: %w", err))
		return
	}

	m := metric.Measurement{Name: "cpu"}
	m.AddField(metric.Field{
		Name:  "percent",
		Value: cpuPercent[0],
		Type:  c.metricType,
	})
	g.AddMeasurement(m)
}

type Memory struct {
	Type       string      `toml:"type"` // e.g. "gauge", "meter"(default)
	metricType metric.Type `toml:"-"`
}

func (ms *Memory) Init() error {
	switch ms.Type {
	case "meter":
		ms.metricType = metric.MeterType(metric.UnitPercent)
	default:
		ms.metricType = metric.GaugeType(metric.UnitPercent)
	}
	return nil
}

func (ms *Memory) Gather(g metric.Gather) {
	memStat, err := mem.VirtualMemory()
	if err != nil {
		g.AddError(fmt.Errorf("error collecting memory percent: %w", err))
		return
	}
	m := metric.Measurement{Name: "mem"}
	m.AddField(metric.Field{
		Name:  "percent",
		Value: memStat.UsedPercent,
		Type:  ms.metricType,
	})
	g.AddMeasurement(m)
}
