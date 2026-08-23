package inbox

import (
	"testing"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/jobsearch"
)

func TestBuildSummaryIsBoundedAndDescriptionFree(t *testing.T) {
	verified := &eligibility.Result{
		Status: eligibility.Eligible, RoleFamily: "software-engineering", HardConstraintsVerified: true,
	}
	items := []*Item{
		{ID: "ready-high", Status: "new", NextAction: "apply-plan", MatchScore: 80,
			Job: Job{Title: "Platform Engineer", Company: "Acme", Description: "large private body", Eligibility: verified, Opportunity: jobsearch.Opportunity{Score: 90}}},
		{ID: "ready-low", Status: "shortlisted", NextAction: "review-gaps", MatchScore: 70,
			Job: Job{Title: "Backend Engineer", Company: "Beta", JDText: "large private body", Eligibility: verified, Opportunity: jobsearch.Opportunity{Score: 70}}},
		{ID: "old-receipt", Status: "new", Job: Job{Title: "Cloud Engineer", Eligibility: &eligibility.Result{Status: eligibility.Eligible}}},
		{ID: "review", Status: "new", Job: Job{Title: "Staff Engineer", Eligibility: &eligibility.Result{Status: eligibility.Review}}},
		{ID: "closed", Status: "new", Job: Job{Title: "SRE", Liveness: "closed", Eligibility: verified}},
		{ID: "unassessed", Status: "new", Job: Job{Title: "Product Engineer"}},
	}
	summary := BuildSummary(items, 1)
	if summary.Total != 6 || summary.Active != 6 || summary.ReadyTotal != 2 || summary.Returned != 1 || !summary.Truncated {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.HardConstraintsPending != 1 || summary.Unassessed != 1 || summary.Closed != 1 {
		t.Fatalf("summary counts = %#v", summary)
	}
	if len(summary.Ready) != 1 || summary.Ready[0].ID != "ready-high" {
		t.Fatalf("ready = %#v", summary.Ready)
	}
}

func TestBuildSummaryUsesDefaultLimit(t *testing.T) {
	verified := &eligibility.Result{Status: eligibility.Eligible, RoleFamily: "software-engineering", HardConstraintsVerified: true}
	items := make([]*Item, DefaultSummaryLimit+3)
	for i := range items {
		items[i] = &Item{ID: string(rune('a' + i)), Status: "new", Job: Job{Title: "Platform Engineer", Eligibility: verified}}
	}
	summary := BuildSummary(items, 0)
	if summary.Returned != DefaultSummaryLimit || !summary.Truncated {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestBuildSummaryKeepsEmptyReadyListStable(t *testing.T) {
	summary := BuildSummary([]*Item{{ID: "unassessed", Status: "new", Job: Job{Title: "Engineer"}}}, 5)
	if summary.Ready == nil || len(summary.Ready) != 0 {
		t.Fatalf("ready = %#v, want a non-nil empty list", summary.Ready)
	}
}
