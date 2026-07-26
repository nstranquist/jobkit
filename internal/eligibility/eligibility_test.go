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

func TestRoleFamilyClassification(t *testing.T) {
	tests := map[string]string{
		"Senior Product Engineer":             "software-engineering",
		"Machine Learning Engineer, Platform": "software-engineering",
		"AI Deployment Engineer, Startups":    "technical-adoption",
		"Technical Account Manager":           "technical-adoption",
		"Technical Program Manager, Platform": "management",
	}
	for title, want := range tests {
		result := Evaluate(Posting{Title: title, Location: "Remote - US", Remote: true}, testPolicy())
		if result.RoleFamily != want {
			t.Errorf("%q family = %q, want %q", title, result.RoleFamily, want)
		}
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
