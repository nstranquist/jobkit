// Package prep generates an interview-preparation sheet from the profile +
// JD + match result: technical deep-dive questions per matched skill, gap
// defenses for missing requirements, a STAR story bank from the candidate's
// own bullets, and questions to ask the interviewer. Deterministic markdown.
package prep

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/profile"
)

// questionTemplates per lexicon category; %s is the skill name.
var questionTemplates = map[string][]string{
	"language": {
		"Walk me through the most complex system you've built in %s. What would you redesign today?",
		"What sharp edges of %s have bitten you in production, and how do you guard against them now?",
	},
	"framework": {
		"How do you structure a large codebase using %s so it stays maintainable?",
		"When would you advise a team NOT to use %s?",
	},
	"infra": {
		"Describe an incident involving %s — from detection through postmortem. What changed afterward?",
		"How do you keep %s configuration reviewable and safe to change?",
	},
	"cloud": {
		"How have you controlled cost and blast radius running on %s?",
		"Sketch the architecture of something you shipped on %s — where were the failure points?",
	},
	"data": {
		"How did you model and scale data in %s? Walk through a migration you ran without downtime.",
		"What's your approach to debugging a slow query / hot partition in %s?",
	},
	"practice": {
		"How do you approach %s on a team — concretely, the last time it mattered?",
	},
	"domain": {
		"Tell me about your deepest work in %s. What do most engineers get wrong about it?",
	},
	"tool": {
		"How does %s fit into your daily workflow? Any power-user habits worth stealing?",
	},
}

