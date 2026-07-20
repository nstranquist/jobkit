package claims

import (
	"strings"
	"testing"
)

func TestExtractShapes(t *testing.T) {
	text := "Built a platform serving 1,000+ developers across 30,000+ repositories. " +
		"Increased delivery speed by 33% over 7+ years; baseline reached 95.8% Hit@1. " +
		"Comp target $180,000. Joined Jan 2021, left Mar 2026."
	got := Extract(text)
	want := map[string]bool{"1000+": true, "30000+": true, "33%": true, "7+": true, "95.8%": true, "$180000": true}
	if len(got) != len(want) {
		var tokens []string
		for _, v := range got {
			tokens = append(tokens, v.Token)
		}
		t.Fatalf("tokens = %v, want keys of %v", tokens, want)
	}
	for _, v := range got {
		if !want[v.Token] {
			t.Fatalf("unexpected token %q (context %q)", v.Token, v.Context)
		}
		if v.Context == "" {
			t.Fatalf("empty context for %q", v.Token)
		}
	}
}

func TestExtractSkipsContactDetails(t *testing.T) {
	// Synthetic contact details only — never put real personal profile data in-repo.
	text := "Sam Rivera | sam@example.com | (555) 010-0199 | " +
		"linkedin.com/in/samrivera | github.com/samrivera/repo123 | https://x.dev/95users"
	if got := Extract(text); len(got) != 0 {
		t.Fatalf("contact details must not extract, got %+v", got)
	}
	// Real claims adjacent to contact details still extract.
	got := Extract("sam@example.com — serving 1,000+ developers")
	if len(got) != 1 || got[0].Token != "1000+" {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractSkipsCalendarYears(t *testing.T) {
	got := Extract("From 2019 to 2026 I shipped software.")
	if len(got) != 0 {
		t.Fatalf("calendar years should be skipped, got %+v", got)
	}
}

func TestCheckCoversAndFlags(t *testing.T) {
	allowed := []string{
		"Backstage IDP serving 1,000+ developers across 30,000+ repositories",
		"33% team delivery speed increase at Enhearten",
		"7+ years of experience",
	}
	clean := Check("I support 1,000+ developers and 30,000+ repositories, with 7+ years experience and a 33% improvement.", allowed)
	if clean != nil {
		t.Fatalf("expected clean, got %+v", clean)
	}
	dirty := Check("I led 5,000+ engineers and cut costs 42%.", allowed)
	if len(dirty) != 2 {
		t.Fatalf("violations = %+v, want 2", dirty)
	}
	tokens := map[string]bool{}
	for _, v := range dirty {
		tokens[v.Token] = true
	}
	if !tokens["5000+"] || !tokens["42%"] {
		t.Fatalf("tokens = %v", tokens)
	}
}

func TestCheckYearsPhrase(t *testing.T) {
	// "7 years" without the plus must still be covered by an entry
	// containing "7 years".
	allowed := []string{"7 years of production experience"}
	if v := Check("I have 7 years of experience.", allowed); v != nil {
		t.Fatalf("expected covered, got %+v", v)
	}
	if v := Check("I have 12 years of experience.", allowed); len(v) != 1 {
		t.Fatalf("expected 1 violation, got %+v", v)
	}
}

func TestCheckPlusEntailment(t *testing.T) {
	// A verified "7+ years" covers a claimed "7 years"...
	if v := Check("with 7 years of experience", []string{"7+ years shipping products"}); v != nil {
		t.Fatalf("plus-entailment failed: %+v", v)
	}
	// ...and "1,000+ developers" covers "1000 developers"...
	if v := Check("serving 1000 developers", []string{"1,000+ developers"}); v != nil {
		t.Fatalf("bare-number entailment failed: %+v", v)
	}
	// ...but never the reverse: exact 7 does not license claiming 7+.
	if v := Check("with 7+ years of experience", []string{"7 years at one employer"}); len(v) != 1 {
		t.Fatalf("reverse entailment must fail, got %+v", v)
	}
	// Percentages stay exact.
	if v := Check("improved 33%", []string{"33.5% improvement"}); len(v) != 1 {
		t.Fatalf("percent must be exact, got %+v", v)
	}
	// Digit boundaries: an allowed "133%" never covers a claimed "33%".
	if v := Check("improved 33%", []string{"scaled 133% year over year"}); len(v) != 1 {
		t.Fatalf("digit-boundary check failed, got %+v", v)
	}
	if v := Check("improved 33%", []string{"a 33% improvement"}); v != nil {
		t.Fatalf("legitimate 33%% must pass, got %+v", v)
	}
}

func TestBootstrapDedupes(t *testing.T) {
	entries := Bootstrap("serving 1,000+ developers", "again 1,000+ developers", "and 33% faster")
	if len(entries) != 2 {
		t.Fatalf("entries = %v", entries)
	}
	for _, e := range entries {
		if strings.TrimSpace(e) == "" {
			t.Fatal("blank bootstrap entry")
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := t.TempDir() + "/claims.yaml"
	in := &File{Source: "test", Updated: "2026-07-16", Allowed: []string{"1,000+ developers"}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "test" || len(out.Allowed) != 1 || out.Allowed[0] != "1,000+ developers" {
		t.Fatalf("round trip = %+v", out)
	}
}
