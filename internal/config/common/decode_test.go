package common

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDecodeStrict_UnknownField(t *testing.T) {
	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	yamlData := `
name: test
value: 42
unknown_field: oops
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	var result TestStruct
	err := DecodeStrict(node.Content[0], &result)
	if err == nil {
		t.Error("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Errorf("expected error to mention 'unknown_field', got: %v", err)
	}
}

func TestDecodeStrict_ValidStruct(t *testing.T) {
	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	yamlData := `
name: test
value: 42
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	var result TestStruct
	err := DecodeStrict(node.Content[0], &result)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Name != "test" || result.Value != 42 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDecodeStrict_NestedStruct(t *testing.T) {
	type Inner struct {
		Field string `yaml:"field"`
	}
	type Outer struct {
		Name  string `yaml:"name"`
		Inner Inner  `yaml:"inner"`
	}

	yamlData := `
name: test
inner:
  field: value
  bad_field: oops
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	var result Outer
	err := DecodeStrict(node.Content[0], &result)
	if err == nil {
		t.Error("expected error for unknown nested field, got nil")
	}
	if !strings.Contains(err.Error(), "bad_field") {
		t.Errorf("expected error to mention 'bad_field', got: %v", err)
	}
}
