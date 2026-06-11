// Package jd parses job descriptions: extracts title/company hints, detects
// skills against an embedded lexicon, weights them by section (requirements
// vs nice-to-have), and estimates seniority.
package jd

import (
	"bufio"
	_ "embed"
	"regexp"
	"sort"
	"strings"
)

//go:embed lexicon.txt
var lexiconRaw string

// LexEntry is one canonical skill with match aliases.
type LexEntry struct {
	Canonical string
	Aliases   []string // lowercased, includes canonical
	Category  string
}

var lexicon []LexEntry

func init() {
	for _, line := range strings.Split(lexiconRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		e := LexEntry{Canonical: parts[0], Category: parts[2]}
		e.Aliases = append(e.Aliases, strings.ToLower(parts[0]))
		for _, a := range strings.Split(parts[1], ",") {
			a = strings.ToLower(strings.TrimSpace(a))
			if a != "" {
				e.Aliases = append(e.Aliases, a)
			}
		}
		lexicon = append(lexicon, e)
	}
}

// Lexicon exposes the parsed lexicon (read-only by convention).
func Lexicon() []LexEntry { return lexicon }

// SkillHit is one detected skill with its evidence weight.
type SkillHit struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Weight   float64 `json:"weight"`   // section-weighted importance
	Required bool    `json:"required"` // seen in a requirements-flavored section
}

// JD is a parsed job description.
type JD struct {
	Title     string     `json:"title,omitempty"`
	Company   string     `json:"company,omitempty"`
	Seniority string     `json:"seniority"`
	Skills    []SkillHit `json:"skills"`
	Raw       string     `json:"-"`
}

var (
	reqHeader  = regexp.MustCompile(`(?i)^\s*#*\s*(requirements?|qualifications?|must[- ]haves?|what (you('ll)? (bring|have)|we('re| are) looking for)|you (have|bring|are)|minimum qualifications)\b`)
	niceHeader = regexp.MustCompile(`(?i)^\s*#*\s*(nice[- ]to[- ]haves?|bonus( points)?|preferred( qualifications)?|plus(es)?|extra credit)\b`)
	anyHeader  = regexp.MustCompile(`(?i)^\s*#*\s*(about|responsibilities|what you('ll)? do|the role|benefits|perks|compensation|who we are|our (team|stack|mission)|overview|description)\b`)
	titleLine  = regexp.MustCompile(`(?i)^\s*(?:job\s*)?title\s*[:\-]\s*(.+)$`)
	companyLn  = regexp.MustCompile(`(?i)^\s*company\s*[:\-]\s*(.+)$`)
	// '.'/'/' are NOT word chars: "k8s." and "ci/cd," must still match at
	// sentence boundaries. Aliases containing them ("next.js") still match
	// whole, since boundaries are only checked outside the alias.
	wordChar = regexp.MustCompile(`[a-z0-9+#]`)
)

// Parse analyzes raw JD text.
func Parse(raw string) *JD {
	j := &JD{Raw: raw}
	lower := strings.ToLower(raw)

	// Title/company: explicit "Title:"/"Company:" lines win; else first
	// non-empty line that looks like a role title.
	sc := bufio.NewScanner(strings.NewReader(raw))
	firstLine := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if m := titleLine.FindStringSubmatch(line); m != nil && j.Title == "" {
			j.Title = strings.TrimSpace(m[1])
		}
		if m := companyLn.FindStringSubmatch(line); m != nil && j.Company == "" {
			j.Company = strings.TrimSpace(m[1])
		}
		if firstLine == "" {
			firstLine = line
		}
	}
	if j.Title == "" && looksLikeTitle(firstLine) {
		j.Title = firstLine
	}

	// Section-weighted skill scan.
	weights := sectionWeights(raw)
	counts := map[string]*SkillHit{}
	for _, e := range lexicon {
		for _, alias := range e.Aliases {
			for _, pos := range findAll(lower, alias) {
				w := weightAt(weights, pos)
				h, ok := counts[e.Canonical]
				if !ok {
					h = &SkillHit{Name: e.Canonical, Category: e.Category}
					counts[e.Canonical] = h
				}
				h.Count++
				h.Weight += w
				if w >= 2.0 {
					h.Required = true
				}
			}
		}
	}
	for _, h := range counts {
		j.Skills = append(j.Skills, *h)
	}
	sort.Slice(j.Skills, func(a, b int) bool {
		if j.Skills[a].Weight != j.Skills[b].Weight {
			return j.Skills[a].Weight > j.Skills[b].Weight
		}
		return j.Skills[a].Name < j.Skills[b].Name
	})

	j.Seniority = detectSeniority(j.Title, lower)
	return j
}

func looksLikeTitle(line string) bool {
	if line == "" || len(line) > 90 {
		return false
	}
	l := strings.ToLower(line)
	for _, kw := range []string{"engineer", "developer", "architect", "scientist", "manager", "lead", "designer", "sre", "swe"} {
		if strings.Contains(l, kw) {
			return true
		}
	}
	return false
}

// sectionWeights maps byte offsets → weight by walking section headers.
type span struct {
	start  int
	weight float64
}

func sectionWeights(raw string) []span {
	spans := []span{{0, 1.0}}
	off := 0
	for _, line := range strings.SplitAfter(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case reqHeader.MatchString(trimmed):
			spans = append(spans, span{off, 2.0})
		case niceHeader.MatchString(trimmed):
			spans = append(spans, span{off, 0.75})
		case anyHeader.MatchString(trimmed):
			spans = append(spans, span{off, 1.0})
		}
		off += len(line)
	}
	return spans
}

func weightAt(spans []span, pos int) float64 {
	w := 1.0
	for _, s := range spans {
		if pos >= s.start {
			w = s.weight
		}
	}
	return w
}

// findAll returns positions of alias in text with word-boundary checks, so
// "go" doesn't match inside "google" and "react" doesn't match "reaction".
func findAll(text, alias string) []int {
	var out []int
	start := 0
	for {
		i := strings.Index(text[start:], alias)
		if i < 0 {
			return out
		}
		pos := start + i
		end := pos + len(alias)
		beforeOK := pos == 0 || !wordChar.MatchString(string(text[pos-1]))
		afterOK := end >= len(text) || !wordChar.MatchString(string(text[end]))
		if beforeOK && afterOK {
			out = append(out, pos)
		}
		start = pos + len(alias)
	}
}

func detectSeniority(title, lowerBody string) string {
	t := strings.ToLower(title)
	combined := t + " " + lowerBody
	switch {
	case containsAny(t, "principal", "staff", "distinguished"):
		return "staff+"
	case containsAny(t, "senior", "sr.", "sr ", "lead"):
		return "senior"
	case containsAny(t, "junior", "entry", "intern", "graduate", "new grad"):
		return "junior"
	case containsAny(combined, "8+ years", "10+ years"):
		return "staff+"
	case containsAny(combined, "5+ years", "6+ years", "7+ years", "senior"):
		return "senior"
	case containsAny(combined, "0-2 years", "1+ year", "entry level", "entry-level"):
		return "junior"
	default:
		return "mid"
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
