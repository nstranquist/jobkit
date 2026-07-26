// Package eligibility evaluates hard job constraints independently from skill
// fit and opportunity ranking. It is deterministic and intentionally
// conservative: uncertain constraints become review, not silent rejection.
package eligibility

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nstranquist/jobkit/internal/privatefs"
	"gopkg.in/yaml.v3"
)

type Status string

const (
	Eligible   Status = "eligible"
	Review     Status = "review"
	Ineligible Status = "ineligible"
	Unassessed Status = "unassessed"
)

type Config struct {
	SchemaVersion int          `yaml:"schema_version" json:"schema_version"`
	Candidate     Candidate    `yaml:"candidate" json:"candidate"`
	Policy        PolicyConfig `yaml:"policy" json:"policy"`
}

type Candidate struct {
	HomeLocations    []string `yaml:"home_locations" json:"home_locations"`
	AllowedCountries []string `yaml:"allowed_countries" json:"allowed_countries"`
	Languages        []string `yaml:"languages" json:"languages"`
	YearsExperience  int      `yaml:"years_experience" json:"years_experience"`
	RelocationOpen   bool     `yaml:"relocation_open" json:"relocation_open"`
}

type PolicyConfig struct {
	AllowedRoleFamilies []string `yaml:"allowed_role_families" json:"allowed_role_families"`
	MaxTravelPercent    *int     `yaml:"max_travel_percent,omitempty" json:"max_travel_percent,omitempty"`
	YearsStretch        int      `yaml:"years_stretch" json:"years_stretch"`
	AllowManagement     bool     `yaml:"allow_management" json:"allow_management"`
	AllowSales          bool     `yaml:"allow_sales" json:"allow_sales"`
}

type Policy struct {
	Candidate           Candidate `json:"-"`
	AllowedRoleFamilies []string  `json:"allowed_role_families"`
	MaxTravelPercent    int       `json:"max_travel_percent"`
	YearsStretch        int       `json:"years_stretch"`
	AllowManagement     bool      `json:"allow_management"`
	AllowSales          bool      `json:"allow_sales"`
}

