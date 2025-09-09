package registry

import (
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/OutOfBedlam/metric"
)

var inputRegistry = make(map[string]RegisterItem)

type RegisterItem struct {
	Type         reflect.Type
	SampleConfig string
}

func Register(name string, nilPtr any) error {
	sampleConfig := ""
	if sample, ok := nilPtr.(interface{ SampleConfig() string }); ok {
		sampleConfig = sample.SampleConfig()
	}
	if _, ok := nilPtr.(metric.Input); ok {
		inputRegistry[name] = RegisterItem{
			Type:         reflect.TypeOf(nilPtr).Elem(),
			SampleConfig: sampleConfig,
		}
		return nil
	}
	return fmt.Errorf("type %T does not unknown type", nilPtr)
}

func GenerateSampleConfig(w io.Writer) {
	inputNames := []string{}
	for k := range inputRegistry {
		inputNames = append(inputNames, k)
	}
	slices.Sort(inputNames)
	for _, k := range inputNames {
		sample := inputRegistry[k].SampleConfig
		fmt.Fprintln(w, sample)
		fmt.Fprintln(w)
	}
}

func LoadConfig(c *metric.Collector, content string) error {
	cfg := make(map[string]any)
	inputNames := []string{}
	meta, err := toml.Decode(content, &cfg)
	if err != nil {
		return err
	}
	for _, keys := range meta.Keys() {
		if len(keys) != 2 {
			continue
		}
		kind, name := keys[0], keys[1]
		switch kind {
		case "input":
			reg, ok := inputRegistry[name]
			if !ok {
				return fmt.Errorf("unknown input type: %s", name)
			}
			if slices.Contains(inputNames, name) {
				continue
			}
			inputNames = append(inputNames, name)
			sections := ((cfg["input"].(map[string]any))[name]).([]map[string]any)
			for _, section := range sections {
				v := reflect.New(reg.Type).Interface()
				if b, err := toml.Marshal(section); err != nil {
					return err
				} else {
					if _, err := toml.Decode(string(b), v); err != nil {
						return err
					}
				}
				input, ok := v.(metric.Input)
				if !ok {
					return fmt.Errorf("type %s does not implement metric.Input", name)
				}
				if in, ok := input.(interface{ Init() error }); ok {
					if err := in.Init(); err != nil {
						return fmt.Errorf("error initializing input %s: %w", name, err)
					}
				} else {
					return fmt.Errorf("type %s does not implement metric.Input", name)
				}
				if err := c.AddInput(input); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
