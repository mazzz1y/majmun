package common

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

func DecodeStrict(node *yaml.Node, v any) error {
	if err := checkAllowedKeys(node, v); err != nil {
		return err
	}
	return node.Decode(v)
}

func checkAllowedKeys(node *yaml.Node, v any) error {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}

	allowed := extractYAMLKeys(v)

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		key := keyNode.Value
		if key == "<<" {
			continue
		}
		if !allowed[key] {
			return fmt.Errorf("line %d: unknown field %s", keyNode.Line, key)
		}
	}
	return nil
}

func extractYAMLKeys(v any) map[string]bool {
	keys := make(map[string]bool)
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return keys
	}

	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		keys[name] = true
	}
	return keys
}
