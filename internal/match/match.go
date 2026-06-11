// Package match scores a profile against a parsed job description and
// produces a gap report: what matches (with evidence), what's missing, and
// concrete advice for tailoring the application.
package match

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/profile"
)

// MatchedSkill is a JD skill the profile covers.
type MatchedSkill struct {
	Name     string  `json:"name"`
	Weight   float64 `json:"weight"`
	Required bool    `json:"required"`
	Level    string  `json:"level,omitempty"`
	Years    float64 `json:"years,omitempty"`
	Evidence string  `json:"evidence"` // declared | tagged | text
}

// MissingSkill is a JD skill the profile doesn't cover.
type MissingSkill struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Weight   float64 `json:"weight"`
	Required bool    `json:"required"`
}

// Result is the full gap report.
type Result struct {
	Score     float64        `json:"score"` // 0..100 weighted coverage
	Seniority string         `json:"jd_seniority"`
	Matched   []MatchedSkill `json:"matched"`
	Missing   []MissingSkill `json:"missing"`
	Advice    []string       `json:"advice"`
}

// Score evaluates p against j.
func Score(p *profile.Profile, j *jd.JD) *Result {
	res := &Result{Seniority: j.Seniority}
	terms := p.SkillTerms()
	allText := strings.ToLower(p.AllText())

	var totalW, matchedW float64
	for _, hit := range j.Skills {
		totalW += hit.Weight
		evidence := evidenceFor(p, terms, allText, hit.Name)
		if evidence == "" {
			res.Missing = append(res.Missing, MissingSkill{
				Name: hit.Name, Category: hit.Category, Weight: hit.Weight, Required: hit.Required,
			})
			continue
		}
		matchedW += hit.Weight
		m := MatchedSkill{Name: hit.Name, Weight: hit.Weight, Required: hit.Required, Evidence: evidence}
		if s := p.SkillByTerm(hit.Name); s != nil {
			m.Level, m.Years = s.Level, s.Years
		}
		res.Matched = append(res.Matched, m)
	}
	if totalW > 0 {
		res.Score = 100 * matchedW / totalW
	}
	sort.Slice(res.Matched, func(a, b int) bool { return res.Matched[a].Weight > res.Matched[b].Weight })
	sort.Slice(res.Missing, func(a, b int) bool { return res.Missing[a].Weight > res.Missing[b].Weight })

	res.Advice = advise(res)
	return res
}

// evidenceFor reports how the profile covers a canonical JD skill:
// "declared" (skills list), "tagged" (bullet tags), "text" (mentioned in
// bullets/summary), or "" when absent. Checks every lexicon alias so a JD
// asking for "Kubernetes" matches a profile that says "k8s".
func evidenceFor(p *profile.Profile, terms map[string]bool, allText, canonical string) string {
	aliases := []string{strings.ToLower(canonical)}
	for _, e := range jd.Lexicon() {
		if e.Canonical == canonical {
			aliases = e.Aliases
			break
		}
	}
	for _, a := range aliases {
		if p.SkillByTerm(a) != nil {
			return "declared"
		}
	}
	for _, a := range aliases {
		if terms[a] {
			return "tagged"
		}
	}
	for _, a := range aliases {
		if containsWord(allText, a) {
			return "text"
		}
	}
	return ""
}

func containsWord(text, word string) bool {
	idx := 0
	for {
		i := strings.Index(text[idx:], word)
		if i < 0 {
			return false
		}
		pos := idx + i
		end := pos + len(word)
		beforeOK := pos == 0 || !isWordChar(text[pos-1])
		afterOK := end >= len(text) || !isWordChar(text[end])
		if beforeOK && afterOK {
			return true
		}
		idx = pos + len(word)
	}
}

func isWordChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '#'
}

func advise(r *Result) []string {
	var out []string
	var reqMissing []string
	for _, m := range r.Missing {
		if m.Required {
			reqMissing = append(reqMissing, m.Name)
		}
	}
	switch {
	case r.Score >= 75:
		out = append(out, "Strong match — apply, and lead your resume with the highest-weight matched skills.")
	case r.Score >= 50:
		out = append(out, "Decent match — tailor the resume to foreground matched skills and address the top gaps in the cover letter.")
	default:
		out = append(out, "Weak coverage — consider whether transferable experience can honestly bridge the gaps before applying.")
	}
	if len(reqMissing) > 0 {
		n := len(reqMissing)
		if n > 5 {
			reqMissing = reqMissing[:5]
		}
		out = append(out, fmt.Sprintf("Required-section gaps (%d): %s — address directly or expect screening filters.", n, strings.Join(reqMissing, ", ")))
	}
	var textOnly []string
	for _, m := range r.Matched {
		if m.Evidence == "text" {
			textOnly = append(textOnly, m.Name)
		}
	}
	if len(textOnly) > 0 {
		if len(textOnly) > 5 {
			textOnly = textOnly[:5]
		}
		out = append(out, fmt.Sprintf("Promote to declared skills (currently only implied in bullet text): %s.", strings.Join(textOnly, ", ")))
	}
	return out
}
