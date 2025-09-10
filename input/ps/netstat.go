package ps

import (
	_ "embed"
	"syscall"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/net"
)

func init() {
	registry.Register("input.netstat", (*NetStat)(nil))
}

//go:embed "netstat.toml"
var netstatSampleConfig string

func (n *NetStat) SampleConfig() string {
	return netstatSampleConfig

}

// status -> metric_name
var statusList = map[string]string{
	"ESTABLISHED": "tcp_established",
	"SYN_SENT":    "tcp_syn_sent",
	"SYN_RECV":    "tcp_syn_recv",
	"FIN_WAIT1":   "tcp_fin_wait1",
	"FIN_WAIT2":   "tcp_fin_wait2",
	"TIME_WAIT":   "tcp_time_wait",
	"CLOSE":       "tcp_close",
	"CLOSE_WAIT":  "tcp_close_wait",
	"LAST_ACK":    "tcp_last_ack",
	"LISTEN":      "tcp_listen",
	"CLOSING":     "tcp_closing",
	"NONE":        "tcp_none",
	"UDP":         "udp_socket",
}

// tcp_established, tcp_syn_sent, tcp_syn_recv, tcp_fin_wait1, tcp_fin_wait2,
// tcp_time_wait, tcp_close, tcp_close_wait, tcp_last_ack, tcp_listen,
// tcp_closing, tcp_none, udp_socket
type NetStat struct {
}

var _ metric.Input = (*NetStat)(nil)

var gaugeType = metric.GaugeType(metric.UnitShort)

func (ns *NetStat) Gather(g *metric.Gather) error {
	stat, err := net.Connections("all")
	if err != nil {
		return err
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

	for kind, name := range statusList {
		value, ok := counts[kind]
		if !ok {
			value = 0
		}
		val := float64(value)
		g.Add("netstat:"+name, val, gaugeType)
	}
	return nil
}
