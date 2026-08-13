package strictyaml

import (
	"strings"
	"testing"
)

func TestUnmarshalRejectsUnknownFieldsAndExtraDocuments(t *testing.T) {
	var target struct {
		Name string
	}
	if err := Unmarshal([]byte("name: JobKit\nunknown: true\n"), &target); err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("unknown field error = %v", err)
	}
	if err := Unmarshal([]byte("name: JobKit\n---\nname: second\n"), &target); err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("multiple document error = %v", err)
	}
	if err := Unmarshal([]byte("name: JobKit\n"), &target); err != nil || target.Name != "JobKit" {
		t.Fatalf("valid decode = %#v, %v", target, err)
	}
}
