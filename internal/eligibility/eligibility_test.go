package eligibility

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPolicy() Policy {
	config := Template([]string{"St. Louis, MO", "Missouri"}, 7, true)
	return config.EffectivePolicy()
}

func TestEvaluateSeparatesEligibilityFromFit(t *testing.T) {
	tests := []struct {
		name   string
		post   Posting
		status Status
		code   string
	}{
		{name: "remote software role", post: Posting{Title: "Senior Software Engineer", Location: "Remote - US", Remote: true, Description: "Requires 7+ years of experience."}, status: Eligible},
		{name: "remote scope unknown", post: Posting{Title: "Senior Software Engineer", Location: "Remote", Remote: true}, status: Review, code: "geography_scope_unknown"},
		{name: "st louis hybrid", post: Posting{Title: "Platform Engineer", Location: "St. Louis, MO (Hybrid)", Description: "3 days hybrid."}, status: Eligible},
		{name: "relocation review", post: Posting{Title: "Full Stack Engineer", Location: "New York, NY", Description: "Onsite role."}, status: Review, code: "geography_relocation"},
		{name: "years stretch", post: Posting{Title: "Senior Backend Engineer", Location: "Remote, US", Remote: true, Description: "Minimum 9+ years of professional experience."}, status: Review, code: "years_stretch"},
		{name: "years hard stop", post: Posting{Title: "Principal Engineer", Location: "Remote, US", Remote: true, Description: "At least 12 years of software experience."}, status: Ineligible, code: "years_exceeds_stretch"},
		{name: "language hard stop", post: Posting{Title: "Developer Experience Engineer", Location: "Seoul, Korea", Description: "Native Korean proficiency required. 30% travel."}, status: Ineligible, code: "language_required"},
		{name: "technical adoption included", post: Posting{Title: "Forward Deployed Engineer", Location: "Remote - US", Remote: true, Description: "Partner with customer engineering teams."}, status: Eligible},
		{name: "internship excluded", post: Posting{Title: "AI Engineer, Intern", Location: "Remote - US", Remote: true}, status: Ineligible, code: "role_family_disallowed"},
		{name: "sales excluded", post: Posting{Title: "Account Executive", Location: "Remote - US", Remote: true}, status: Ineligible, code: "sales_disallowed"},
		{name: "management excluded", post: Posting{Title: "Engineering Manager", Location: "Remote - US", Remote: true}, status: Ineligible, code: "management_disallowed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Evaluate(test.post, testPolicy())
			if result.Status != test.status {
				t.Fatalf("status=%s want=%s result=%#v", result.Status, test.status, result)
			}
			if test.code != "" && !hasReason(result, test.code) {
				t.Fatalf("missing reason %s: %#v", test.code, result)
			}
		})
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eligibility.yaml")
	want := Template([]string{"St. Louis, MO"}, 7, true)
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate.YearsExperience != 7 || !got.Candidate.RelocationOpen || got.Policy.MaxTravelPercent == nil || *got.Policy.MaxTravelPercent != 25 {
		t.Fatalf("unexpected config: %#v", got)
	}
}

