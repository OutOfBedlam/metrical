package ndjson

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OutOfBedlam/metric"
)

type Output struct {
	DestUrl                  string // e.g. "http://127.0.0.1:5654/db/write/TAG"
	HistogramValuePercentile float64
}

type Record struct {
	Name   string `json:"NAME"`
	Time   int64  `json:"TIME"`
	Type   string `json:"TYPE"`
	Period string `json:"PERIOD"`
	// Counter
	Value   float64 `json:"VALUE,omitempty"`
	Samples int64   `json:"SAMPLES,omitempty"`
	// Gauge
	Sum float64 `json:"SUM,omitempty"`
	// Meter
	Last  float64 `json:"LAST,omitempty"`
	First float64 `json:"FIRST,omitempty"`
	Min   float64 `json:"MIN,omitempty"`
	Max   float64 `json:"MAX,omitempty"`
	// Histogram
	P map[string]float64 `json:"P,omitempty"`
}

func (o Output) Export(pd metric.Product) {
	r := Record{}
	r.Name = fmt.Sprintf("%s:%s", pd.Measure, pd.Field)
	r.Time = pd.Time.UnixNano()
	r.Type = pd.Type
	r.Period = pd.Period.String()

	switch p := pd.Value.(type) {
	case *metric.CounterValue:
		r.Value = p.Value
		r.Samples = p.Samples
	case *metric.GaugeValue:
		r.Value = p.Value
		r.Samples = p.Samples
		r.Sum = p.Sum
	case *metric.MeterValue:
		value := 0.0
		if p.Samples > 0 {
			value = p.Sum / float64(p.Samples)
		}
		r.Value = value
		r.Samples = p.Samples
		r.Sum = p.Sum
		r.Last = p.Last
		r.First = p.First
		r.Min = p.Min
		r.Max = p.Max
	case *metric.HistogramValue:
		value := 0.0
		valuePercentile := 0.5
		if o.HistogramValuePercentile > 0 {
			valuePercentile = o.HistogramValuePercentile
		}
		for i, x := range p.P {
			if x == valuePercentile {
				value = p.Values[i]
			}
			if r.P == nil {
				r.P = make(map[string]float64)
			}
			k := fmt.Sprintf("P%d", int(x*1000))
			if k[len(k)-1] == '0' {
				k = k[:len(k)-1]
			}
			r.P[k] = p.Values[i]
		}
		r.Value = value
		r.Samples = p.Samples
	default:
		fmt.Printf("Unknown product type: %T\n", p)
		return
	}
	n, err := json.Marshal(r)
	if err != nil {
		fmt.Printf("Error marshaling product: %v\n", err)
		return
	}
	if o.DestUrl == "" {
		fmt.Println(string(n))
	} else {
		rsp, err := http.DefaultClient.Post(
			o.DestUrl,
			"application/x-ndjson",
			strings.NewReader(string(n)))
		if err != nil {
			fmt.Printf("Error sending product: %v\n", err)
			return
		}
		defer rsp.Body.Close()
		if rsp.StatusCode != http.StatusOK {
			fmt.Printf("Error response from server: %s\n", rsp.Status)
			return
		}
	}
}
