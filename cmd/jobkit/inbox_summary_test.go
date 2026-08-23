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
	if !strings.Contains(text, "649 total") || !strings.Contains(text, "acme--platform") || len(text) > 500 {
		t.Fatalf("summary = %q", text)
	}
}
