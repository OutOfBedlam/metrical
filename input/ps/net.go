package ps

import (
	"path"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/net"
)

type Net struct {
	Interfaces []string

	prev map[string]uint64
}

var _ metric.Input = (*Net)(nil)

func (n *Net) Init() error {
	return nil
}

func (n *Net) Gather(g metric.Gather) {
	counters, err := net.IOCounters(true)
	if err != nil {
		g.AddError(err)
		return
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		g.AddError(err)
		return
	}

	interfacesByName := make(map[string]net.InterfaceStat, len(interfaces))
	for _, iface := range interfaces {
		interfacesByName[iface.Name] = iface
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

	for _, c := range counters {
		if len(n.Interfaces) != 0 {
			found := false
			for _, iface := range n.Interfaces {
				if ok, err := path.Match(iface, c.Name); err == nil && ok {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		counts["bytes_sent"] += c.BytesSent
		counts["bytes_recv"] += c.BytesRecv
		counts["packets_sent"] += c.PacketsSent
		counts["packets_recv"] += c.PacketsRecv
		counts["err_in"] += c.Errin
		counts["err_out"] += c.Errout
		counts["drop_in"] += c.Dropin
		counts["drop_out"] += c.Dropout
	}

	m := metric.Measurement{Name: "net"}
	if n.prev != nil {
		bytesCounterType := metric.CounterType(metric.UnitBytes)
		shortCounterType := metric.CounterType(metric.UnitShort)
		m.AddField(
			metric.Field{Name: "bytes_sent", Value: float64(counts["bytes_sent"] - n.prev["bytes_sent"]), Type: bytesCounterType},
			metric.Field{Name: "bytes_recv", Value: float64(counts["bytes_recv"] - n.prev["bytes_recv"]), Type: bytesCounterType},
			metric.Field{Name: "packets_sent", Value: float64(counts["packets_sent"] - n.prev["packets_sent"]), Type: shortCounterType},
			metric.Field{Name: "packets_recv", Value: float64(counts["packets_recv"] - n.prev["packets_recv"]), Type: shortCounterType},
			metric.Field{Name: "err_in", Value: float64(counts["err_in"] - n.prev["err_in"]), Type: shortCounterType},
			metric.Field{Name: "err_out", Value: float64(counts["err_out"] - n.prev["err_out"]), Type: shortCounterType},
			metric.Field{Name: "drop_in", Value: float64(counts["drop_in"] - n.prev["drop_in"]), Type: shortCounterType},
			metric.Field{Name: "drop_out", Value: float64(counts["drop_out"] - n.prev["drop_out"]), Type: shortCounterType},
		)
	}
	n.prev = counts
	g.AddMeasurement(m)
}
