// Package resume builds tailored resumes: given the master profile and an
// optional parsed JD, it reorders skills and ranks bullets by relevance, then
// renders to Markdown, ATS-safe plain text, or print-ready HTML.
package resume

import (
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/profile"
)

// Options controls tailoring.
type Options struct {
	MaxBulletsPerRole int  // 0 = keep all
	TailorOrder       bool // reorder bullets/skills by JD relevance
}

// Doc is a render-ready resume.
type Doc struct {
	Profile       *profile.Profile
	SkillOrder    []profile.Skill             // skills in render order
	Bullets       map[string][]profile.Bullet // key: company+"\x00"+role
	TargetTitle   string
	TargetCompany string
	Tailored      bool
}

// Key identifies an experience entry in Doc.Bullets.
func Key(e profile.Experience) string { return e.Company + "\x00" + e.Role }

// Build tailors p toward j (j may be nil for the full general resume).
func Build(p *profile.Profile, j *jd.JD, opts Options) *Doc {
	d := &Doc{Profile: p, Bullets: map[string][]profile.Bullet{}}

	// Relevance terms from the JD: canonical names + all lexicon aliases.
	rel := map[string]float64{}
	if j != nil {
		d.Tailored = true
		d.TargetTitle = j.Title
		d.TargetCompany = j.Company
		for _, hit := range j.Skills {
			for _, e := range jd.Lexicon() {
				if e.Canonical == hit.Name {
					for _, a := range e.Aliases {
						if hit.Weight > rel[a] {
							rel[a] = hit.Weight
						}
					}
				}
			}
		}
	}

	// Skills: matched-first (by JD weight desc), then the rest in declared order.
	d.SkillOrder = append([]profile.Skill{}, p.Skills...)
	if len(rel) > 0 {
		sort.SliceStable(d.SkillOrder, func(a, b int) bool {
			return skillRelevance(d.SkillOrder[a], rel) > skillRelevance(d.SkillOrder[b], rel)
		})
	}

	// Bullets per role: score by term overlap; stable so original order is
	// the tiebreak. Cap at MaxBulletsPerRole when tailoring.
	for _, e := range p.Experience {
		bullets := append([]profile.Bullet{}, e.Bullets...)
		if len(rel) > 0 {
			sort.SliceStable(bullets, func(a, b int) bool {
				return bulletRelevance(bullets[a], rel) > bulletRelevance(bullets[b], rel)
			})
		}
		if opts.MaxBulletsPerRole > 0 && len(bullets) > opts.MaxBulletsPerRole {
			bullets = bullets[:opts.MaxBulletsPerRole]
		}
		d.Bullets[Key(e)] = bullets
	}
	return d
}

func skillRelevance(s profile.Skill, rel map[string]float64) float64 {
	best := rel[strings.ToLower(s.Name)]
	for _, a := range s.Aliases {
		if w := rel[strings.ToLower(a)]; w > best {
			best = w
		}
	}
	return best
}

func bulletRelevance(b profile.Bullet, rel map[string]float64) float64 {
	score := 0.0
	for _, t := range b.Tags {
		score += rel[strings.ToLower(t)]
	}
	lower := strings.ToLower(b.Text)
	for term, w := range rel {
		if strings.Contains(lower, term) {
			score += w * 0.5
		}
	}
	return score
}
