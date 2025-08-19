package ps

import (
	"fmt"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

var _ metric.InputFunc = Collect

const CPU_PERCENT = "cpu_percent"
const MEM_PERCENT = "mem_percent"

func Collect() (metric.Measurement, error) {
	m := metric.Measurement{Name: "ps"}

	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return m, fmt.Errorf("error collecting CPU percent: %w", err)
	}
	m.Fields = append(m.Fields, metric.Field{
		Name:  CPU_PERCENT,
		Value: cpuPercent[0],
		Unit:  metric.UnitPercent,
		Type:  metric.FieldTypeMeter,
	})

	memStat, err := mem.VirtualMemory()
	if err != nil {
		return m, fmt.Errorf("error collecting memory percent: %w", err)
	}
	m.Fields = append(m.Fields, metric.Field{
		Name:  MEM_PERCENT,
		Value: memStat.UsedPercent,
		Unit:  metric.UnitPercent,
		Type:  metric.FieldTypeMeter,
	})
	return m, nil
}
