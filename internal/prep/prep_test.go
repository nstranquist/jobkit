package prep

import (
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/profile"
)

func TestBuildSections(t *testing.T) {
	p := &profile.Profile{
		Name: "Test Person",
		Skills: []profile.Skill{
			{Name: "Go", Level: "expert", Years: 5},
			{Name: "PostgreSQL", Level: "proficient"},
		},
		Experience: []profile.Experience{{
			Company: "Acme", Role: "Engineer", Start: "2020-01",
			Bullets: []profile.Bullet{
				{Text: "Built a Go billing service handling 10k req/s.", Tags: []string{"go"}},
			},
		}},
	}
	j := jd.Parse("Senior Backend Engineer\nCompany: Initech\n\nRequirements\n- Go\n- Rust\n")
	res := match.Score(p, j)
	sheet := Build(p, j, res)

	for _, want := range []string{
		"# Interview prep — Senior Backend Engineer @ Initech",
		"## Likely technical deep-dives",
		"### Go",
		"*Your anchor story:* Built a Go billing service",
		"## Gap defense",
		"**Rust**",
		"## STAR story bank",
		"## Questions to ask them",
	} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("prep sheet missing %q\n---\n%s", want, sheet)
		}
	}
	// Rust gap should bridge from Go (same lexicon category: language).
	if !strings.Contains(sheet, "your Go depth") {
		t.Fatalf("expected Go bridge for Rust gap:\n%s", sheet)
	}
}

func TestNearestSkillUsesCompatibleBridgeFamilies(t *testing.T) {
	p := &profile.Profile{Skills: []profile.Skill{{Name: "RAG", Level: "expert"}, {Name: "Go", Level: "proficient"}}}
	if got := nearestSkill(p, "Concurrency", "domain"); got != "" {
		t.Fatalf("Concurrency bridge = %q, want no unrelated RAG bridge", got)
	}
	if got := nearestSkill(p, "LLM", "domain"); got != "RAG" {
		t.Fatalf("LLM bridge = %q, want RAG", got)
	}
}

func TestBuildUsesProjectBulletsAsEvidence(t *testing.T) {
	p := &profile.Profile{
		Name:     "Test Person",
		Skills:   []profile.Skill{{Name: "Go", Level: "expert"}},
		Projects: []profile.Project{{Name: "Catalog", Bullets: []profile.Bullet{{Text: "Built a Go catalog engine with deterministic indexing.", Tags: []string{"go"}}}}},
	}
	j := jd.Parse("Senior Go Engineer\n\nRequirements\n- Go\n")
	sheet := Build(p, j, match.Score(p, j))
	if !strings.Contains(sheet, "Your anchor story:*") || !strings.Contains(sheet, "Built a Go catalog engine") {
		t.Fatalf("project evidence missing:\n%s", sheet)
	}
}
