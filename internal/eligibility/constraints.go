package eligibility

import (
	"regexp"
	"sort"
	"strings"
)

// RequirementLevel separates mandatory job constraints from non-blocking
// preferences. Eligibility only fails closed for mandatory constraints.
type RequirementLevel string

const (
	RequirementRequired  RequirementLevel = "required"
	RequirementPreferred RequirementLevel = "preferred"
)

// ConstraintKind identifies a hard condition that is independent from skill
// fit. These values are stable JSON contract values.
type ConstraintKind string

const (
	ConstraintSecurityClearance ConstraintKind = "security_clearance"
	ConstraintCitizenship       ConstraintKind = "citizenship"
	ConstraintWorkAuthorization ConstraintKind = "work_authorization"
	ConstraintSponsorship       ConstraintKind = "sponsorship"
	ConstraintTravel            ConstraintKind = "travel"
)

// ConstraintState reports whether the candidate profile proves a detected
// condition. Unverified required constraints fail closed.
type ConstraintState string

const (
	ConstraintSatisfied   ConstraintState = "satisfied"
	ConstraintUnsatisfied ConstraintState = "unsatisfied"
	ConstraintUnverified  ConstraintState = "unverified"
)

// Constraint is deterministic evidence for one posting condition.
type Constraint struct {
	Kind              ConstraintKind   `json:"kind"`
	Level             RequirementLevel `json:"level"`
	Value             string           `json:"value"`
	State             ConstraintState  `json:"state"`
	Evidence          string           `json:"evidence"`
	CandidateEvidence string           `json:"candidate_evidence,omitempty"`
}

type requirementSection int

const (
	sectionUnknown requirementSection = iota
	sectionRequired
	sectionPreferred
)

var clearancePatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{name: "TS/SCI", pattern: regexp.MustCompile(`(?i)\b(?:active\s+)?(?:ts\s*/?\s*sci|top secret\s*/?\s*sci)\b(?:\s+(?:security\s+)?clearance)?`)},
	{name: "Top Secret", pattern: regexp.MustCompile(`(?i)\b(?:active\s+)?top secret\b(?:\s+(?:security\s+)?clearance)?`)},
	{name: "Secret", pattern: regexp.MustCompile(`(?i)\b(?:active\s+)?secret\b(?:\s+(?:security\s+)?clearance)`)},
	{name: "Public Trust", pattern: regexp.MustCompile(`(?i)\bpublic trust\b(?:\s+(?:clearance|eligibility))?`)},
}

var citizenshipPattern = regexp.MustCompile(`(?i)\b(?:must (?:be|hold)|requires?|required|only)\b[^.\n]{0,35}(?:u\.?\s*s\.?|united states)\s+citizens?(?:hip)?\b|(?:u\.?\s*s\.?|united states)\s+citizens?(?:hip)?\b[^.\n]{0,35}\b(?:is )?(?:required|only)\b`)
var workAuthorizationPattern = regexp.MustCompile(`(?i)\b(?:must (?:be|have)|requires?|required)\b[^.\n]{0,45}\b(?:legally )?authori[sz](?:ed|ation) to work\b(?:[^.\n]{0,30}\b(?:u\.?s\.?|united states)\b)?`)
var noSponsorshipPattern = regexp.MustCompile(`(?i)\b(?:cannot|can't|do not|does not|will not|won't|unable to)\s+(?:provide|offer|support)?\s*(?:visa|employment)?\s*sponsor(?:ship)?\b|\bno\s+(?:visa|employment)?\s*sponsor(?:ship)?\b|\bmust not require\b[^.\n]{0,30}\bsponsor(?:ship)?\b`)
var explicitPreferredPattern = regexp.MustCompile(`(?i)\b(preferred|ideally|bonus|nice to have|a plus)\b`)
var explicitRequiredPattern = regexp.MustCompile(`(?i)\b(required|requires?|must|active|minimum|need to|needs to|shall)\b`)

