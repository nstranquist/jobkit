// Package strictyaml decodes one YAML document and rejects unknown fields.
package strictyaml

import (
	"bytes"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

// Unmarshal decodes exactly one YAML document into target.
func Unmarshal(payload []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple YAML documents are not allowed")
		}
		return fmt.Errorf("decode trailing YAML data: %w", err)
	}
	return nil
}
