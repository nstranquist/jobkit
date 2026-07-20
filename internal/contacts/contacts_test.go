package contacts

import (
	"path/filepath"
	"testing"
)

func TestLedgerReplaysContactReferralHistory(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "contacts.jsonl")}
	if err := l.Append(Event{
		ID: "acme-ai-jane", Type: EvCreated, Status: "lead", Name: "Jane Manager",
		Company: "Acme AI", Role: "Hiring Manager", Channel: "linkedin", InboxID: "inbox-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(Event{
		ID: "acme-ai-jane", Type: EvReferral, Status: "referral-requested",
		Note: "asked for routing advice", TrackID: "track-1",
	}); err != nil {
		t.Fatal(err)
	}

	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.Status != "referral-requested" {
		t.Fatalf("status = %q, want referral-requested", item.Status)
	}
	if item.InboxID != "inbox-1" || item.TrackID != "track-1" {
		t.Fatalf("links = inbox:%q track:%q", item.InboxID, item.TrackID)
	}
	if item.LastTouchAt.IsZero() {
		t.Fatalf("last touch was not set")
	}
	if len(item.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(item.Events))
	}
}

func TestFindAllowsUniqueSubstring(t *testing.T) {
	items := []*Item{{ID: "acme-ai-jane"}, {ID: "modal-sam"}}
	got, err := Find(items, "modal")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "modal-sam" {
		t.Fatalf("got %q", got.ID)
	}
}
