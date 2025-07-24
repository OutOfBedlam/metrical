package ps

import (
	"fmt"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type PSInput struct{}

const CPU_PERCENT = "cpu_percent"
const MEM_PERCENT = "mem_percent"

func (p *PSInput) Name() string {
	return "ps"
}

func (p *PSInput) Field(field string) (string, string) {
	switch field {
	case CPU_PERCENT:
		return "CPU", "%"
	case MEM_PERCENT:
		return "Memory", "%"
	default:
		return "", ""
	}
}

func (p *PSInput) Collect() (map[string]float64, error) {
	m := map[string]float64{}
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return nil, fmt.Errorf("error collecting CPU percent: %w", err)
	}
	m[CPU_PERCENT] = cpuPercent[0]

	memStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("error collecting memory percent: %w", err)
	}
	m[MEM_PERCENT] = memStat.UsedPercent

	return m, nil
}
