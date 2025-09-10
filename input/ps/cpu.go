package ps

import (
	_ "embed"
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/cpu"
)

func init() {
	registry.Register("input.cpu", (*CPU)(nil))
}

//go:embed "cpu.toml"
var cpuSampleConfig string

func (c *CPU) SampleConfig() string {
	return cpuSampleConfig
}

type CPU struct {
	Type       string      `toml:"type"`
	PerCPU     bool        `toml:"per_cpu"`
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

func (c *CPU) Gather(g *metric.Gather) error {
	cpuPercent, err := cpu.Percent(0, c.PerCPU)
	if err != nil {
		return fmt.Errorf("error collecting CPU percent: %w", err)
	}

	if c.PerCPU {
		for i, p := range cpuPercent {
			g.Add(fmt.Sprintf("cpu:cpu_%d", i), p, c.metricType)
		}
	} else {
		g.Add("cpu:cpu_all", cpuPercent[0], c.metricType)
	}
	return nil
}
