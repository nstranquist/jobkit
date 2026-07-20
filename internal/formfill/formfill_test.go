package formfill

import (
	"strings"
	"testing"
)

func TestSplitName(t *testing.T) {
	d := Data{FullName: "Sam Alan Rivera"}
	d.SplitName()
	if d.FirstName != "Sam Alan" || d.LastName != "Rivera" {
		t.Fatalf("split = %q / %q", d.FirstName, d.LastName)
	}
	single := Data{FullName: "Prince"}
	single.SplitName()
	if single.FirstName != "Prince" || single.LastName != "" {
		t.Fatalf("single = %q / %q", single.FirstName, single.LastName)
	}
	preset := Data{FullName: "A B", FirstName: "Custom"}
	preset.SplitName()
	if preset.FirstName != "Custom" {
		t.Fatal("preset first name must win")
	}
}

func TestPickLinks(t *testing.T) {
	d := Data{}
	d.PickLinks(map[string]string{
		"LinkedIn":       "https://linkedin.com/in/x",
		"GitHub":         "https://github.com/x",
		"Portfolio Site": "https://x.dev",
	})
	if d.LinkedIn == "" || d.GitHub == "" || d.Website == "" {
		t.Fatalf("links = %+v", d)
	}
}

func TestSnippetSafetyAndEscaping(t *testing.T) {
	d := Data{
		FullName: `Sam "The Builder" O'Rivera`,
		Email:    "sam@example.com",
		Phone:    "(555) 010-0199",
	}
	js, err := Snippet(d)
	if err != nil {
		t.Fatal(err)
	}
	// Data must arrive JSON-escaped, not raw.
	if !strings.Contains(js, `\"The Builder\"`) {
		t.Fatalf("quotes not escaped in payload:\n%s", js)
	}
	// The snippet must never submit.
	for _, forbidden := range []string{".submit(", ".click(", "requestSubmit"} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("snippet contains forbidden call %q", forbidden)
		}
	}
	// Core selectors present for both providers.
	for _, sel := range []string{"#first_name", `input[name="name"]`, `urls[LinkedIn]`, "querySelectorAll('label')"} {
		if !strings.Contains(js, sel) {
			t.Fatalf("missing selector %q", sel)
		}
	}
	// File inputs are refused in setVal.
	if !strings.Contains(js, "el.type === 'file'") {
		t.Fatal("file-input guard missing")
	}
}
