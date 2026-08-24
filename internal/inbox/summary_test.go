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

func TestBuildSummaryReportsActiveStableIdentityDuplicates(t *testing.T) {
	job := Job{Provider: "greenhouse", Board: "acme", ExternalID: "123", Company: "Acme", Title: "Backend Engineer"}
	renamed := job
	renamed.Title = "Platform Engineer"
	items := []*Item{
		{ID: "old-active", Status: "new", Job: job},
		{ID: "new-active", Status: "planned", Job: renamed},
		{ID: "archived-copy", Status: "skipped", Job: job},
	}
	summary := BuildSummary(items, 5)
	if summary.ActiveDuplicateGroups != 1 || summary.ActiveDuplicateItems != 2 {
		t.Fatalf("duplicate counts = groups:%d items:%d, want 1 group and 2 active items", summary.ActiveDuplicateGroups, summary.ActiveDuplicateItems)
	}
	groups := ActiveDuplicateGroups(items)
	if len(groups) != 1 || len(groups[0].Items) != 2 || groups[0].Items[0].ID != "new-active" || groups[0].Items[1].ID != "old-active" {
		t.Fatalf("duplicate groups = %#v", groups)
	}
	if groups[0].Items[0].ExternalID != "123" || groups[0].Items[0].URL != "" {
		t.Fatalf("duplicate item shape = %#v", groups[0].Items[0])
	}
}

func TestActiveDuplicateGroupsDoNotBridgeDifferentATSJobsThroughLegacyURL(t *testing.T) {
	sharedURL := "https://example.com/apply"
	first := Job{Provider: "greenhouse", Board: "acme", ExternalID: "1", Company: "Acme", Title: "Backend Engineer", URL: sharedURL}
	legacy := Job{Company: "Acme", Title: "Backend Engineer", URL: sharedURL}
	second := Job{Provider: "greenhouse", Board: "acme", ExternalID: "2", Company: "Acme", Title: "Platform Engineer", URL: sharedURL}
	groups := ActiveDuplicateGroups([]*Item{
		{ID: "ats-one", Status: "new", Job: first},
		{ID: "legacy", Status: "new", Job: legacy},
		{ID: "ats-two", Status: "new", Job: second},
	})
	if len(groups) != 1 || len(groups[0].Items) != 2 {
		t.Fatalf("groups = %#v, want one legacy bridge without collapsing both ATS jobs", groups)
	}
}
