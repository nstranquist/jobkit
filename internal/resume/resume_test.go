package resume

import (
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/profile"
)

func testProfile() *profile.Profile {
	return &profile.Profile{
		Name:     "Test Person",
		Headline: "Software Engineer",
		Email:    "t@example.com",
		Skills: []profile.Skill{
			{Name: "TypeScript"},
			{Name: "Go", Aliases: []string{"golang"}},
		},
		Experience: []profile.Experience{{
			Company: "Acme", Role: "Engineer", Start: "2020-01", End: "present",
			Bullets: []profile.Bullet{
				{Text: "Led frontend migration to React."},
				{Text: "Built Go microservices.", Tags: []string{"go"}},
				{Text: "Organized team offsites."},
			},
		}},
		Education: []profile.Education{{School: "State U", Degree: "B.S.", Field: "CS", Year: "2018"}},
	}
}

func TestTailoringReordersSkillsAndBullets(t *testing.T) {
	p := testProfile()
	j := jd.Parse("Backend Engineer\n\nRequirements\n- Go\n")
	d := Build(p, j, Options{MaxBulletsPerRole: 2})

	if d.SkillOrder[0].Name != "Go" {
		t.Fatalf("first skill = %s, want Go", d.SkillOrder[0].Name)
	}
	bullets := d.Bullets[Key(p.Experience[0])]
	if len(bullets) != 2 {
		t.Fatalf("bullets = %d, want capped at 2", len(bullets))
	}
	if !strings.Contains(bullets[0].Text, "Go microservices") {
		t.Fatalf("top bullet = %q, want the Go one first", bullets[0].Text)
	}
}

func TestUntailoredKeepsOrder(t *testing.T) {
	p := testProfile()
	d := Build(p, nil, Options{})
	if d.Tailored {
		t.Fatal("nil JD must not mark tailored")
	}
	if d.SkillOrder[0].Name != "TypeScript" {
		t.Fatal("untailored build must keep declared skill order")
	}
	if got := len(d.Bullets[Key(p.Experience[0])]); got != 3 {
		t.Fatalf("bullets = %d, want all 3", got)
	}
}

func TestRenderersProduceAllSections(t *testing.T) {
	p := testProfile()
	d := Build(p, nil, Options{})
	md := RenderMarkdown(d)
	txt := RenderText(d)
	html := RenderHTML(d)
	for _, want := range []string{"Test Person", "Acme", "State U"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q", want)
		}
		if !strings.Contains(txt, want) {
			t.Fatalf("text missing %q", want)
		}
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q", want)
		}
	}
	if !strings.Contains(html, "<!doctype html>") {
		t.Fatal("html must be a complete document")
	}
	if strings.Contains(txt, "#") || strings.Contains(txt, "**") {
		t.Fatal("ATS text must not contain markdown markup")
	}
}
