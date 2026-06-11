package match

import (
	"testing"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/profile"
)

func testProfile() *profile.Profile {
	return &profile.Profile{
		Name: "Test Person",
		Skills: []profile.Skill{
			{Name: "Go", Level: "expert", Years: 5, Aliases: []string{"golang"}},
			{Name: "PostgreSQL", Level: "proficient", Years: 4},
		},
		Experience: []profile.Experience{{
			Company: "Acme", Role: "Engineer", Start: "2020-01",
			Bullets: []profile.Bullet{
				{Text: "Ran services on Kubernetes clusters.", Tags: []string{"k8s"}},
				{Text: "Wrote integration tests for the billing API."},
			},
		}},
	}
}

const jdText = `Senior Backend Engineer

Requirements
- Go and PostgreSQL in production
- Kubernetes
- Rust
`

func TestScoreCoverageAndEvidence(t *testing.T) {
	p := testProfile()
	j := jd.Parse(jdText)
	res := Score(p, j)

	ev := map[string]string{}
	for _, m := range res.Matched {
		ev[m.Name] = m.Evidence
	}
	if ev["Go"] != "declared" {
		t.Fatalf("Go evidence = %q, want declared", ev["Go"])
	}
	if ev["PostgreSQL"] != "declared" {
		t.Fatalf("PostgreSQL evidence = %q, want declared", ev["PostgreSQL"])
	}
	// Kubernetes is covered only via the k8s bullet tag.
	if ev["Kubernetes"] != "tagged" {
		t.Fatalf("Kubernetes evidence = %q, want tagged", ev["Kubernetes"])
	}

	missing := map[string]bool{}
	for _, m := range res.Missing {
		missing[m.Name] = true
	}
	if !missing["Rust"] {
		t.Fatal("Rust should be missing")
	}
	if res.Score <= 0 || res.Score >= 100 {
		t.Fatalf("score = %.1f, want strictly between 0 and 100", res.Score)
	}
}

func TestPerfectAndZeroScores(t *testing.T) {
	p := testProfile()
	full := jd.Parse("Requirements\n- Go\n- PostgreSQL\n")
	if got := Score(p, full).Score; got != 100 {
		t.Fatalf("full coverage score = %.1f, want 100", got)
	}
	none := jd.Parse("Requirements\n- Haskell\n- Elixir\n")
	if got := Score(p, none).Score; got != 0 {
		t.Fatalf("zero coverage score = %.1f, want 0", got)
	}
}
