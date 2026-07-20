// Package claims is the fact-lock gate for generated application material.
//
// The riskiest thing a resume/letter pipeline can do is emit a quantified
// claim ("1,000+ developers", "33%", "7 years") that the applicant cannot
// defend. Generation in jobkit is deterministic, so the drift vector is the
// profile (and any future template/LLM change). This package maintains an
// allowlist of verified quantified claims (~/.jobkit/claims.yaml) and checks
// any text against it: every quantified token in the text must appear in
// some allowlisted claim, or the check fails closed with the exact
// violations. Prose is the profile's responsibility; numbers are the
// liability, so numbers are what the gate locks.
package claims

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is the on-disk allowlist.
type File struct {
	// Source documents where these claims were verified (free text),
	// e.g. "nicos-resume v1.7.3 fact lock (facts/2026-06-25-fact-lock.md)".
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	// Updated is a human-maintained date stamp for the last review.
	Updated string `yaml:"updated,omitempty" json:"updated,omitempty"`
	// Allowed holds verified claims. A text token passes when its
	// normalized form appears inside any allowed entry.
	Allowed []string `yaml:"allowed" json:"allowed"`
}

// Violation is one quantified token with no allowlisted claim behind it.
type Violation struct {
	Token   string `json:"token"`
	Context string `json:"context"` // surrounding words from the checked text
}

// quantified matches the claim shapes the gate cares about, most specific
// first: percentages, comma/plus figures, money, year spans, bare 3+ digit
// numbers. Calendar years (19xx/20xx) are excluded later — dates are locked
// by the profile timeline, not this gate, and flagging every "2026" would
// bury real violations.
var quantified = regexp.MustCompile(
	`(?i)\$\d[\d,]*(?:\.\d+)?[km]?|\d+(?:\.\d+)?%|\d[\d,]*\+|\d+(?:\.\d+)?\s*(?:years?|yrs?)\b|\b\d{3,}\b`)

var yearLike = regexp.MustCompile(`^(19|20)\d{2}$`)

// Contact details and links are identity, not achievement claims; blank
// them before extraction so phone digits and URL fragments don't pollute
// the gate.
var (
	phoneLike = regexp.MustCompile(`\+?\d?[\s.-]?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}`)
	emailLike = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	urlLike   = regexp.MustCompile(`(?:https?://|www\.)\S+|\b[a-z0-9.-]+\.(?:com|dev|io|app|org|net)/\S*`)
)

// Extract returns the normalized quantified tokens found in text, deduped,
// with one context snippet each.
func Extract(text string) []Violation {
	for _, re := range []*regexp.Regexp{urlLike, emailLike, phoneLike} {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			return strings.Repeat(" ", len(m))
		})
	}
	seen := map[string]bool{}
	var out []Violation
	for _, loc := range quantified.FindAllStringIndex(text, -1) {
		raw := text[loc[0]:loc[1]]
		token := Normalize(raw)
		if token == "" || yearLike.MatchString(token) || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, Violation{Token: token, Context: contextAround(text, loc[0], loc[1])})
	}
	return out
}

// Normalize canonicalizes a quantified token: lowercase, commas stripped,
// whitespace collapsed, "yrs"→"years".
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ",", "")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "yrs", "years")
	s = strings.ReplaceAll(s, "yr", "year")
	return s
}

// Check returns the quantified tokens in text that no allowed claim covers.
// A token is covered when it appears (normalized substring) in any allowed
// entry, or when its plus-marked form does: a verified "7+ years" entails a
// claimed "7 years" (at least seven covers exactly-stated seven), never the
// reverse. Percentages and money stay exact-match. nil means clean.
func Check(text string, allowed []string) []Violation {
	normalized := make([]string, len(allowed))
	for i, a := range allowed {
		normalized[i] = Normalize(a)
	}
	var out []Violation
	for _, v := range Extract(text) {
		candidates := []string{v.Token}
		if plus := plusVariant(v.Token); plus != "" {
			candidates = append(candidates, plus)
		}
		covered := false
		for _, a := range normalized {
			for _, cand := range candidates {
				if containsToken(a, cand) {
					covered = true
					break
				}
			}
			if covered {
				break
			}
		}
		if !covered {
			out = append(out, v)
		}
	}
	return out
}

// containsToken reports whether allowed contains token at digit boundaries:
// the characters immediately before and after the match must not extend the
// number, so an allowed "133%" never covers a claimed "33%" and an allowed
// "10000" never covers a claimed "1000".
func containsToken(allowed, token string) bool {
	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }
	for from := 0; ; {
		i := strings.Index(allowed[from:], token)
		if i < 0 {
			return false
		}
		at := from + i
		end := at + len(token)
		beforeOK := at == 0 || (!isDigit(allowed[at-1]) && allowed[at-1] != '.')
		afterOK := end >= len(allowed) ||
			(!isDigit(allowed[end]) && !(allowed[end] == '.' && end+1 < len(allowed) && isDigit(allowed[end+1])))
		if beforeOK && afterOK {
			return true
		}
		from = at + 1
	}
}

// plusVariant returns the "N+" form of a bare-number or years token
// ("7 years" → "7+ years", "1000" → "1000+"), or "" when plus-entailment
// does not apply (percentages, money, already-plus tokens).
func plusVariant(token string) string {
	if strings.ContainsAny(token, "%$+") {
		return ""
	}
	i := 0
	for i < len(token) && (token[i] >= '0' && token[i] <= '9' || token[i] == '.') {
		i++
	}
	if i == 0 {
		return ""
	}
	return token[:i] + "+" + token[i:]
}

// Load reads the allowlist; a missing file returns (nil, os.ErrNotExist)
// so callers can treat the gate as not-configured.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// Save writes the allowlist.
func Save(path string, f *File) error {
	raw, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	header := "# jobkit claims allowlist — every quantified claim in generated\n" +
		"# resumes/letters must trace to an entry here. Add a claim ONLY after\n" +
		"# verifying it; this file is the fact lock for application material.\n"
	return os.WriteFile(path, append([]byte(header), raw...), 0o644)
}

// Bootstrap builds an initial allowlist from source texts (typically the
// reviewed profile): each quantified token becomes one allowed entry with
// its context, for the human to curate.
func Bootstrap(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, text := range texts {
		for _, v := range Extract(text) {
			if seen[v.Token] {
				continue
			}
			seen[v.Token] = true
			out = append(out, strings.TrimSpace(v.Context))
		}
	}
	sort.Strings(out)
	return out
}

// contextAround returns up to a few words either side of [start,end).
func contextAround(text string, start, end int) string {
	const window = 40
	lo := start - window
	if lo < 0 {
		lo = 0
	}
	hi := end + window
	if hi > len(text) {
		hi = len(text)
	}
	snippet := text[lo:hi]
	// Trim partial words at the cut points.
	if lo > 0 {
		if i := strings.IndexAny(snippet, " \n\t"); i >= 0 {
			snippet = snippet[i+1:]
		}
	}
	if hi < len(text) {
		if i := strings.LastIndexAny(snippet, " \n\t"); i >= 0 {
			snippet = snippet[:i]
		}
	}
	return strings.Join(strings.Fields(snippet), " ")
}
