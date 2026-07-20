package company

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCompanyRankedUsesFreshSignalsAndCompTarget(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	cfg := &Config{}
	co := cfg.Upsert(Company{
		Name: "Acme AI", Tags: []string{"ai", "agents"}, TargetComp: 425000,
	})
	if co.Name != "Acme AI" {
		t.Fatalf("upserted company = %#v", co)
	}
	co, err := cfg.AddSignal("Acme AI", Signal{TS: now.AddDate(0, 0, -3), Type: "funding", Source: "Series B"})
	if err != nil {
		t.Fatal(err)
	}
	if len(co.Signals) != 1 {
		t.Fatalf("signals = %#v, want one", co.Signals)
	}

	ranked := cfg.Ranked(now)
	if len(ranked) != 1 {
		t.Fatalf("ranked = %d, want 1", len(ranked))
	}
	if ranked[0].Score < 45 {
		t.Fatalf("score = %d, want warm-intro threshold", ranked[0].Score)
	}
	if ranked[0].NextAction != "warm-intro-now" {
		t.Fatalf("next action = %q, want warm-intro-now", ranked[0].NextAction)
	}
	if ranked[0].LastSignal.IsZero() {
		t.Fatalf("last signal was not set")
	}
}

func TestCompanySaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companies.yaml")
	cfg := &Config{}
	co := cfg.Upsert(Company{Name: "Modal", Domain: "modal.com", Tags: []string{"ai", "platform"}, Boards: []string{"ashby:Modal"}})
	if co.Name != "Modal" {
		t.Fatalf("upserted company = %#v", co)
	}
	co, err := cfg.AddNote("Modal", "strong infra fit")
	if err != nil {
		t.Fatal(err)
	}
	if len(co.Notes) != 1 {
		t.Fatalf("notes = %#v, want one", co.Notes)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	co, ok := got.Find("Modal")
	if !ok {
		t.Fatalf("Modal not found after load")
	}
	if co.Domain != "modal.com" || len(co.Boards) != 1 || len(co.Notes) != 1 {
		t.Fatalf("loaded company = %#v", co)
	}
}