func TestExplicitZeroTravelIsPreservedAndOmissionDefaults(t *testing.T) {
	zero := 0
	config := Template([]string{"St. Louis, MO"}, 7, false)
	config.Policy.MaxTravelPercent = &zero
	if got := config.EffectivePolicy().MaxTravelPercent; got != 0 {
		t.Fatalf("explicit zero became %d", got)
	}
	config.Policy.MaxTravelPercent = nil
	if got := config.EffectivePolicy().MaxTravelPercent; got != 25 {
		t.Fatalf("omitted travel became %d, want 25", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eligibility.yaml")
	payload := "schema_version: 1\ncandidate:\n  years_experience: 7\npolicy:\n  max_travel_percent: 0\n  typo_field: true\n"
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "typo_field") {
		t.Fatalf("expected strict field error, got %v", err)
	}
}

func TestEffectiveCandidateIsNotSerializedInsidePolicy(t *testing.T) {
	config := Template([]string{"St. Louis, MO"}, 7, true)
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"policy":{"candidate"`) {
		t.Fatalf("runtime-only candidate duplicated inside policy: %s", payload)
	}
}

func TestEligibilityFilters(t *testing.T) {
	if !Allows("actionable", Eligible) || !Allows("actionable", Review) || Allows("actionable", Ineligible) {
		t.Fatal("actionable filter contract changed")
	}
	if !ValidFilter("all") || ValidFilter("maybe") {
		t.Fatal("filter validation contract changed")
	}
}

func TestDeveloperExperienceIsSoftwareEngineering(t *testing.T) {
	result := Evaluate(Posting{
		Title: "Senior Developer Experience Engineer", Location: "Remote - US", Remote: true,
		Description: "Build internal developer tooling. 7 years of experience.",
	}, testPolicy())
	if result.Status != Eligible || result.RoleFamily != "software-engineering" {
		t.Fatalf("result = %#v, want eligible software-engineering", result)
	}
}

func TestScalePublicSectorClearanceFailsClosed(t *testing.T) {
	posting := Posting{
		Title:    "Senior Mission Software Engineer, Public Sector",
		Location: "St. Louis, MO",
		Description: `Required
* 5+ years of software engineering experience.
* An active TS/SCI clearance.

Bonus
* Familiarity with Kubernetes and Terraform.

Location and travel
Expect 25% travel with up to 50% travel as necessary.`,
	}
	result := Evaluate(posting, testPolicy())
	if result.Status != Ineligible || result.HardConstraintsVerified {
		t.Fatalf("result = %#v, want fail-closed ineligible", result)
	}
	if !hasReason(result, "security_clearance_unverified") || !hasReason(result, "travel_exceeds_maximum") {
		t.Fatalf("missing hard-constraint reasons: %#v", result.Reasons)
	}
	if !hasConstraint(result, ConstraintSecurityClearance, RequirementRequired, ConstraintUnverified) {
		t.Fatalf("missing typed clearance evidence: %#v", result.Constraints)
	}
}

func TestActiveClearanceSatisfiesRequiredClearance(t *testing.T) {
	policy := testPolicy()
	policy.Candidate.ActiveSecurityClearances = []string{"TS/SCI"}
	result := Evaluate(Posting{
		Title: "Forward Deployed Software Engineer", Location: "Remote - US", Remote: true,
		Description: "Required\nAn active Secret security clearance.\nExpect 25% travel.",
	}, policy)
	if result.Status != Eligible || !result.HardConstraintsVerified {
		t.Fatalf("result = %#v, want verified eligible", result)
	}
	if !hasConstraint(result, ConstraintSecurityClearance, RequirementRequired, ConstraintSatisfied) {
		t.Fatalf("missing satisfied clearance: %#v", result.Constraints)
	}
}

func TestPreferredClearanceDoesNotBlock(t *testing.T) {
	result := Evaluate(Posting{
		Title: "Platform Engineer", Location: "Remote - US", Remote: true,
		Description: "Preferred qualifications\nActive Secret security clearance is a plus.",
	}, testPolicy())
	if result.Status != Eligible || !result.HardConstraintsVerified {
		t.Fatalf("result = %#v, want eligible", result)
	}
	if !hasConstraint(result, ConstraintSecurityClearance, RequirementPreferred, ConstraintUnverified) {
		t.Fatalf("missing preferred clearance: %#v", result.Constraints)
	}
}

func TestCitizenshipAndAuthorizationRequireCandidateEvidence(t *testing.T) {
	posting := Posting{
		Title: "Software Engineer", Location: "Remote - US", Remote: true,
		Description: "Requirements\nCandidates must be U.S. citizens.\nCandidates must be legally authorized to work in the United States.",
	}
	result := Evaluate(posting, testPolicy())
	if result.Status != Ineligible || !hasReason(result, "citizenship_unverified") || !hasReason(result, "work_authorization_unverified") {
		t.Fatalf("result = %#v, want both unverified constraints", result)
	}
	policy := testPolicy()
	policy.Candidate.Citizenships = []string{"United States"}
	policy.Candidate.WorkAuthorizations = []string{"United States"}
	result = Evaluate(posting, policy)
	if result.Status != Eligible || !result.HardConstraintsVerified {
		t.Fatalf("result = %#v, want verified eligible", result)
	}
}

func TestNoSponsorshipRequirementFailsClosedWhenUnknown(t *testing.T) {
	posting := Posting{
		Title: "Backend Engineer", Location: "Remote - US", Remote: true,
		Description: "Requirements\nWe are unable to provide visa sponsorship for this role.",
	}
	result := Evaluate(posting, testPolicy())
	if result.Status != Ineligible || !hasReason(result, "sponsorship_unverified") {
		t.Fatalf("result = %#v, want sponsorship gate", result)
	}
	no := false
	policy := testPolicy()
	policy.Candidate.RequiresSponsorship = &no
	result = Evaluate(posting, policy)
	if result.Status != Eligible || !result.HardConstraintsVerified {
		t.Fatalf("result = %#v, want verified eligible", result)
	}
}

func TestRequirementSectionHeaders(t *testing.T) {
	tests := map[string]requirementSection{
		"Required":                 sectionRequired,
		"Requirements":             sectionRequired,
		"What we're looking for":   sectionRequired,
		"Preferred qualifications": sectionPreferred,
		"Bonus":                    sectionPreferred,
		"Location and travel":      sectionUnknown,
	}
	for input, want := range tests {
		got, ok := requirementSectionHeader(input)
		if !ok || got != want {
			t.Errorf("header %q = (%v, %v), want (%v, true)", input, got, ok, want)
		}
	}
}

func TestRequiredSectionMakesSponsorshipMandatory(t *testing.T) {
	line := "We are unable to provide visa sponsorship for this role."
	if got := levelForLine(line, sectionRequired); got != RequirementRequired {
		t.Fatalf("level = %q, preferred match = %q, required match = %q", got, explicitPreferredPattern.FindString(line), explicitRequiredPattern.FindString(line))
	}
}

func TestApplicationFormQuestionsAreNotRequirements(t *testing.T) {
	result := Evaluate(Posting{
		Title: "Software Engineer", Location: "Remote - US", Remote: true,
		Description: "Are you legally authorized to work in the country where the job is located?\nWill you now or in the future require company sponsorship?",
	}, testPolicy())
	if result.Status != Eligible || len(result.Constraints) != 0 {
		t.Fatalf("result = %#v, form questions must not become requirements", result)
	}
}

func TestExplicitOnsiteRequirementOverridesRemoteHint(t *testing.T) {
	result := Evaluate(Posting{
		Title: "Platform Engineer", Location: "Remote - US", Remote: true,
		Description: "This role requires on-site work with customers.",
	}, testPolicy())
	if result.WorkMode != "onsite" {
		t.Fatalf("work mode = %q, want onsite", result.WorkMode)
	}
}

func TestRoleFamilyClassification(t *testing.T) {
	tests := map[string]string{
		"Senior Product Engineer":             "software-engineering",
		"Machine Learning Engineer, Platform": "software-engineering",
		"AI Deployment Engineer, Startups":    "technical-adoption",
		"Technical Account Manager":           "technical-adoption",
		"Technical Program Manager, Platform": "management",
		"Software Engineer Co-op":             "internship",
	}
	for title, want := range tests {
		result := Evaluate(Posting{Title: title, Location: "Remote - US", Remote: true}, testPolicy())
		if result.RoleFamily != want {
			t.Errorf("%q family = %q, want %q", title, result.RoleFamily, want)
		}
	}
}

func TestInternshipsRequireExplicitRoleFamilyOptIn(t *testing.T) {
	config := Template([]string{"St. Louis, MO"}, 1, true)
	config.Policy.AllowedRoleFamilies = append(config.Policy.AllowedRoleFamilies, "internship")
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	result := Evaluate(Posting{Title: "Software Engineer Intern", Location: "Remote - US", Remote: true}, config.EffectivePolicy())
	if result.Status != Eligible || result.RoleFamily != "internship" {
		t.Fatalf("result = %#v, want explicitly enabled internship", result)
	}
}

func hasReason(result Result, code string) bool {
	for _, reason := range result.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func hasConstraint(result Result, kind ConstraintKind, level RequirementLevel, state ConstraintState) bool {
	for _, constraint := range result.Constraints {
		if constraint.Kind == kind && constraint.Level == level && constraint.State == state {
			return true
		}
	}
	return false
}
