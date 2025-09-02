package ndjson

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OutOfBedlam/metric"
)

type Output struct {
	DestUrl string // e.g. "http://127.0.0.1:5654/db/write/TAG"
}

func (o Output) Export(pd metric.Product) {
	m := map[string]any{
		"NAME": fmt.Sprintf("%s:%s", pd.Measure, pd.Field),
		"TIME": pd.Time.UnixNano(),
		"TYPE": pd.Type,
	}
	switch p := pd.Value.(type) {
	case *metric.CounterValue:
		m["VALUE"] = p.Value
		m["SAMPLES"] = p.Samples
	case *metric.GaugeValue:
		m["VALUE"] = p.Value
		m["SAMPLES"] = p.Samples
		m["SUM"] = p.Sum
	case *metric.MeterValue:
		value := 0.0
		if p.Samples > 0 {
			value = p.Sum / float64(p.Samples)
		}
		m["VALUE"] = value
		m["SAMPLES"] = p.Samples
		m["SUM"] = p.Sum
		m["LAST"] = p.Last
		m["FIRST"] = p.First
		m["MIN"] = p.Min
		m["MAX"] = p.Max
	case *metric.HistogramValue:
		value := 0.0
		for i, x := range p.P {
			if x == 0.5 {
				value = p.Values[i]
			}
			m[fmt.Sprintf("P%d", int(x*100))] = p.Values[i]
		}
		m["VALUE"] = value
		m["SAMPLES"] = p.Samples
	default:
		fmt.Printf("Unknown product type: %T\n", p)
		return
	}
	n, err := json.Marshal(m)
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