func evaluateCandidateHardConstraints(text string, candidate Candidate, result *Result) {
	section := sectionUnknown
	for _, rawLine := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimLeft(rawLine, "*-• \t"))
		if line == "" {
			continue
		}
		if next, ok := requirementSectionHeader(line); ok {
			section = next
			continue
		}
		level := levelForLine(line, section)
		evaluateClearanceLine(line, level, candidate, result)
		evaluateCitizenshipLine(line, level, candidate, result)
		evaluateWorkAuthorizationLine(line, level, candidate, result)
		evaluateSponsorshipLine(line, level, candidate, result)
	}
}

func requirementSectionHeader(line string) (requirementSection, bool) {
	header := strings.ToLower(strings.TrimSpace(strings.Trim(line, "# :")))
	switch header {
	case "required", "requirement", "requirements", "minimum qualification", "minimum qualifications",
		"basic qualification", "basic qualifications", "must-have", "must-haves",
		"what you bring", "what you have", "what you need", "what you'll bring", "what you'll have",
		"what we bring", "what we have", "what we need", "what we're looking for", "what we are looking for":
		return sectionRequired, true
	case "bonus", "bonus point", "bonus points", "preferred", "preferred qualification", "preferred qualifications",
		"nice-to-have", "nice-to-haves", "nice to have", "nice to haves", "plus", "pluses", "extra credit":
		return sectionPreferred, true
	case "about", "responsibility", "responsibilities", "what you'll do", "what you will do", "the role",
		"benefit", "benefits", "perk", "perks", "compensation", "location", "location and travel",
		"who we are", "our team", "our stack", "our mission", "overview", "description":
		return sectionUnknown, true
	default:
		return sectionUnknown, false
	}
}

func levelForLine(line string, section requirementSection) RequirementLevel {
	if explicitPreferredPattern.MatchString(line) {
		return RequirementPreferred
	}
	if explicitRequiredPattern.MatchString(line) {
		return RequirementRequired
	}
	if section == sectionRequired {
		return RequirementRequired
	}
	if section == sectionPreferred {
		return RequirementPreferred
	}
	return RequirementPreferred
}

func evaluateClearanceLine(line string, level RequirementLevel, candidate Candidate, result *Result) {
	for _, item := range clearancePatterns {
		if !item.pattern.MatchString(line) {
			continue
		}
		state, evidence := clearanceState(item.name, candidate.ActiveSecurityClearances)
		constraint := Constraint{
			Kind: ConstraintSecurityClearance, Level: level, Value: item.name,
			State: state, Evidence: compactEvidence(line), CandidateEvidence: evidence,
		}
		if constraintExists(result.Constraints, constraint) {
			return
		}
		result.Constraints = append(result.Constraints, constraint)
		if level == RequirementRequired && state != ConstraintSatisfied {
			result.HardConstraintsVerified = false
			result.add(Ineligible, "security_clearance_unverified", string(ConstraintSecurityClearance), "posting requires an active "+item.name+" clearance; the candidate profile does not prove it")
		}
		return
	}
}

func evaluateCitizenshipLine(line string, level RequirementLevel, candidate Candidate, result *Result) {
	if !citizenshipPattern.MatchString(line) {
		return
	}
	state, candidateEvidence := listEvidence(candidate.Citizenships, "United States")
	constraint := Constraint{
		Kind: ConstraintCitizenship, Level: level, Value: "United States",
		State: state, Evidence: compactEvidence(line), CandidateEvidence: candidateEvidence,
	}
	if constraintExists(result.Constraints, constraint) {
		return
	}
	result.Constraints = append(result.Constraints, constraint)
	if level == RequirementRequired && state != ConstraintSatisfied {
		result.HardConstraintsVerified = false
		result.add(Ineligible, "citizenship_unverified", string(ConstraintCitizenship), "posting requires United States citizenship; the candidate profile does not prove it")
	}
}

