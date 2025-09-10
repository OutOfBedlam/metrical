package ps

import (
	_ "embed"
	"slices"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/net"
)

func init() {
	registry.Register("input.net", (*Net)(nil))
}

//go:embed "net.toml"
var netSampleConfig string

func (n *Net) SampleConfig() string {
	return netSampleConfig
}

// bytes_sent, bytes_recv, packets_sent, packets_recv, err_in, err_out, drop_in, drop_out
type Net struct {
	Interfaces []string `toml:"interfaces"` // empty for all interfaces (default) e.g. []{"eth*", "en*"}
	PerNIC     bool     `toml:"per_nic"`    // false for aggregate all interfaces (default), true for per-interface stats

	iface []string // filtered interface names
}

var _ metric.Input = (*Net)(nil)

func (n *Net) Init() error {
	var interfaceFilter metric.Filter
	if len(n.Interfaces) > 0 {
		filter, err := metric.Compile(n.Interfaces)
		if err != nil {
			return err
		}
		interfaceFilter = filter
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	for _, iface := range interfaces {
		if interfaceFilter == nil || interfaceFilter.Match(iface.Name) {
			n.iface = append(n.iface, iface.Name)
		}
	}
	return nil
}

func (n *Net) Gather(g *metric.Gather) error {
	counters, err := net.IOCounters(true)
	if err != nil {
		return err
	}

	allCounts := map[string]uint64{
		"bytes_sent":   0,
		"bytes_recv":   0,
		"packets_sent": 0,
		"packets_recv": 0,
		"err_in":       0,
		"err_out":      0,
		"drop_in":      0,
		"drop_out":     0,
	}

	bytesOdometerType := metric.OdometerType(metric.UnitBytes)
	shortOdometerType := metric.OdometerType(metric.UnitShort)

	for _, c := range counters {
		if len(n.iface) != 0 {
			if idx := slices.Index(n.iface, c.Name); idx < 0 { // ensure n.iface is sorted
				continue
			}
		}
		allCounts["bytes_sent"] += c.BytesSent
		allCounts["bytes_recv"] += c.BytesRecv
		allCounts["packets_sent"] += c.PacketsSent
		allCounts["packets_recv"] += c.PacketsRecv
		allCounts["err_in"] += c.Errin
		allCounts["err_out"] += c.Errout
		allCounts["drop_in"] += c.Dropin
		allCounts["drop_out"] += c.Dropout
		if n.PerNIC {
			nicCounts := map[string]uint64{
				"bytes_sent":   c.BytesSent,
				"bytes_recv":   c.BytesRecv,
				"packets_sent": c.PacketsSent,
				"packets_recv": c.PacketsRecv,
				"err_in":       c.Errin,
				"err_out":      c.Errout,
				"drop_in":      c.Dropin,
				"drop_out":     c.Dropout,
			}
			for k, v := range nicCounts {
				var typ metric.Type
				switch k {
				case "bytes_sent", "bytes_recv":
					typ = bytesOdometerType
				default:
					typ = shortOdometerType
				}
				g.Add("net:"+c.Name+":"+k, float64(v), typ)
			}
		}
	}

	for k, v := range allCounts {
		var typ metric.Type
		switch k {
		case "bytes_sent", "bytes_recv":
			typ = bytesOdometerType
		default:
			typ = shortOdometerType
		}
		g.Add("net:all:"+k, float64(v), typ)
	}
	return nil
}