// Build renders the prep sheet as markdown.
func Build(p *profile.Profile, j *jd.JD, res *match.Result) string {
	var b strings.Builder
	role := j.Title
	if role == "" {
		role = "the role"
	}
	fmt.Fprintf(&b, "# Interview prep — %s", role)
	if j.Company != "" {
		fmt.Fprintf(&b, " @ %s", j.Company)
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Match score **%.0f/100** · JD seniority **%s** · prepared for %s\n\n", res.Score, j.Seniority, p.Name)

	// 1. Technical deep dives: top matched skills, required first.
	var ordered []match.MatchedSkill
	for _, m := range res.Matched {
		if m.Required {
			ordered = append(ordered, m)
		}
	}
	for _, m := range res.Matched {
		if !m.Required {
			ordered = append(ordered, m)
		}
	}
	if len(ordered) > 6 {
		ordered = ordered[:6]
	}
	if len(ordered) > 0 {
		b.WriteString("## Likely technical deep-dives\n\n")
		b.WriteString("They will probe the skills they asked for and you claim. Have one concrete story ready per question.\n\n")
		for _, m := range ordered {
			cat := categoryOf(m.Name)
			qs := questionTemplates[cat]
			if len(qs) == 0 {
				qs = questionTemplates["domain"]
			}
			fmt.Fprintf(&b, "### %s\n", m.Name)
			for _, q := range qs {
				fmt.Fprintf(&b, "- %s\n", fmt.Sprintf(q, m.Name))
			}
			if ev := bestBulletFor(p, m.Name); ev != "" {
				fmt.Fprintf(&b, "- *Your anchor story:* %s\n", ev)
			}
			b.WriteString("\n")
		}
	}

	// 2. Gap defense: missing required skills.
	var gaps []match.MissingSkill
	for _, m := range res.Missing {
		if m.Required {
			gaps = append(gaps, m)
		}
	}
	if len(gaps) > 4 {
		gaps = gaps[:4]
	}
	if len(gaps) > 0 {
		b.WriteString("## Gap defense\n\n")
		b.WriteString("Required skills you don't (yet) show. Don't bluff — bridge from the nearest thing you do have, then show ramp speed.\n\n")
		for _, g := range gaps {
			bridge := nearestSkill(p, g.Name, g.Category)
			if bridge != "" {
				fmt.Fprintf(&b, "- **%s** — expect \"what's your experience with %s?\" Bridge: your %s depth covers the same fundamentals; name the overlap, then a concrete plan to close the rest.\n", g.Name, g.Name, bridge)
			} else {
				fmt.Fprintf(&b, "- **%s** — expect \"what's your experience with %s?\" Answer with your fastest comparable ramp-up and what you'd do in week one.\n", g.Name, g.Name)
			}
		}
		b.WriteString("\n")
	}

	// 3. STAR bank: the candidate's most JD-relevant bullets.
	stories := topBullets(p, j, 4)
	if len(stories) > 0 {
		b.WriteString("## STAR story bank\n\n")
		b.WriteString("Rehearse each as Situation → Task → Action → Result, with one number per story.\n\n")
		for i, s := range stories {
			fmt.Fprintf(&b, "%d. %s\n", i+1, s)
		}
		b.WriteString("\n")
	}

	// 4. Questions to ask them.
	b.WriteString("## Questions to ask them\n\n")
	asks := []string{
		"What does the first 90 days look like for this role — what should be true by then?",
		"What's the messiest part of the codebase or system right now, and is there appetite to fix it?",
		"How do changes get to production? Walk me through the last incident.",
		"How is engineering work prioritized, and who decides?",
	}
	switch j.Seniority {
	case "senior", "staff+":
		asks = append(asks, "What decisions would I own outright, and which need consensus?")
	case "junior":
		asks = append(asks, "How is mentorship structured — who would I learn from day to day?")
	}
	for _, a := range asks {
		fmt.Fprintf(&b, "- %s\n", a)
	}
	return b.String()
}

func categoryOf(canonical string) string {
	for _, e := range jd.Lexicon() {
		if e.Canonical == canonical {
			return e.Category
		}
	}
	return "domain"
}

// bestBulletFor mirrors letter.bestBullet but stays local to avoid an
// import cycle risk later; tag match wins, then text mention.
func bestBulletFor(p *profile.Profile, canonical string) string {
	aliases := []string{strings.ToLower(canonical)}
	for _, e := range jd.Lexicon() {
		if e.Canonical == canonical {
			aliases = e.Aliases
			break
		}
	}
	var textHit string
	for _, e := range p.Experience {
		for _, bl := range e.Bullets {
			for _, t := range bl.Tags {
				for _, a := range aliases {
					if strings.EqualFold(t, a) {
						return bl.Text
					}
				}
			}
			lower := strings.ToLower(bl.Text)
			for _, a := range aliases {
				if textHit == "" && strings.Contains(lower, a) {
					textHit = bl.Text
				}
			}
		}
	}
	for _, project := range p.Projects {
		for _, bl := range project.Bullets {
			for _, tag := range bl.Tags {
				for _, alias := range aliases {
					if strings.EqualFold(tag, alias) {
						return bl.Text
					}
				}
			}
			lower := strings.ToLower(bl.Text)
			for _, alias := range aliases {
				if textHit == "" && strings.Contains(lower, alias) {
					textHit = bl.Text
				}
			}
		}
	}
	return textHit
}

// nearestSkill picks the candidate's strongest declared skill in a compatible
// bridge family. Broad lexicon categories alone are not enough: RAG and
// Concurrency are both "domain" skills, but one is not honest bridge material
// for the other.
func nearestSkill(p *profile.Profile, missingName, category string) string {
	byName := map[string]struct {
		canonical string
		category  string
	}{}
	for _, e := range jd.Lexicon() {
		entry := struct {
			canonical string
			category  string
		}{e.Canonical, e.Category}
		byName[strings.ToLower(e.Canonical)] = entry
		for _, a := range e.Aliases {
			byName[a] = entry
		}
	}
	wantFamily := bridgeFamily(missingName, category)
	if wantFamily == "" {
		return ""
	}
	for _, s := range p.TopSkills(len(p.Skills)) {
		entry, ok := byName[strings.ToLower(s.Name)]
		if ok && bridgeFamily(entry.canonical, entry.category) == wantFamily {
			return s.Name
		}
	}
	return ""
}

func bridgeFamily(canonical, category string) string {
	canonical = strings.ToLower(strings.TrimSpace(canonical))
	if category != "domain" && category != "practice" {
		return category
	}
	groups := map[string][]string{
		"ai":                    {"machine learning", "deep learning", "llm", "generative ai", "rag", "prompt engineering", "nlp", "computer vision", "mlops", "ai agents", "mcp", "embeddings", "fine-tuning"},
		"platform-architecture": {"distributed systems", "backend", "devops", "sre", "platform engineering", "networking", "concurrency", "embedded"},
		"product-client":        {"frontend", "full stack", "ios", "android", "mobile", "accessibility"},
		"security":              {"security", "authentication", "cryptography"},
		"quality":               {"testing", "tdd", "e2e testing", "code review", "debugging", "refactoring"},
		"architecture":          {"system design", "api design", "performance", "scalability", "functional programming"},
		"operations":            {"observability", "incident response"},
		"collaboration":         {"agile", "documentation", "mentoring", "technical leadership", "cross-functional collaboration"},
	}
	for family, names := range groups {
		for _, name := range names {
			if canonical == name {
				return family
			}
		}
	}
	return ""
}

// topBullets ranks all experience bullets by JD-skill overlap.
func topBullets(p *profile.Profile, j *jd.JD, n int) []string {
	rel := map[string]float64{}
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
	type scored struct {
		text  string
		score float64
	}
	var all []scored
	for _, e := range p.Experience {
		for _, bl := range e.Bullets {
			s := 0.0
			for _, t := range bl.Tags {
				s += rel[strings.ToLower(t)]
			}
			lower := strings.ToLower(bl.Text)
			for term, w := range rel {
				if strings.Contains(lower, term) {
					s += w * 0.5
				}
			}
			if s > 0 {
				all = append(all, scored{bl.Text, s})
			}
		}
	}
	for _, project := range p.Projects {
		for _, bl := range project.Bullets {
			s := 0.0
			for _, tag := range bl.Tags {
				s += rel[strings.ToLower(tag)]
			}
			lower := strings.ToLower(bl.Text)
			for term, weight := range rel {
				if strings.Contains(lower, term) {
					s += weight * 0.5
				}
			}
			if s > 0 {
				all = append(all, scored{bl.Text, s})
			}
		}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].score > all[b].score })
	if len(all) > n {
		all = all[:n]
	}
	var out []string
	for _, s := range all {
		out = append(out, s.text)
	}
	return out
}
