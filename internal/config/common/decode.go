package common

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

func DecodeStrict(node *yaml.Node, v any) error {
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	if err := enc.Encode(node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	dec := yaml.NewDecoder(buf)
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}
