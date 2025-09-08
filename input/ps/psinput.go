package ps

import (
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type PS struct {
}

var _ metric.Input = (*PS)(nil)

const CPU_PERCENT = "cpu_percent"
const MEM_PERCENT = "mem_percent"

func (ps *PS) Init() error {
	return nil
}

func (ps *PS) Gather(g metric.Gather) {
	m := metric.Measurement{Name: "ps"}

	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		g.AddError(fmt.Errorf("error collecting CPU percent: %w", err))
		return
	}
	m.Fields = append(m.Fields, metric.Field{
		Name:  CPU_PERCENT,
		Value: cpuPercent[0],
		Type:  metric.MeterType(metric.UnitPercent),
	})

	memStat, err := mem.VirtualMemory()
	if err != nil {
		g.AddError(fmt.Errorf("error collecting memory percent: %w", err))
		return
	}
	m.Fields = append(m.Fields, metric.Field{
		Name:  MEM_PERCENT,
		Value: memStat.UsedPercent,
		Type:  metric.MeterType(metric.UnitPercent),
	})
	g.AddMeasurement(m)
}
