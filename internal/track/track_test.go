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

func TestTagsReplayMerge(t *testing.T) {
	l := tempLedger(t)
	t0 := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(l.Append(Event{TS: t0, ID: "acme--swe", Type: EvCreated, Company: "Acme", Role: "SWE", Status: "interested",
		Tags: map[string]string{TagResumeVersion: "v1.7.3", TagLane: "ai-platform"}}))
	must(l.Append(Event{TS: t0.Add(time.Hour), ID: "acme--swe", Type: EvStatus, Status: "applied",
		Tags: map[string]string{TagSource: "referral", TagLane: "fullstack"}}))
	apps, err := l.Replay()
	must(err)
	a := apps[0]
	if a.Tags[TagResumeVersion] != "v1.7.3" {
		t.Fatalf("resume_version = %q", a.Tags[TagResumeVersion])
	}
	if a.Tags[TagLane] != "fullstack" { // later event wins
		t.Fatalf("lane = %q, want fullstack", a.Tags[TagLane])
	}
	if a.Tags[TagSource] != "referral" {
		t.Fatalf("source = %q", a.Tags[TagSource])
	}
}

func TestStatsByTag(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	applied := now.Add(-2 * 24 * time.Hour)
	apps := []*Application{
		{ID: "a", Status: "applied", AppliedAt: applied, Tags: map[string]string{TagLane: "ai-platform"}},
		{ID: "b", Status: "interview", AppliedAt: applied, Tags: map[string]string{TagLane: "ai-platform"}},
		{ID: "c", Status: "applied", AppliedAt: applied, Tags: map[string]string{TagLane: "fullstack"}},
		{ID: "d", Status: "interested", Tags: map[string]string{TagLane: "fullstack"}},
		{ID: "e", Status: "applied", AppliedAt: applied}, // untagged
	}
	s := BuildStats(apps, now)
	ai := s.ByTag[TagLane]["ai-platform"]
	if ai == nil || ai.Applied != 2 || ai.Responded != 1 || ai.Interviews != 1 {
		t.Fatalf("ai-platform row = %+v", ai)
	}
	if ai.ResponseRate != 0.5 {
		t.Fatalf("ai-platform response rate = %v", ai.ResponseRate)
	}
	fs := s.ByTag[TagLane]["fullstack"]
	if fs == nil || fs.Total != 2 || fs.Applied != 1 || fs.Responded != 0 {
		t.Fatalf("fullstack row = %+v", fs)
	}
}

func TestStatsByTagEmpty(t *testing.T) {
	s := BuildStats([]*Application{{ID: "a", Status: "applied"}}, time.Now())
	if s.ByTag != nil {
		t.Fatalf("ByTag = %+v, want nil when nothing is tagged", s.ByTag)
	}
}

func TestParseTagSpec(t *testing.T) {
	got, err := ParseTagSpec("Resume-Version=v1.7.3, lane=ai-platform")
	if err != nil {
		t.Fatal(err)
	}
	if got["resume_version"] != "v1.7.3" || got["lane"] != "ai-platform" {
		t.Fatalf("got %+v", got)
	}
	if _, err := ParseTagSpec("novalue"); err == nil {
		t.Fatal("expected error for missing =")
	}
	if _, err := ParseTagSpec("k="); err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestFindAmbiguous(t *testing.T) {
	apps := []*Application{{ID: "acme--swe"}, {ID: "acme--sre"}}
	got, err := Find(apps, "acme")
	if err == nil {
		t.Fatalf("expected ambiguity error, got %+v", got)
	}
	a, err := Find(apps, "swe")
	if err != nil || a.ID != "acme--swe" {
		t.Fatalf("a=%v err=%v", a, err)
	}
}
