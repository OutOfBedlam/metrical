package ps

import (
	_ "embed"
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/cpu"
)

func init() {
	registry.Register("cpu", (*CPU)(nil))
}

//go:embed "cpu.toml"
var cpuSampleConfig string

func (c *CPU) SampleConfig() string {
	return cpuSampleConfig
}

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
