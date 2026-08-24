package main

import (
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/inbox"
)

func TestBoundInboxListRejectsImplicitLargeJSON(t *testing.T) {
	items := make([]*inbox.Item, maximumImplicitInboxJSONItems+1)
	_, err := boundInboxList(parseArgs([]string{"inbox", "list", "--json"}), items)
	if err == nil || !strings.Contains(err.Error(), "exceeds the implicit output limit") {
		t.Fatalf("err = %v", err)
	}
	bounded, err := boundInboxList(parseArgs([]string{"inbox", "list", "--json", "--limit", "12"}), items)
	if err != nil || len(bounded) != 12 {
		t.Fatalf("bounded = %d, err = %v", len(bounded), err)
	}
	full, err := boundInboxList(parseArgs([]string{"inbox", "list", "--json", "--limit", "0"}), items)
	if err != nil || len(full) != len(items) {
		t.Fatalf("full = %d, err = %v", len(full), err)
	}
}

func TestRenderInboxSummaryIsCompact(t *testing.T) {
	summary := inbox.Summary{
		Total: 649, Active: 620, ReadyTotal: 1, Returned: 1, HardConstraintsPending: 11,
		Ready: []inbox.SummaryItem{{ID: "acme--platform", Company: "Acme", Title: "Platform Engineer", Lane: inbox.LanePlatform, MatchScore: 84, Eligibility: eligibility.Eligible}},
	}
	text := renderInboxSummary(summary)
	if !strings.Contains(text, "649 total") || !strings.Contains(text, "0 active duplicate groups") || !strings.Contains(text, "acme--platform") || len(text) > 500 {
		t.Fatalf("summary = %q", text)
	}
}

func TestRenderInboxDuplicatesIsActionableAndDescriptionFree(t *testing.T) {
	groups := []inbox.DuplicateGroup{{
		Identity: "ats|greenhouse|acme|123",
		Items: []inbox.DuplicateItem{
			{ID: "old-id", Status: "new", Company: "Acme", Title: "Backend Engineer", Provider: "greenhouse", Board: "acme", ExternalID: "123"},
			{ID: "new-id", Status: "planned", Company: "Acme", Title: "Platform Engineer", Provider: "greenhouse", Board: "acme", ExternalID: "123"},
		},
	}}
	text := renderInboxDuplicates(groups)
	if !strings.Contains(text, "Active duplicate groups: 1") || !strings.Contains(text, "old-id") || !strings.Contains(text, "new-id") {
		t.Fatalf("duplicates = %q", text)
	}
	if strings.Contains(text, "description") {
		t.Fatalf("duplicate output leaked a description field: %q", text)
	}
	if got := renderInboxDuplicates(nil); got != "no active duplicate inbox records\n" {
		t.Fatalf("empty duplicates = %q", got)
	}
}

func TestRenderInboxDedupePlanRequiresExplicitApply(t *testing.T) {
	plans := []inbox.DuplicateRepair{{
		Identity: "ats|greenhouse|acme|123", WinnerID: "keep-id", LatestID: "latest-id", LoserIDs: []string{"old-id"},
	}}
	text := renderInboxDedupePlan(plans)
	if !strings.Contains(text, "keep: keep-id") || !strings.Contains(text, "skip: old-id") || !strings.Contains(text, "dedupe --apply") {
		t.Fatalf("dedupe plan = %q", text)
	}
}
