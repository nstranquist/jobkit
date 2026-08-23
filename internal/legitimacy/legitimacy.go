// Package legitimacy classifies whether a posting looks like a real opening.
// The result is qualitative and must never change JobKit's 0–100 fit score.
package legitimacy

import (
	"regexp"
	"strings"
)

type Level string

const (
	High       Level = "high"
	Caution    Level = "caution"
	Suspicious Level = "suspicious"
)

// Result is a posting-legitimacy assessment (career-ops Block G analogue).
type Result struct {
	Level   Level    `json:"level"`
	Signals []string `json:"signals,omitempty"`
}

// Classify inspects posting text only. No HTTP, no web search, no score input.
func Classify(text string) Result {
	res := Result{Level: High}
	lower := strings.ToLower(text)
	add := func(level Level, signal string) {
		res.Signals = append(res.Signals, signal)
		if worse(level, res.Level) {
			res.Level = level
		}
	}

	for _, rule := range suspiciousRules {
		if rule.re.MatchString(text) {
			add(Suspicious, rule.signal)
		}
	}
	for _, rule := range cautionRules {
		if rule.re.MatchString(text) {
			add(Caution, rule.signal)
		}
	}
	// Very short generic body with no requirements section is a ghost-job hint.
	if len(strings.Fields(text)) < 40 && !strings.Contains(lower, "requirement") {
		add(Caution, "thin-generic-posting")
	}
	return res
}

func worse(a, b Level) bool {
	rank := map[Level]int{High: 0, Caution: 1, Suspicious: 2}
	return rank[a] > rank[b]
}

type rule struct {
	signal string
	re     *regexp.Regexp
}

var suspiciousRules = []rule{
	{signal: "wire-or-crypto-pay", re: regexp.MustCompile(`(?i)(?:wire transfer|send bitcoin|send (?:usdt|crypto)|western union)`)},
	{signal: "whatsapp-apply", re: regexp.MustCompile(`(?i)apply (?:via|on|through) whatsapp`)},
	{signal: "ssn-upfront", re: regexp.MustCompile(`(?i)(?:ssn|social security number).{0,40}(?:to (?:apply|begin|start)|before interview)`)},
	{signal: "guaranteed-job", re: regexp.MustCompile(`(?i)guaranteed (?:job|offer|employment)`)},
}

var cautionRules = []rule{
	{signal: "ignore-previous-instructions", re: regexp.MustCompile(`(?i)ignore (?:all )?(?:previous|prior|above) instructions`)},
	{signal: "evergreen-or-repost", re: regexp.MustCompile(`(?i)(?:evergreen requisition|this is a standing pipeline|always accepting applications for this role)`)},
	{signal: "no-company-specificity", re: regexp.MustCompile(`(?i)competitive salary.{0,40}fast[- ]paced environment`)},
}
