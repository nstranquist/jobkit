// Package letter drafts cover letters deterministically from the master
// profile and a parsed JD: it weaves the strongest matched skills (with the
// best supporting bullet for each) into a short, honest letter. No LLM
// required; the output is a strong first draft meant for human editing.
package letter

import (
	"fmt"
	"strings"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/profile"
)

// Options controls letter generation.
type Options struct {
	Company string // overrides JD-detected company
	Role    string // overrides JD-detected title
	Tone    string // professional (default) | warm | direct
	Manager string // hiring manager name if known
}

// Build produces the letter text.
func Build(p *profile.Profile, j *jd.JD, res *match.Result, opts Options) string {
	company := firstNonEmpty(opts.Company, j.Company, "your team")
	role := firstNonEmpty(opts.Role, j.Title, "this role")
	tone := strings.ToLower(firstNonEmpty(opts.Tone, "professional"))

	greeting := "Dear Hiring Manager,"
	if opts.Manager != "" {
		greeting = fmt.Sprintf("Dear %s,", opts.Manager)
	} else if tone == "warm" {
		greeting = fmt.Sprintf("Hello %s team,", company)
	}

	// Top matched skills, required-first.
	var top []match.MatchedSkill
	for _, m := range res.Matched {
		if m.Required {
			top = append(top, m)
		}
	}
	for _, m := range res.Matched {
		if !m.Required {
			top = append(top, m)
		}
	}
	if len(top) > 3 {
		top = top[:3]
	}

	var b strings.Builder
	b.WriteString(greeting + "\n\n")

	// Opening.
	years := maxYears(p)
	hook := p.Headline
	if hook == "" {
		hook = "software engineer"
	}
	switch tone {
	case "direct":
		fmt.Fprintf(&b, "I'm applying for the %s position at %s. ", role, company)
	case "warm":
		fmt.Fprintf(&b, "I was genuinely excited to see the %s opening at %s. ", role, company)
	default:
		fmt.Fprintf(&b, "I'm writing to apply for the %s position at %s. ", role, company)
	}
	if years > 0 {
		fmt.Fprintf(&b, "I'm a %s with %s years of experience, and the role lines up closely with the work I do best.\n\n", hook, trimFloat(years))
	} else {
		fmt.Fprintf(&b, "As a %s, the role lines up closely with the work I do best.\n\n", hook)
	}

	// Evidence paragraph: one sentence per top matched skill, anchored to the
	// most relevant bullet from experience.
	if len(top) > 0 {
		var names []string
		for _, m := range top {
			names = append(names, m.Name)
		}
		fmt.Fprintf(&b, "You're looking for depth in %s — that maps directly to my background:\n\n", joinNatural(names))
		for _, m := range top {
			if ev := bestBullet(p, m.Name); ev != "" {
				fmt.Fprintf(&b, "- %s: %s\n", m.Name, ev)
			} else if m.Years > 0 {
				fmt.Fprintf(&b, "- %s: %s years of hands-on use.\n", m.Name, trimFloat(m.Years))
			} else {
				fmt.Fprintf(&b, "- %s: hands-on production experience.\n", m.Name)
			}
		}
		b.WriteString("\n")
	}

	// Honest-gap paragraph when a required skill is missing (top one only).
	for _, miss := range res.Missing {
		if miss.Required {
			fmt.Fprintf(&b, "I'll be upfront that %s is newer territory for me; my track record of ramping quickly on adjacent tools means I'd close that gap fast.\n\n", miss.Name)
			break
		}
	}

	// Closing.
	switch tone {
	case "direct":
		fmt.Fprintf(&b, "I'd welcome a conversation about how I can contribute. Thank you for your time.\n\n")
	case "warm":
		fmt.Fprintf(&b, "I'd love to talk about how I could help %s ship great work. Thanks so much for considering me.\n\n", company)
	default:
		fmt.Fprintf(&b, "I would welcome the opportunity to discuss how my experience can contribute to %s. Thank you for your consideration.\n\n", company)
	}
	fmt.Fprintf(&b, "Sincerely,\n%s", p.Name)
	if p.Email != "" {
		fmt.Fprintf(&b, "\n%s", p.Email)
	}
	b.WriteString("\n")
	return b.String()
}

// bestBullet finds the experience bullet most relevant to a canonical skill
// (tag match wins, then text mention).
func bestBullet(p *profile.Profile, canonical string) string {
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
	return textHit
}

func maxYears(p *profile.Profile) float64 {
	best := 0.0
	for _, s := range p.Skills {
		if s.Years > best {
			best = s.Years
		}
	}
	return best
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	return strings.TrimSuffix(s, ".0")
}

func joinNatural(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
