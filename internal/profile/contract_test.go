package profile

import (
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestProfileContractV1RoundTrip(t *testing.T) {
	path := filepath.Join("..", "..", "contracts", "profile", "v1.example.yaml")
	want, err := Load(path)
	if err != nil {
		t.Fatalf("load v1 contract fixture: %v", err)
	}
	if len(want.Links) == 0 || len(want.Skills) == 0 || len(want.Experience) == 0 || len(want.Projects) == 0 || len(want.Education) == 0 || len(want.Certifications) == 0 {
		t.Fatal("v1 contract fixture must exercise every collection field")
	}

	body, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("marshal v1 contract fixture: %v", err)
	}
	var got Profile
	if err := yaml.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal v1 contract fixture: %v", err)
	}
	if !reflect.DeepEqual(*want, got) {
		t.Fatalf("v1 contract changed during YAML round trip\nwant: %#v\n got: %#v", *want, got)
	}
}
