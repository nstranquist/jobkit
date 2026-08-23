package inbox

import (
	"fmt"
	"testing"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/jobsearch"
)

func TestBuildSlateEnforcesMixAndEmployerCap(t *testing.T) {
	var items []*Item
	add := func(company, title, family string, score int) {
		id := fmt.Sprintf("%s-%d", company, len(items))
		items = append(items, &Item{
			ID: id, Status: "new", MatchScore: float64(score),
			Job: Job{Company: company, Title: title, Opportunity: jobsearch.Opportunity{Score: score},
				Eligibility: &eligibility.Result{Status: eligibility.Eligible, RoleFamily: family, HardConstraintsVerified: true}},
		})
	}
	for i := 0; i < 7; i++ {
		add(fmt.Sprintf("Platform%d", i/2), "Senior Platform Engineer", "software-engineering", 90-i)
	}
	for i := 0; i < 4; i++ {
		add(fmt.Sprintf("Product%d", i), "Senior Full Stack Product Engineer", "software-engineering", 80-i)
	}
	add("FieldCo", "Forward Deployed Engineer", "technical-adoption", 75)
	add("StretchCo", "Staff Software Engineer", "software-engineering", 70)

	slate := BuildSlate(items, DefaultSlatePolicy())
	if len(slate.Selections) != 10 {
		t.Fatalf("selections = %d, want 10: %#v", len(slate.Selections), slate.Warnings)
	}
	for lane, want := range map[string]int{LanePlatform: 5, LaneFullstack: 3, LaneAdoption: 1, LaneStretch: 1} {
		if slate.Filled[lane] != want {
			t.Errorf("lane %s = %d, want %d", lane, slate.Filled[lane], want)
		}
	}
	companyCounts := map[string]int{}
	for _, selected := range slate.Selections {
		companyCounts[selected.Company]++
		if companyCounts[selected.Company] > 2 {
			t.Fatalf("company cap exceeded for %s", selected.Company)
		}
	}
}

func TestBuildSlateExcludesUnassessedAndIneligible(t *testing.T) {
	ineligible := &eligibility.Result{Status: eligibility.Ineligible, RoleFamily: "software-engineering"}
	items := []*Item{
		{ID: "unassessed", Status: "new", Job: Job{Title: "Platform Engineer"}},
		{ID: "ineligible", Status: "new", Job: Job{Title: "Platform Engineer", Eligibility: ineligible}},
	}
	slate := BuildSlate(items, SlatePolicy{Platform: 1, EmployerCap: 2})
	if len(slate.Selections) != 0 || slate.Skipped["unassessed"] != 1 || slate.Skipped["ineligible"] != 1 {
		t.Fatalf("slate = %#v", slate)
	}
}

func TestBuildSlateRequiresVerifiedEligibilityByDefault(t *testing.T) {
	items := []*Item{
		{ID: "review", Status: "new", Job: Job{Title: "Platform Engineer", Eligibility: &eligibility.Result{Status: eligibility.Review, RoleFamily: "software-engineering", HardConstraintsVerified: true}}},
		{ID: "old", Status: "new", Job: Job{Title: "Platform Engineer", Eligibility: &eligibility.Result{Status: eligibility.Eligible, RoleFamily: "software-engineering"}}},
	}
	slate := BuildSlate(items, SlatePolicy{Platform: 1, EmployerCap: 2})
	if len(slate.Selections) != 0 || slate.Skipped["review_required"] != 1 || slate.Skipped["hard_constraints_unverified"] != 1 {
		t.Fatalf("slate = %#v", slate)
	}

	policy := SlatePolicy{Platform: 1, EmployerCap: 2, AllowReview: true}
	slate = BuildSlate(items[:1], policy)
	if len(slate.Selections) != 1 || slate.Selections[0].ID != "review" {
		t.Fatalf("review opt-in slate = %#v", slate)
	}
}

func TestLaneForKeepsTechnicalAdoptionArchitectInAdoptionLane(t *testing.T) {
	item := &Item{Job: Job{
		Title: "Solutions Architect",
		Eligibility: &eligibility.Result{
			Status: eligibility.Eligible, RoleFamily: "technical-adoption", HardConstraintsVerified: true,
		},
	}}
	if lane := LaneFor(item); lane != LaneAdoption {
		t.Fatalf("lane = %q, want %q", lane, LaneAdoption)
	}
}
