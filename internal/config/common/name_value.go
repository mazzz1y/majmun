package common

import "fmt"

type NameValue struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

func (nv *NameValue) Validate() error {
	if nv.Name == "" {
		return fmt.Errorf("name is required")
	}
	if nv.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func MergeNameValues(base, override []NameValue) []NameValue {
	if len(base) == 0 {
		return override
	}
	if len(override) == 0 {
		return base
	}

	merged := make(map[string]string, len(base)+len(override))
	order := make([]string, 0, len(base)+len(override))

	for _, nv := range base {
		merged[nv.Name] = nv.Value
		order = append(order, nv.Name)
	}
	for _, nv := range override {
		if _, exists := merged[nv.Name]; !exists {
			order = append(order, nv.Name)
		}
		merged[nv.Name] = nv.Value
	}

	result := make([]NameValue, 0, len(merged))
	for _, name := range order {
		result = append(result, NameValue{Name: name, Value: merged[name]})
	}
	return result
}