func evaluateWorkAuthorizationLine(line string, level RequirementLevel, candidate Candidate, result *Result) {
	if !workAuthorizationPattern.MatchString(line) {
		return
	}
	state, candidateEvidence := listEvidence(candidate.WorkAuthorizations, "United States")
	constraint := Constraint{
		Kind: ConstraintWorkAuthorization, Level: level, Value: "United States",
		State: state, Evidence: compactEvidence(line), CandidateEvidence: candidateEvidence,
	}
	if constraintExists(result.Constraints, constraint) {
		return
	}
	result.Constraints = append(result.Constraints, constraint)
	if level == RequirementRequired && state != ConstraintSatisfied {
		result.HardConstraintsVerified = false
		result.add(Ineligible, "work_authorization_unverified", string(ConstraintWorkAuthorization), "posting requires United States work authorization; the candidate profile does not prove it")
	}
}

func evaluateSponsorshipLine(line string, level RequirementLevel, candidate Candidate, result *Result) {
	if !noSponsorshipPattern.MatchString(line) {
		return
	}
	state := ConstraintUnverified
	candidateEvidence := ""
	if candidate.RequiresSponsorship != nil {
		if *candidate.RequiresSponsorship {
			state = ConstraintUnsatisfied
			candidateEvidence = "candidate profile records that sponsorship is required"
		} else {
			state = ConstraintSatisfied
			candidateEvidence = "candidate profile records that sponsorship is not required"
		}
	}
	constraint := Constraint{
		Kind: ConstraintSponsorship, Level: level, Value: "no sponsorship",
		State: state, Evidence: compactEvidence(line), CandidateEvidence: candidateEvidence,
	}
	if constraintExists(result.Constraints, constraint) {
		return
	}
	result.Constraints = append(result.Constraints, constraint)
	if level == RequirementRequired && state != ConstraintSatisfied {
		result.HardConstraintsVerified = false
		result.add(Ineligible, "sponsorship_unverified", string(ConstraintSponsorship), "posting does not provide sponsorship; the candidate profile does not prove that sponsorship is unnecessary")
	}
}

func clearanceState(required string, active []string) (ConstraintState, string) {
	requiredRank := clearanceRank(required)
	for _, value := range active {
		if clearanceRank(value) >= requiredRank && requiredRank > 0 {
			return ConstraintSatisfied, "candidate profile records active " + strings.TrimSpace(value)
		}
	}
	if len(active) > 0 {
		return ConstraintUnsatisfied, "candidate profile records active " + strings.Join(active, ", ")
	}
	return ConstraintUnverified, ""
}

func clearanceRank(value string) int {
	normalized := strings.ToLower(strings.NewReplacer("/", " ", "-", " ", "_", " ").Replace(value))
	switch {
	case strings.Contains(normalized, "sci"):
		return 4
	case strings.Contains(normalized, "top secret") || normalized == "ts" || strings.Contains(normalized, " ts ") || strings.HasPrefix(normalized, "ts "):
		return 3
	case strings.Contains(normalized, "secret"):
		return 2
	case strings.Contains(normalized, "public trust"):
		return 1
	default:
		return 0
	}
}

func listEvidence(values []string, required string) (ConstraintState, string) {
	for _, value := range values {
		if equivalentCountry(value, required) || containsToken(value, required) || containsToken(required, value) {
			return ConstraintSatisfied, "candidate profile records " + strings.TrimSpace(value)
		}
	}
	if len(values) > 0 {
		return ConstraintUnsatisfied, "candidate profile records " + strings.Join(values, ", ")
	}
	return ConstraintUnverified, ""
}

func constraintExists(values []Constraint, candidate Constraint) bool {
	for _, value := range values {
		if value.Kind == candidate.Kind && value.Level == candidate.Level && value.Value == candidate.Value {
			return true
		}
	}
	return false
}

func sortConstraints(values []Constraint) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].Level != values[j].Level {
			return values[i].Level < values[j].Level
		}
		return values[i].Value < values[j].Value
	})
}

func equivalentCountry(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(value)
		value = strings.NewReplacer(".", "", " ", "", "-", "").Replace(value)
		switch value {
		case "us", "usa", "unitedstates", "unitedstatesofamerica":
			return "unitedstates"
		default:
			return value
		}
	}
	return normalize(left) == normalize(right)
}

func compactEvidence(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const maximum = 240
	if len(value) <= maximum {
		return value
	}
	return strings.TrimSpace(value[:maximum-3]) + "..."
}