type Posting struct {
	Title       string `json:"title"`
	Location    string `json:"location,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
	Description string `json:"description,omitempty"`
}

type Reason struct {
	Code     string `json:"code"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type Result struct {
	Status        Status   `json:"status"`
	RoleFamily    string   `json:"role_family"`
	WorkMode      string   `json:"work_mode"`
	RequiredYears int      `json:"required_years,omitempty"`
	TravelPercent int      `json:"travel_percent,omitempty"`
	Reasons       []Reason `json:"reasons,omitempty"`
	Override      string   `json:"override,omitempty"`
}

func intPtr(value int) *int { return &value }

func Template(homeLocations []string, years int, relocationOpen bool) Config {
	return Config{
		SchemaVersion: 1,
		Candidate: Candidate{
			HomeLocations: normalizeList(homeLocations), AllowedCountries: []string{"United States", "USA", "US"},
			Languages: []string{"English"}, YearsExperience: years, RelocationOpen: relocationOpen,
		},
		Policy: PolicyConfig{
			AllowedRoleFamilies: []string{"software-engineering", "technical-adoption"},
			MaxTravelPercent:    intPtr(25), YearsStretch: 2,
		},
	}
}

func (c Config) EffectivePolicy() Policy {
	maxTravel := 25
	if c.Policy.MaxTravelPercent != nil {
		maxTravel = *c.Policy.MaxTravelPercent
	}
	policy := Policy{
		Candidate: c.Candidate, AllowedRoleFamilies: append([]string(nil), c.Policy.AllowedRoleFamilies...),
		MaxTravelPercent: maxTravel, YearsStretch: c.Policy.YearsStretch,
		AllowManagement: c.Policy.AllowManagement, AllowSales: c.Policy.AllowSales,
	}
	if policy.YearsStretch < 0 {
		policy.YearsStretch = 0
	}
	if len(policy.AllowedRoleFamilies) == 0 {
		policy.AllowedRoleFamilies = []string{"software-engineering"}
	}
	return policy
}

func Load(path string) (Config, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	return config, nil
}

func Save(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return privatefs.WriteFile(path, payload)
}

func (c Config) Validate() error {
	if c.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if c.Candidate.YearsExperience < 0 {
		return fmt.Errorf("candidate.years_experience must not be negative")
	}
	if c.Policy.MaxTravelPercent != nil && (*c.Policy.MaxTravelPercent < 0 || *c.Policy.MaxTravelPercent > 100) {
		return fmt.Errorf("policy.max_travel_percent must be between 0 and 100")
	}
	allowed := map[string]bool{"software-engineering": true, "technical-adoption": true, "management": true, "sales": true}
	for _, family := range c.Policy.AllowedRoleFamilies {
		if !allowed[family] {
			return fmt.Errorf("unknown allowed role family %q", family)
		}
	}
	return nil
}

func Evaluate(posting Posting, policy Policy) Result {
	result := Result{Status: Eligible, RoleFamily: classifyRole(posting.Title), WorkMode: classifyWorkMode(posting)}
	allowedFamilies := set(policy.AllowedRoleFamilies)
	if len(allowedFamilies) == 0 {
		allowedFamilies["software-engineering"] = true
	}
	if result.RoleFamily == "management" && !policy.AllowManagement {
		result.add(Ineligible, "management_disallowed", "management_sales", "people-management role is outside the configured search")
	} else if result.RoleFamily == "sales" && !policy.AllowSales {
		result.add(Ineligible, "sales_disallowed", "management_sales", "sales-led role is outside the configured search")
	} else if !allowedFamilies[result.RoleFamily] {
		if result.RoleFamily == "unknown" {
			result.add(Review, "role_family_unknown", "role_family", "role family is ambiguous and needs human review")
		} else {
			result.add(Ineligible, "role_family_disallowed", "role_family", fmt.Sprintf("role family %s is outside the configured search", result.RoleFamily))
		}
	}

	requiredLanguages := requiredLanguages(posting.Description)
	knownLanguages := setFold(policy.Candidate.Languages)
	for _, language := range requiredLanguages {
		if !knownLanguages[strings.ToLower(language)] {
			result.add(Ineligible, "language_required", "language", fmt.Sprintf("posting requires %s, which is not in the candidate language profile", language))
		}
	}

	result.RequiredYears = requiredYears(posting.Description)
	if result.RequiredYears > 0 && policy.Candidate.YearsExperience > 0 && result.RequiredYears > policy.Candidate.YearsExperience {
		delta := result.RequiredYears - policy.Candidate.YearsExperience
		if delta <= policy.YearsStretch {
			result.add(Review, "years_stretch", "years", fmt.Sprintf("requires %d years; candidate profile records %d", result.RequiredYears, policy.Candidate.YearsExperience))
		} else {
			result.add(Ineligible, "years_exceeds_stretch", "years", fmt.Sprintf("requires %d years; configured stretch ceiling is %d", result.RequiredYears, policy.Candidate.YearsExperience+policy.YearsStretch))
		}
	}

	result.TravelPercent = requiredTravel(posting.Description)
	if result.TravelPercent > policy.MaxTravelPercent {
		severity := Review
		if result.TravelPercent > policy.MaxTravelPercent+25 {
			severity = Ineligible
		}
		result.add(severity, "travel_exceeds_preference", "travel", fmt.Sprintf("requires %d%% travel; configured maximum is %d%%", result.TravelPercent, policy.MaxTravelPercent))
	}

	evaluateGeography(posting, policy.Candidate, &result)
	sort.SliceStable(result.Reasons, func(i, j int) bool {
		if result.Reasons[i].Category != result.Reasons[j].Category {
			return result.Reasons[i].Category < result.Reasons[j].Category
		}
		return result.Reasons[i].Code < result.Reasons[j].Code
	})
	return result
}

func Allows(filter string, status Status) bool {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", "actionable":
		return status == Eligible || status == Review
	case "all":
		return true
	case string(Eligible):
		return status == Eligible
	case string(Review):
		return status == Review
	case string(Ineligible):
		return status == Ineligible
	default:
		return false
	}
}

func ValidFilter(filter string) bool {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", "actionable", "all", string(Eligible), string(Review), string(Ineligible):
		return true
	default:
		return false
	}
}

func Rank(status Status) int {
	switch status {
	case Eligible:
		return 0
	case Review:
		return 1
	case Ineligible:
		return 2
	default:
		return 3
	}
}

func (r *Result) add(status Status, code, category, summary string) {
	if Rank(status) > Rank(r.Status) {
		r.Status = status
	}
	r.Reasons = append(r.Reasons, Reason{Code: code, Category: category, Summary: summary})
}

func classifyRole(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	switch {
	case matchesAny(title, `\b(account executive|sales manager|sales representative|business development)\b`, `\bcustomer success manager\b`, `\bsales engineer\b`):
		return "sales"
	case matchesAny(title, `\bforward deployed\b`, `\bsolutions? (engineer|architect)\b`, `\bdeveloper advocate\b`, `\bdeveloper relations\b`, `\btechnical account manager\b`, `\bcustomer engineer\b`, `\b(ai|technical) adoption\b`, `\b(ai )?deployment engineer\b`, `\bfield engineer\b`):
		return "technical-adoption"
	case matchesAny(title, `\bmanager\b`, `\bdirector\b`, `\b(head|vp|vice president) of\b`):
		return "management"
	case matchesAny(title,
		`\b(software|platform|backend|front[- ]?end|full[- ]?stack|devops|infrastructure|cloud|site reliability|systems?|ai|ml) engineer\b`,
		`\b(developer experience|developer productivity|developer tools?|devex) engineer\b`,
		`\b(machine learning|product|security|reliability) engineer\b`,
		`\bmember of technical staff\b`, `\bsoftware developer\b`, `\bsre\b`, `\bstaff engineer\b`, `\bprincipal engineer\b`):
		return "software-engineering"
	default:
		return "unknown"
	}
}

