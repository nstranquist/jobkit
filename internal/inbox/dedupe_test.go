package inbox

import (
	"testing"
	"time"
)

func TestPlanDuplicateRepairsKeepsWorkflowOwnerAndLatestMetadata(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	oldJob := Job{Provider: "ashby", Board: "acme", ExternalID: "123", Company: "Acme", Title: "Backend Engineer"}
	newJob := oldJob
	newJob.Title = "Platform Engineer"
	items := []*Item{
		{ID: "planned-old", Status: "planned", Job: oldJob, CreatedAt: t0, UpdatedAt: t0, LastSeenAt: t0},
		{ID: "new-latest", Status: "new", Job: newJob, CreatedAt: t0.Add(time.Hour), UpdatedAt: t0.Add(time.Hour), LastSeenAt: t0.Add(time.Hour)},
	}
	plans := PlanDuplicateRepairs(items)
	if len(plans) != 1 {
		t.Fatalf("plans = %#v, want one", plans)
	}
	plan := plans[0]
	if plan.WinnerID != "planned-old" || plan.LatestID != "new-latest" {
		t.Fatalf("plan = %#v, want workflow winner and latest metadata", plan)
	}
	if len(plan.LoserIDs) != 1 || plan.LoserIDs[0] != "new-latest" || plan.BlockedReason != "" {
		t.Fatalf("plan = %#v, want one safe loser", plan)
	}
}

func TestPlanDuplicateRepairsBlocksTwoWorkflowOwners(t *testing.T) {
	job := Job{Provider: "greenhouse", Board: "acme", ExternalID: "123", Company: "Acme", Title: "Backend Engineer"}
	renamed := job
	renamed.Title = "Platform Engineer"
	plans := PlanDuplicateRepairs([]*Item{
		{ID: "one", Status: "planned", Job: job},
		{ID: "two", Status: "shortlisted", Job: renamed},
	})
	if len(plans) != 1 || plans[0].BlockedReason == "" {
		t.Fatalf("plans = %#v, want blocked workflow collision", plans)
	}
}
