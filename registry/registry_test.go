package registry

import (
	"testing"

	"github.com/OutOfBedlam/metric"
	"github.com/stretchr/testify/require"
)

type CPUMock struct {
	Measure string `toml:"measure"`
}

func (c *CPUMock) Init() error {
	return nil
}

func (c *CPUMock) Gather(g metric.Gather) {
	g.Add("cpu:"+c.Measure, 10, metric.MeterType(metric.UnitPercent))
}

type MEMMock struct {
	Measure string `toml:"measure"`
}

func (m *MEMMock) Init() error {
	return nil
}

func (m *MEMMock) Gather(g metric.Gather) {
	g.Add("mem:"+m.Measure, 20, metric.GaugeType(metric.UnitBytes))
}

func TestConfig(t *testing.T) {
	Register("cpu", (*CPUMock)(nil))
	Register("mem", (*MEMMock)(nil))

	tests := []struct {
		name    string
		content string
		expect  []string
		wantErr bool
	}{
		{
			name: "valid config",
			content: `
				[[input.cpu]]
					measure = "percent"
				[[input.mem]]
					measure = "stack"
				[[input.mem]]
					measure = "heap"
				`,
			expect: []string{"cpu:percent", "mem:stack", "mem:heap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := metric.NewCollector()
			if err := LoadConfig(c, tt.content); (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				var inputNames = c.MetricNames()
				require.EqualValues(t, tt.expect, inputNames)
			}
		})
	}
}
