package jd

import (
	"regexp"
	"strings"
)

// InstructionFlags returns imperative / injection phrases found in posting
// text. The phrases are data to record, never instructions to execute.
func InstructionFlags(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, rule := range instructionRules {
		if loc := rule.re.FindStringIndex(text); loc != nil {
			if seen[rule.label] {
				continue
			}
			seen[rule.label] = true
			out = append(out, rule.label)
		}
	}
	return out
}

type instructionRule struct {
	label string
	re    *regexp.Regexp
}

var instructionRules = []instructionRule{
	{label: "ignore-previous-instructions", re: regexp.MustCompile(`(?i)ignore (?:all )?(?:previous|prior|above) instructions`)},
	{label: "reveal-system-prompt", re: regexp.MustCompile(`(?i)(?:reveal|show|print|dump) (?:your )?(?:system )?prompt`)},
	{label: "fake-system-block", re: regexp.MustCompile(`(?i)(?:^|\n)\s*(?:system:|<system>|\[system\])`)},
	{label: "as-the-ai-reviewer", re: regexp.MustCompile(`(?i)as the (?:ai|assistant|model|llm) (?:reviewing|evaluating|reading) this`)},
	{label: "disobey-prior-rules", re: regexp.MustCompile(`(?i)(?:disobey|do not (?:follow|obey)|override) (?:your )?(?:prior |previous )?rules`)},
}
