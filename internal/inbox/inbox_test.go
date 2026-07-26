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

func TestNewIDIsStableAndIncludesReadableSlug(t *testing.T) {
	job := Job{Company: "Acme Corp", Title: "Senior Backend Engineer", ApplyURL: "https://example.com/apply"}
	a := NewID(job)
	b := NewID(job)
	if a != b {
		t.Fatalf("ids not stable: %q != %q", a, b)
	}
	if want := "acme-corp--senior-backend-engineer-"; len(a) <= len(want) || a[:len(want)] != want {
		t.Fatalf("id = %q, want readable prefix %q", a, want)
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
