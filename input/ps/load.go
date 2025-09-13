package ps

import (
	_ "embed"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/load"
)

func init() {
	registry.Register("load", (*Load)(nil))
}

//go:embed "load.toml"
var loadSampleConfig string

func (l *Load) SampleConfig() string {
	return loadSampleConfig
}

var _ metric.Input = (*Load)(nil)

type Load struct {
}

func (l *Load) Gather(g *metric.Gather) error {
	stat, err := load.Avg()
	if err != nil {
		return err
	}
	g.Add("load:load1", stat.Load1, metric.GaugeType(metric.UnitShort))
	g.Add("load:load5", stat.Load5, metric.GaugeType(metric.UnitShort))
	g.Add("load:load15", stat.Load15, metric.GaugeType(metric.UnitShort))
	return nil
}
