package diskio

import (
	_ "embed"
	"fmt"
	"log/slog"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/disk"
)

func init() {
	registry.Register("diskio", (*DiskIO)(nil))
}

//go:embed "diskio.toml"
var diskioSampleConfig string

func (d *DiskIO) SampleConfig() string {
	return diskioSampleConfig
}

var _ metric.Input = (*DiskIO)(nil)

type DiskIO struct {
	DeviceFilters []string `toml:"devices"`

	devices            []string
	metricBytesType    metric.Type
	metricCountType    metric.Type
	metricDurationType metric.Type
}

func (d *DiskIO) Init() error {
	devicesFilter, err := metric.Compile(d.DeviceFilters)
	if err != nil {
		slog.Error("diskio compiling device filter", "error", err)
		return err
	}
	if stat, err := disk.IOCounters(); err != nil {
		slog.Error("diskio getting io counters", "error", err)
		return err
	} else {
		for device := range stat {
			if devicesFilter != nil && !devicesFilter.Match(device) {
				continue
			}
			d.devices = append(d.devices, device)
		}
	}
	d.metricBytesType = metric.OdometerType(metric.UnitBytes)
	d.metricCountType = metric.OdometerType(metric.UnitShort)
	d.metricDurationType = metric.OdometerType(metric.UnitDuration)
	return nil
}

func (d *DiskIO) Gather(g *metric.Gather) error {
	ioCounters, err := disk.IOCounters(d.devices...)
	if err != nil {
		slog.Error("diskio getting io counters", "error", err)
		return err
	}
	for device, ioCounter := range ioCounters {
		name := fmt.Sprintf("diskio:%s:", device)
		g.Add(name+"read_bytes", float64(ioCounter.ReadBytes), d.metricBytesType)
		g.Add(name+"write_bytes", float64(ioCounter.WriteBytes), d.metricBytesType)
		g.Add(name+"read_count", float64(ioCounter.ReadCount), d.metricCountType)
		g.Add(name+"write_count", float64(ioCounter.WriteCount), d.metricCountType)
		g.Add(name+"read_time", float64(ioCounter.ReadTime*1000_000), d.metricDurationType)
		g.Add(name+"write_time", float64(ioCounter.WriteTime*1000_000), d.metricDurationType)
		g.Add(name+"io_time", float64(ioCounter.IoTime*1000_000), d.metricDurationType)
		g.Add(name+"weighted_io_time", float64(ioCounter.WeightedIO*1000_000), d.metricDurationType)
	}
	return nil
}
