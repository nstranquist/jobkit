package track

import (
	"path/filepath"
	"testing"
	"time"
)

func tempLedger(t *testing.T) *Ledger {
	t.Helper()
	return &Ledger{Path: filepath.Join(t.TempDir(), "apps.jsonl")}
}

func TestReplayLifecycle(t *testing.T) {
	l := tempLedger(t)
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(l.Append(Event{TS: t0, ID: "initech--swe", Type: EvCreated, Company: "Initech", Role: "SWE", Status: "interested"}))
	must(l.Append(Event{TS: t0.Add(time.Hour), ID: "initech--swe", Type: EvStatus, Status: "applied"}))
	must(l.Append(Event{TS: t0.Add(2 * time.Hour), ID: "initech--swe", Type: EvNote, Note: "referred by Sam"}))
	must(l.Append(Event{TS: t0.Add(3 * time.Hour), ID: "globex--sre", Type: EvCreated, Company: "Globex", Role: "SRE", Status: "discovered"}))

	apps, err := l.Replay()
	must(err)
	if len(apps) != 2 {
		t.Fatalf("apps = %d, want 2", len(apps))
	}
	// Sorted by UpdatedAt desc → globex first.
	if apps[0].ID != "globex--sre" {
		t.Fatalf("first app = %s, want globex--sre", apps[0].ID)
	}
	a, err := Find(apps, "initech")
	must(err)
	if a.Status != "applied" {
		t.Fatalf("status = %s, want applied", a.Status)
	}
	if a.AppliedAt != t0.Add(time.Hour) {
		t.Fatalf("appliedAt = %v", a.AppliedAt)
	}
	if len(a.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(a.Events))
	}
}

func TestNewIDCollision(t *testing.T) {
	apps := []*Application{{ID: "acme--engineer"}}
	if got := NewID(apps, "Acme", "Engineer"); got != "acme--engineer-2" {
		t.Fatalf("got %s", got)
	}
	if got := NewID(nil, "Acme, Inc.", "Sr. Engineer (Platform)"); got != "acme-inc--sr-engineer-platform" {
		t.Fatalf("slug id = %s", got)
	}
}

func TestStatsAndFollowups(t *testing.T) {
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	apps := []*Application{
		{ID: "a", Status: "applied", AppliedAt: old, UpdatedAt: old},
		{ID: "b", Status: "interview", AppliedAt: now.Add(-3 * 24 * time.Hour), UpdatedAt: now},
		{ID: "c", Status: "rejected", AppliedAt: old, UpdatedAt: now},
		{ID: "d", Status: "interested", UpdatedAt: now},
	}
	s := BuildStats(apps, now)
	if s.Total != 4 || s.Active != 3 {
		t.Fatalf("total=%d active=%d", s.Total, s.Active)
	}
	if s.Applied != 3 {
		t.Fatalf("applied = %d, want 3", s.Applied)
	}
	// b (interview) and c (rejected) count as responses; a does not.
	if s.Responded != 2 {
		t.Fatalf("responded = %d, want 2", s.Responded)
	}
	due := FollowUps(apps, 7, now)
	if len(due) != 1 || due[0].ID != "a" {
		t.Fatalf("followups = %+v", due)
	}
}

func TestFindAmbiguous(t *testing.T) {
	apps := []*Application{{ID: "acme--swe"}, {ID: "acme--sre"}}
	if _, err := Find(apps, "acme"); err == nil {
		t.Fatal("expected ambiguity error")
	}
	a, err := Find(apps, "swe")
	if err != nil || a.ID != "acme--swe" {
		t.Fatalf("a=%v err=%v", a, err)
	}
}
