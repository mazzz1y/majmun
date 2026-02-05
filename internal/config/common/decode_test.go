package common

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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

func TestDecodeStrict_UnknownField(t *testing.T) {
	type TestStruct struct {
		Name string `yaml:"name"`
	}

	yamlData := `
name: test
unknown: bad
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	var result TestStruct
	err := DecodeStrict(node.Content[0], &result)
	if err == nil {
		t.Error("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected error to mention 'unknown', got: %v", err)
	}
}

func TestDecodeStrict_WithYAMLAnchor(t *testing.T) {
	type Condition struct {
		Clients []string `yaml:"clients"`
	}
	type Rule struct {
		Condition *Condition `yaml:"condition"`
	}

	yamlData := `
.anchor: &anchor
  clients: ["client1", "client2"]

rule:
  condition:
    <<: *anchor
`
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &node); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	for i := 0; i < len(node.Content[0].Content); i += 2 {
		if node.Content[0].Content[i].Value == "rule" {
			var rule Rule
			err := DecodeStrict(node.Content[0].Content[i+1], &rule)
			if err != nil {
				t.Errorf("unexpected error with YAML anchor: %v", err)
			}
			if rule.Condition == nil {
				t.Error("condition should not be nil")
			} else if len(rule.Condition.Clients) != 2 {
				t.Errorf("expected 2 clients, got %d", len(rule.Condition.Clients))
			}
			return
		}
	}
	t.Error("rule not found in yaml")
}
