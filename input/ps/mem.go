package ps

import (
	_ "embed"
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/mem"
)

func init() {
	registry.Register("input.mem", (*Memory)(nil))
}

//go:embed "mem.toml"
var memSampleConfig string

func (ms *Memory) SampleConfig() string {
	return memSampleConfig
}

type Memory struct {
	Type string `toml:"type"` // e.g. "gauge", "meter"(default)

	metricPercentType metric.Type `toml:"-"`
	metricBytesType   metric.Type `toml:"-"`
}

var _ metric.Input = (*Memory)(nil)

func (ms *Memory) Init() error {
	switch ms.Type {
	case "meter":
		ms.metricPercentType = metric.MeterType(metric.UnitPercent)
		ms.metricBytesType = metric.MeterType(metric.UnitBytes)
	default:
		ms.metricPercentType = metric.GaugeType(metric.UnitPercent)
		ms.metricBytesType = metric.GaugeType(metric.UnitBytes)
	}
	return nil
}

func (ms *Memory) Gather(g *metric.Gather) error {
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("error collecting memory percent: %w", err)
	}
	g.Add("mem:percent", memStat.UsedPercent, ms.metricPercentType)
	return nil
}
