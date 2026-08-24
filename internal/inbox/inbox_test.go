package inbox

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nstranquist/jobkit/internal/eligibility"
)

func TestLedgerReplayOrdersAndUpdatesItems(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "inbox.jsonl")}
	job := Job{Company: "Acme", Title: "Backend Engineer", URL: "https://example.com/jobs/1"}
	id := NewID(job)
	if err := l.Append(Event{ID: id, Type: EvSaved, Status: "new", Job: &job, MatchScore: 82, NextAction: NextAction(82)}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{ID: id, Type: EvStatus, Status: "planned", Note: "plan written"}); err != nil {
		t.Fatal(err)
	}
	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Status != "planned" || items[0].MatchScore != 82 || items[0].NextAction != "apply-plan" {
		t.Fatalf("item = %#v", items[0])
	}
	if len(items[0].Events) != 2 {
		t.Fatalf("events = %d, want 2", len(items[0].Events))
	}
}

func TestLedgerReplayTracksSeenEvents(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "inbox.jsonl")}
	t0 := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * time.Hour)
	job := Job{Company: "Acme", Title: "Backend Engineer", URL: "https://example.com/jobs/1"}
	id := NewID(job)
	if err := l.Append(Event{TS: t0, ID: id, Type: EvSaved, Status: "new", Source: "search:backend", Query: "backend", Job: &job}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{TS: t1, ID: id, Type: EvSeen, Source: "search:backend", Query: "backend", Job: &job}); err != nil {
		t.Fatal(err)
	}
	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if !items[0].LastSeenAt.Equal(t1) || items[0].SeenCount != 2 {
		t.Fatalf("lastSeen=%s seenCount=%d, want %s and 2", items[0].LastSeenAt, items[0].SeenCount, t1)
	}
}

func TestNewIDIsStableAcrossATSTitleAndURLChanges(t *testing.T) {
	job := Job{
		Provider: "greenhouse", Board: "acme", ExternalID: "job-123",
		Company: "Acme Corp", Title: "Senior Backend Engineer", ApplyURL: "https://example.com/apply/old",
	}
	changed := job
	changed.Title = "Senior Platform Engineer"
	changed.URL = "https://example.com/jobs/new-title"
	changed.ApplyURL = "https://example.com/apply/new"
	a := NewID(job)
	b := NewID(changed)
	if a != b {
		t.Fatalf("ATS identity changed with mutable metadata: %q != %q", a, b)
	}
	if want := "acme--job-123-"; len(a) <= len(want) || a[:len(want)] != want {
		t.Fatalf("id = %q, want readable prefix %q", a, want)
	}
}

func TestNewIDUsesNormalizedURLForManualJobs(t *testing.T) {
	a := NewID(Job{Company: "Acme", Title: "Backend Engineer", URL: "https://EXAMPLE.com/jobs/1/?utm_source=mail#apply"})
	b := NewID(Job{Company: "Acme", Title: "Platform Engineer", URL: "https://example.com/jobs/1"})
	if a != b {
		t.Fatalf("manual URL identity changed with title/tracking metadata: %q != %q", a, b)
	}
}

func TestResolveIDReusesLegacyHistoryAcrossTitleChange(t *testing.T) {
	oldJob := Job{Provider: "greenhouse", Board: "acme", ExternalID: "123", Company: "Acme", Title: "Backend Engineer"}
	newJob := oldJob
	newJob.Title = "Senior Platform Engineer"
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	items := []*Item{
		{ID: "acme--backend-engineer-legacy", Status: "skipped", Job: oldJob, UpdatedAt: t0.Add(time.Hour)},
		{ID: "acme--platform-engineer-active", Status: "planned", Job: oldJob, UpdatedAt: t0},
	}
	if got := ResolveID(items, newJob); got != "acme--platform-engineer-active" {
		t.Fatalf("resolved ID = %q, want active legacy history", got)
	}
	items = append(items, &Item{ID: "acme--already-applied", Status: "applied", Job: oldJob, UpdatedAt: t0.Add(-time.Hour)})
	if got := ResolveID(items, newJob); got != "acme--already-applied" {
		t.Fatalf("resolved ID = %q, want applied history to prevent reapplication", got)
	}
}

func TestSamePostingUsesURLForLegacyRecordButTrustsCompleteATSIdentity(t *testing.T) {
	legacy := Job{Company: "Acme", Title: "Backend Engineer", URL: "https://example.com/jobs/1?utm_source=mail"}
	current := Job{Provider: "greenhouse", Board: "acme", ExternalID: "1", Company: "Acme", Title: "Platform Engineer", URL: "https://example.com/jobs/1"}
	if !SamePosting(legacy, current) {
		t.Fatal("legacy URL should bridge to the current ATS identity")
	}
	other := current
	other.ExternalID = "2"
	if SamePosting(current, other) {
		t.Fatal("different complete ATS identities must not collapse even when a provider reuses a URL")
	}
}

func TestGreenhouseIdentityNormalizesLegacyScientificNotation(t *testing.T) {
	legacy := Job{Provider: "greenhouse", Board: "figma", ExternalID: "6.100023004e+09", Company: "figma", Title: "Software Engineer - Mobile Web"}
	current := legacy
	current.ExternalID = "6100023004"
	current.Title = "Software Engineer, Mobile Web"
	if !SamePosting(legacy, current) {
		t.Fatal("legacy scientific Greenhouse ID must match the exact decimal ID")
	}
	if NewID(legacy) != NewID(current) {
		t.Fatalf("normalized IDs differ: %q != %q", NewID(legacy), NewID(current))
	}
}

func TestNextActionWithEligibilityOverridesFitScore(t *testing.T) {
	ineligible := &eligibility.Result{Status: eligibility.Ineligible}
	if got := NextActionWithEligibility(99, ineligible); got != "skip-ineligible" {
		t.Fatalf("got %q, want skip-ineligible", got)
	}
	review := &eligibility.Result{Status: eligibility.Review}
	if got := NextActionWithEligibility(99, review); got != "review-eligibility" {
		t.Fatalf("got %q, want review-eligibility", got)
	}
}
