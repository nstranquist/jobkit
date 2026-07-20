package letter

import (
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/profile"
)

func sampleProfile() *profile.Profile {
	return &profile.Profile{
		Name:     "Sam Rivera",
		Headline: "Senior Software Engineer | Platform & AI",
		Email:    "sam@example.com",
		Skills: []profile.Skill{
			{Name: "Go", Level: "expert", Years: 6, Aliases: []string{"golang"}},
			{Name: "PostgreSQL", Level: "expert", Years: 6, Aliases: []string{"postgres"}},
			{Name: "Kubernetes", Level: "proficient", Years: 3, Aliases: []string{"k8s"}},
		},
		Experience: []profile.Experience{
			{
				Company: "Vandelay Industries",
				Role:    "Senior Software Engineer",
				Bullets: []profile.Bullet{
					{Text: "Designed a Go payments-ledger service with exactly-once settlement.", Tags: []string{"go", "postgresql"}},
					{Text: "Migrated 14 services to Kubernetes on EKS.", Tags: []string{"kubernetes", "aws"}},
				},
			},
		},
	}
}

func sampleJD() *jd.JD {
	return &jd.JD{
		Title:   "Senior Backend Engineer",
		Company: "Initech",
		Raw:     "Requirements: Go, PostgreSQL, Kubernetes. Nice to have: Rust.",
	}
}

func sampleMatch() *match.Result {
	return &match.Result{
		Score: 82,
		Matched: []match.MatchedSkill{
			{Name: "Go", Required: true, Years: 6},
			{Name: "PostgreSQL", Required: true, Years: 6},
			{Name: "Kubernetes", Required: true, Years: 3},
			{Name: "React", Required: false, Years: 1},
		},
		Missing: []match.MissingSkill{
			{Name: "Rust", Required: false},
		},
	}
}

func TestBuildProfessionalTone(t *testing.T) {
	out := Build(sampleProfile(), sampleJD(), sampleMatch(), Options{})
	for _, want := range []string{
		"Dear Hiring Manager,",
		"Senior Backend Engineer",
		"Initech",
		"Senior Software Engineer", // headline first segment only
		"Go",
		"payments-ledger",
		"Sincerely,\nSam Rivera",
		"sam@example.com",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("letter missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Platform & AI") {
		t.Fatalf("headline pipe segment should not appear mid-sentence:\n%s", out)
	}
	// Only top 3 matched skills in evidence list.
	if strings.Count(out, "- ") > 3 {
		t.Fatalf("expected ≤3 evidence bullets, got:\n%s", out)
	}
}

func TestBuildWarmToneAndManager(t *testing.T) {
	out := Build(sampleProfile(), sampleJD(), sampleMatch(), Options{
		Tone:    "warm",
		Manager: "Alex Chen",
	})
	if !strings.Contains(out, "Dear Alex Chen,") {
		t.Fatalf("manager greeting missing:\n%s", out)
	}
	// Manager overrides warm company greeting.
	if strings.Contains(out, "Hello Initech team") {
		t.Fatalf("warm company greeting should not win over manager:\n%s", out)
	}
	if !strings.Contains(out, "I'd love to talk") {
		t.Fatalf("warm closing missing:\n%s", out)
	}
}

func TestBuildDirectToneAndHonestGap(t *testing.T) {
	res := sampleMatch()
	res.Missing = []match.MissingSkill{{Name: "gRPC", Required: true}}
	out := Build(sampleProfile(), sampleJD(), res, Options{Tone: "direct"})
	if !strings.Contains(out, "I'm applying for") {
		t.Fatalf("direct opening missing:\n%s", out)
	}
	if !strings.Contains(out, "gRPC is newer territory") {
		t.Fatalf("honest required-gap paragraph missing:\n%s", out)
	}
	if !strings.Contains(out, "I'd welcome a conversation") {
		t.Fatalf("direct closing missing:\n%s", out)
	}
}

func TestBuildOmitsNonRequiredGap(t *testing.T) {
	out := Build(sampleProfile(), sampleJD(), sampleMatch(), Options{})
	if strings.Contains(out, "Rust is newer territory") {
		t.Fatalf("nice-to-have gaps should not trigger honest-gap paragraph:\n%s", out)
	}
}

func TestJoinNatural(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"Go"}, "Go"},
		{[]string{"Go", "PostgreSQL"}, "Go and PostgreSQL"},
		{[]string{"Go", "PostgreSQL", "Kubernetes"}, "Go, PostgreSQL, and Kubernetes"},
	}
	for _, tc := range cases {
		if got := joinNatural(tc.in); got != tc.want {
			t.Fatalf("joinNatural(%v)=%q want %q", tc.in, got, tc.want)
		}
	}
}
