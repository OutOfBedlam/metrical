package registry

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/OutOfBedlam/metric"
)

var registry = make(map[string]RegisterItem)

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
		if !strings.HasPrefix(name, "input.") {
			name = "input." + name
		}
	} else if _, ok := nilPtr.(metric.Output); ok {
		if !strings.HasPrefix(name, "output.") {
			name = "output." + name
		}
	} else {
		return fmt.Errorf("only Input or Output can be registered, got: %T", nilPtr)
	}
	if _, exists := registry[name]; exists {
		return fmt.Errorf("already registered name: %s", name)
	}
	registry[name] = RegisterItem{
		Type:         reflect.TypeOf(nilPtr).Elem(),
		SampleConfig: sampleConfig,
	}
	return nil
}

func GenerateSampleConfig(w io.Writer) {
	names := []string{}
	for k := range registry {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, k := range names {
		sample := registry[k].SampleConfig
		fmt.Fprintln(w, sample)
		fmt.Fprintln(w)
	}
}

func LoadConfig(c *metric.Collector, content string) error {
	cfg := make(map[string]any)
	processedNames := map[string][]string{
		"input":  {},
		"output": {},
	}
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
		case "input", "output":
			reg, ok := registry[kind+"."+name]
			if !ok {
				return fmt.Errorf("unknown %s type: %s", kind, name)
			}
			if slices.Contains(processedNames[kind], name) {
				continue
			}
			processedNames[kind] = append(processedNames[kind], name)
			sections := ((cfg[kind].(map[string]any))[name]).([]map[string]any)
			for _, section := range sections {
				v := reflect.New(reg.Type).Interface()
				if b, err := toml.Marshal(section); err != nil {
					return err
				} else {
					if _, err := toml.Decode(string(b), v); err != nil {
						return err
					}
				}
				var filter metric.Filter
				if x, ok := section["filter"].(map[string]any); ok {
					includes, excludes := x["includes"], x["excludes"]
					if f, err := compileFilter(includes, excludes); err != nil {
						return err
					} else {
						filter = f
					}
				}
				if input, ok := v.(metric.Input); ok {
					if filter != nil {
						input = &metric.FilterInput{Filter: filter, Input: input}
					}
					if err := c.AddInput(input); err != nil {
						return err
					}
				} else if output, ok := v.(metric.Output); ok {
					if filter != nil {
						output = &metric.FilterOutput{Filter: filter, Output: output}
					}
					if err := c.AddOutput(output); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("type %s is not implement %s", name, kind)
				}
			}
		}
	}
	return nil
}

func compileFilter(includesAny any, excludesAny any) (metric.Filter, error) {
	excludes, ok := excludesAny.([]any)
	if !ok {
		excludes = []any{}
	}
	includes, ok := includesAny.([]any)
	if !ok {
		includes = []any{}
	}
	inc := []string{}
	for _, v := range includes {
		if s, ok := v.(string); ok {
			inc = append(inc, s)
		}
	}
	exc := []string{}
	for _, v := range excludes {
		if s, ok := v.(string); ok {
			exc = append(exc, s)
		}
	}
	if len(inc) == 0 && len(exc) == 0 {
		return nil, nil
	}
	return metric.CompileIncludeAndExclude(inc, exc, ':')
}
