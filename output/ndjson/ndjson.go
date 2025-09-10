package ndjson

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
)

func init() {
	registry.Register("output.ndjson", (*Encoder)(nil))
}

//go:embed "ndjson.toml"
var ndjsonSampleConfig string

func (o *Encoder) SampleConfig() string {
	return ndjsonSampleConfig
}

var _ metric.Output = (*Encoder)(nil)

type Encoder struct {
	DestUrl                  string  `toml:"dest"`
	Timeformat               string  `toml:"timeformat"`
	HistogramValuePercentile float64 `toml:"histogram_value_selector"`
	OdometerValueSelector    string  `toml:"odometer_value_selector"`
}

func (o *Encoder) Init() error {
	if o.Timeformat == "" {
		o.Timeformat = "ns"
	}
	return nil
}

func (o Encoder) Process(pd metric.Product) error {
	r, err := o.convert(pd)
	if err != nil {
		return err
	}
	if r == nil {
		return nil
	}

	n, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	if o.DestUrl == "" {
		fmt.Println(string(n))
	} else {
		rsp, err := http.DefaultClient.Post(
			o.DestUrl,
			"application/x-ndjson",
			strings.NewReader(string(n)))
		if err != nil {
			return err
		}
		defer rsp.Body.Close()
		if rsp.StatusCode >= 200 && rsp.StatusCode < 300 {
			return fmt.Errorf("error response from server: %s", rsp.Status)
		}
	}
	return nil
}

type Record struct {
	Name    string  `json:"NAME"`
	Time    any     `json:"TIME"`
	Type    string  `json:"TYPE"`
	Period  string  `json:"PERIOD"`
	Samples int64   `json:"SAMPLES"`
	Value   float64 `json:"VALUE,omitempty"`
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

func (o Encoder) convert(pd metric.Product) (*Record, error) {
	r := &Record{}
	r.Name = pd.Name
	switch o.Timeformat {
	case "s":
		r.Time = pd.Time.Unix()
	case "ms":
		r.Time = pd.Time.UnixNano() / 1e6
	case "us", "µs":
		r.Time = pd.Time.UnixNano() / 1e3
	case "ns":
		r.Time = pd.Time.UnixNano()
	default:
		r.Time = pd.Time.Format(o.Timeformat)
	}
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
		valuePercentile := o.HistogramValuePercentile
		for i, x := range p.P {
			if (i == 0 && valuePercentile == 0) || x == valuePercentile {
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
	case *metric.OdometerValue:
		switch o.OdometerValueSelector {
		case "no_negative_diff":
			r.Value = p.NonNegativeDiff()
		case "abs_diff":
			r.Value = p.AbsDiff()
		default: // "diff"
			r.Value = p.Diff()
		}
		r.Samples = p.Samples
		r.Last = p.Last
		r.First = p.First
	default:
		return nil, fmt.Errorf("unknown product type: %T", p)
	}
	if r.Samples == 0 {
		return nil, nil
	}
	return r, nil
}
