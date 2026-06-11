package jd

import "testing"

const sampleJD = `Senior Backend Engineer
Company: Initech

About the role
We build payment infrastructure used by thousands of teams.

Requirements
- 5+ years of experience with Go and PostgreSQL
- Production Kubernetes experience
- Strong testing culture

Nice to have
- Rust
- Terraform
`

func TestParseExtractsTitleCompanySeniority(t *testing.T) {
	j := Parse(sampleJD)
	if j.Title != "Senior Backend Engineer" {
		t.Fatalf("title = %q", j.Title)
	}
	if j.Company != "Initech" {
		t.Fatalf("company = %q", j.Company)
	}
	if j.Seniority != "senior" {
		t.Fatalf("seniority = %q", j.Seniority)
	}
}

func TestRequirementsWeighting(t *testing.T) {
	j := Parse(sampleJD)
	skills := map[string]SkillHit{}
	for _, s := range j.Skills {
		skills[s.Name] = s
	}
	goHit, ok := skills["Go"]
	if !ok {
		t.Fatal("Go not detected")
	}
	if !goHit.Required {
		t.Fatal("Go should be flagged required (requirements section)")
	}
	rust, ok := skills["Rust"]
	if !ok {
		t.Fatal("Rust not detected")
	}
	if rust.Required {
		t.Fatal("Rust is nice-to-have, must not be required")
	}
	if rust.Weight >= goHit.Weight {
		t.Fatalf("nice-to-have Rust (%.2f) should weigh less than required Go (%.2f)", rust.Weight, goHit.Weight)
	}
}

func TestWordBoundaries(t *testing.T) {
	j := Parse("We use Google Sheets and react to feedback.")
	for _, s := range j.Skills {
		if s.Name == "Go" {
			t.Fatal("'Go' must not match inside 'Google'")
		}
	}
	// "react to feedback" DOES contain the bare word react — lexicon matches
	// it; this is accepted noise. But "golang" alias must match Go:
	j2 := Parse("Experience with golang required.")
	found := false
	for _, s := range j2.Skills {
		if s.Name == "Go" {
			found = true
		}
	}
	if !found {
		t.Fatal("alias 'golang' should map to canonical Go")
	}
}

func TestNoDoubleCountOnOverlappingAliases(t *testing.T) {
	// One "gRPC" mention must count once (canonical + alias share the span);
	// "ruby on rails" must credit Rails once and NOT credit Ruby separately;
	// "react native" must not also credit React.
	j := Parse("We use gRPC, ruby on rails, and react native.")
	got := map[string]SkillHit{}
	for _, s := range j.Skills {
		got[s.Name] = s
	}
	if g := got["gRPC"]; g.Count != 1 {
		t.Fatalf("gRPC count = %d, want 1", g.Count)
	}
	if r := got["Rails"]; r.Count != 1 {
		t.Fatalf("Rails count = %d, want 1", r.Count)
	}
	if _, ok := got["Ruby"]; ok {
		t.Fatal("Ruby must not be credited from inside 'ruby on rails'")
	}
	if _, ok := got["React"]; ok {
		t.Fatal("React must not be credited from inside 'react native'")
	}
	if rn := got["React Native"]; rn.Count != 1 {
		t.Fatalf("React Native count = %d, want 1", rn.Count)
	}
}

func TestKubernetesAlias(t *testing.T) {
	j := Parse("You will run workloads on k8s.")
	found := false
	for _, s := range j.Skills {
		if s.Name == "Kubernetes" {
			found = true
		}
	}
	if !found {
		t.Fatal("k8s should map to Kubernetes")
	}
}