func classifyWorkMode(posting Posting) string {
	text := strings.ToLower(posting.Location + " " + posting.Description)
	switch {
	case posting.Remote || strings.Contains(strings.ToLower(posting.Location), "remote"):
		return "remote"
	case strings.Contains(text, "hybrid"):
		return "hybrid"
	case strings.Contains(text, "on-site") || strings.Contains(text, "onsite") || strings.Contains(text, "in office"):
		return "onsite"
	default:
		return "unspecified"
	}
}

func evaluateGeography(posting Posting, candidate Candidate, result *Result) {
	location := strings.TrimSpace(posting.Location)
	if location == "" {
		result.add(Review, "geography_unknown", "geography", "posting location is missing")
		return
	}
	lower := strings.ToLower(location)
	allowedCountry := containsAnyToken(lower, candidate.AllowedCountries)
	foreign := firstForeignRegion(lower)
	if foreign != "" && !allowedCountry {
		result.add(Ineligible, "geography_country", "geography", fmt.Sprintf("posting is scoped to %s outside the configured countries", foreign))
		return
	}
	if result.WorkMode == "remote" {
		if !allowedCountry {
			result.add(Review, "geography_scope_unknown", "geography", "remote posting does not identify an allowed country scope")
		}
		return
	}
	if containsAnyToken(lower, candidate.HomeLocations) {
		return
	}
	if candidate.RelocationOpen {
		result.add(Review, "geography_relocation", "geography", fmt.Sprintf("non-remote role in %s requires relocation review", location))
		return
	}
	result.add(Ineligible, "work_mode_location", "work_mode", fmt.Sprintf("non-remote role in %s is outside configured home locations", location))
}

var yearPattern = regexp.MustCompile(`(?i)(?:minimum(?: of)?\s+|at least\s+)?([0-9]{1,2})\+?\s*(?:years?|yrs?)(?:\s+of)?(?:\s+(?:relevant|professional|industry|software))?\s+experience`)
var travelPattern = regexp.MustCompile(`(?i)([0-9]{1,3})\s*%[^.\n]{0,30}\btravel\b|\btravel\b[^.\n]{0,30}?([0-9]{1,3})\s*%`)

func requiredYears(text string) int {
	maximum := 0
	for _, match := range yearPattern.FindAllStringSubmatch(text, -1) {
		value, _ := strconv.Atoi(match[1])
		if value > maximum && value <= 20 {
			maximum = value
		}
	}
	return maximum
}

func requiredTravel(text string) int {
	maximum := 0
	for _, match := range travelPattern.FindAllStringSubmatch(text, -1) {
		for _, raw := range match[1:] {
			value, _ := strconv.Atoi(raw)
			if value > maximum && value <= 100 {
				maximum = value
			}
		}
	}
	return maximum
}

var languageNames = []string{"English", "Korean", "Japanese", "Mandarin", "Chinese", "Spanish", "French", "German", "Portuguese", "Hindi", "Arabic"}

func requiredLanguages(text string) []string {
	var out []string
	lower := strings.ToLower(text)
	for _, language := range languageNames {
		name := strings.ToLower(language)
		patterns := []string{
			`(?i)(?:fluent|fluency|native|professional proficiency|business[- ]level|required|must (?:speak|read|write))[^.\n]{0,45}\b` + regexp.QuoteMeta(name) + `\b`,
			`(?i)\b` + regexp.QuoteMeta(name) + `\b[^.\n]{0,45}(?:required|fluency|proficiency|native|business[- ]level)`,
		}
		for _, pattern := range patterns {
			if regexp.MustCompile(pattern).MatchString(lower) {
				out = append(out, language)
				break
			}
		}
	}
	return normalizeList(out)
}

func firstForeignRegion(location string) string {
	regions := []string{"Korea", "Japan", "China", "Singapore", "India", "Australia", "Canada", "Mexico", "Brazil", "United Kingdom", "UK", "Europe", "EU", "Germany", "France", "Spain", "Portugal", "Netherlands", "Ireland"}
	for _, region := range regions {
		if containsToken(location, region) {
			return region
		}
	}
	return ""
}

func matchesAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if regexp.MustCompile(`(?i)` + pattern).MatchString(value) {
			return true
		}
	}
	return false
}

func containsAnyToken(value string, tokens []string) bool {
	for _, token := range tokens {
		if containsToken(value, token) {
			return true
		}
	}
	return false
}

func containsToken(value, token string) bool {
	value = strings.ToLower(value)
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	if len(token) <= 3 {
		return regexp.MustCompile(`(^|[^a-z])` + regexp.QuoteMeta(token) + `([^a-z]|$)`).MatchString(value)
	}
	return strings.Contains(value, token)
}

func normalizeList(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func set(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[strings.TrimSpace(value)] = true
	}
	return out
}

func setFold(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[strings.ToLower(strings.TrimSpace(value))] = true
	}
	return out
}
