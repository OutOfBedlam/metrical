package ps

import (
	"syscall"

	"github.com/OutOfBedlam/metric"
	"github.com/shirou/gopsutil/v4/net"
)

type NetStat struct {
}

var _ metric.Input = (*NetStat)(nil)

var gaugeType = metric.GaugeType(metric.UnitShort)

func (ns *NetStat) Init() error {
	return nil
}

func (ns *NetStat) Gather(g metric.Gather) {
	stat, err := net.Connections("all")
	if err != nil {
		g.AddError(err)
		return
	}

	counts := make(map[string]int)
	counts["UDP"] = 0

	for _, cs := range stat {
		if cs.Type == syscall.SOCK_DGRAM {
			counts["UDP"]++
			continue
		}
		c, ok := counts[cs.Status]
		if !ok {
			counts[cs.Status] = 0
		}
		counts[cs.Status] = c + 1
	}

	m := metric.Measurement{Name: "netstat"}
	m.AddField(
		metric.Field{Name: "tcp_established", Value: float64(counts["ESTABLISHED"]), Type: gaugeType},
		metric.Field{Name: "tcp_syn_sent", Value: float64(counts["SYN_SENT"]), Type: gaugeType},
		metric.Field{Name: "tcp_syn_recv", Value: float64(counts["SYN_RECV"]), Type: gaugeType},
		metric.Field{Name: "tcp_fin_wait1", Value: float64(counts["FIN_WAIT1"]), Type: gaugeType},
		metric.Field{Name: "tcp_fin_wait2", Value: float64(counts["FIN_WAIT2"]), Type: gaugeType},
		metric.Field{Name: "tcp_time_wait", Value: float64(counts["TIME_WAIT"]), Type: gaugeType},
		metric.Field{Name: "tcp_close", Value: float64(counts["CLOSE"]), Type: gaugeType},
		metric.Field{Name: "tcp_close_wait", Value: float64(counts["CLOSE_WAIT"]), Type: gaugeType},
		metric.Field{Name: "tcp_last_ack", Value: float64(counts["LAST_ACK"]), Type: gaugeType},
		metric.Field{Name: "tcp_listen", Value: float64(counts["LISTEN"]), Type: gaugeType},
		metric.Field{Name: "tcp_closing", Value: float64(counts["CLOSING"]), Type: gaugeType},
		metric.Field{Name: "tcp_none", Value: float64(counts["NONE"]), Type: gaugeType},
		metric.Field{Name: "udp_socket", Value: float64(counts["UDP"]), Type: gaugeType},
	)
	g.AddMeasurement(m)
}
