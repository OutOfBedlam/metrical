package ps

import (
	_ "embed"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/net"
)

func init() {
	registry.Register("net", (*Net)(nil))
}

//go:embed "net.toml"
var netSampleConfig string

func (n *Net) SampleConfig() string {
	return netSampleConfig
}

// bytes_sent, bytes_recv, packets_sent, packets_recv, err_in, err_out, drop_in, drop_out
type Net struct {
	Interfaces []string `toml:"interfaces"` // empty for all interfaces (default) e.g. []{"eth*", "en*"}
	Includes   []string `toml:"includes"`   // empty for all kind (default) e.g. []{"bytes_*", "packets_*"}
	Excludes   []string `toml:"excludes"`   // e.g. []{"*_err*", "*_drop*"}
	PerNIC     bool     `toml:"per_nic"`    // false for aggregate all interfaces (default), true for per-interface stats

	iface  []string // filtered interface names
	filter metric.Filter
}

var _ metric.Input = (*Net)(nil)

func (n *Net) Init() error {
	if len(n.Includes) > 0 || len(n.Excludes) > 0 {
		f, err := metric.CompileIncludeAndExclude(n.Includes, n.Excludes)
		if err != nil {
			return err
		}
		n.filter = f
	}

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

func (n *Net) Gather(g metric.Gather) {
	counters, err := net.IOCounters(true)
	if err != nil {
		g.AddError(err)
		return
	}

	counts := map[string]uint64{
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
			found := false
			for _, iface := range n.iface {
				if iface == c.Name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if !n.PerNIC {
			counts["bytes_sent"] += c.BytesSent
			counts["bytes_recv"] += c.BytesRecv
			counts["packets_sent"] += c.PacketsSent
			counts["packets_recv"] += c.PacketsRecv
			counts["err_in"] += c.Errin
			counts["err_out"] += c.Errout
			counts["drop_in"] += c.Dropin
			counts["drop_out"] += c.Dropout
		} else {
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
			m := metric.Measurement{Name: "net:" + c.Name}
			for k, v := range nicCounts {
				if n.filter != nil && !n.filter.Match(k) {
					continue
				}
				var t metric.Type
				switch k {
				case "bytes_sent", "bytes_recv":
					t = bytesOdometerType
				default:
					t = shortOdometerType
				}
				m.AddField(metric.Field{Name: k, Value: float64(v), Type: t})
			}
			// only add measurement if there is at least one field
			// this allows filtering to exclude all fields and avoid empty measurements
			// e.g. includes=["bytes_*"], excludes=["*_sent", "*_recv"]
			// or includes=["*_err*"]
			if len(m.Fields) > 0 {
				g.AddMeasurement(m)
			}
		}
	}

	if !n.PerNIC {
		m := metric.Measurement{Name: "net"}
		for k, v := range counts {
			if n.filter != nil && !n.filter.Match(k) {
				continue
			}
			var t metric.Type
			switch k {
			case "bytes_sent", "bytes_recv":
				t = bytesOdometerType
			default:
				t = shortOdometerType
			}
			m.AddField(metric.Field{Name: k, Value: float64(v), Type: t})
		}
		if len(m.Fields) > 0 {
			g.AddMeasurement(m)
		}
	}
}
