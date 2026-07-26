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
				Eligibility: &eligibility.Result{Status: eligibility.Eligible, RoleFamily: family}},
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
