package ps

import (
	_ "embed"
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/mem"
)

func init() {
	registry.Register("mem", (*Memory)(nil))
}

//go:embed "mem.toml"
var memSampleConfig string

func (ms *Memory) SampleConfig() string {
	return memSampleConfig
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
