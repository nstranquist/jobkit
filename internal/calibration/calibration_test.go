package calibration

import (
	"path/filepath"
	"testing"

	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/track"
)

func TestBuildExamplesUsesTrackerOutcomeBeforeInboxStatus(t *testing.T) {
	item := &inbox.Item{
		ID:         "acme--backend-deadbeef",
		Status:     "skipped",
		MatchScore: 80,
		Job: inbox.Job{
			Company:     "Acme",
			Title:       "Backend Engineer",
			URL:         "https://example.com/jobs/1?utm_source=feed",
			Description: "Build Go backend systems.",
		},
	}
	app := &track.Application{
		ID:      "acme--backend-engineer",
		Company: "Acme",
		Role:    "Backend Engineer",
		URL:     "https://example.com/jobs/1",
		Status:  "screening",
	}

	examples := BuildExamples([]*inbox.Item{item}, []*track.Application{app}, "backend-platform")
	if len(examples) != 1 {
		t.Fatalf("examples = %d, want 1", len(examples))
	}
	if examples[0].Source != "tracker" || examples[0].Status != "screening" || examples[0].Outcome <= 0 {
		t.Fatalf("example = %#v, want positive tracker outcome", examples[0])
	}
}

func TestSuggestWeightsRanksPositiveAboveNegative(t *testing.T) {
	examples := []Example{
		{
			ID:      "positive",
			Outcome: 2,
			Features: Features{
				Base: 40, Freshness: 0, Compensation: 35, Persona: 0, Saturation: 0,
			},
		},
		{
			ID:      "negative",
			Outcome: -2,
			Features: Features{
				Base: 55, Freshness: 25, Compensation: 0, Persona: 0, Saturation: 25,
			},
		},
	}

	defaultMetrics := Evaluate(examples, jobsearch.DefaultOpportunityWeights())
	suggested := SuggestWeights(examples)
	if suggested.Metrics.PairwiseAccuracy < defaultMetrics.PairwiseAccuracy {
		t.Fatalf("suggested accuracy = %.2f, default = %.2f", suggested.Metrics.PairwiseAccuracy, defaultMetrics.PairwiseAccuracy)
	}
	if suggested.Metrics.PairwiseAccuracy != 1 {
		t.Fatalf("suggested metrics = %#v, want perfect pairwise ranking", suggested.Metrics)
	}
}

func TestConfigSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calibration.yaml")
	cfg := &Config{
		Persona: "agent-infra",
		Samples: 12,
		Weights: jobsearch.OpportunityWeights{
			Base: 1, Freshness: 0.5, Compensation: 2, Persona: 1.5, Saturation: 2,
		},
		Metrics: Metrics{Samples: 12, Positive: 8, Negative: 4, Pairs: 32, PairwiseAccuracy: 0.75},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Persona != cfg.Persona || loaded.Samples != cfg.Samples {
		t.Fatalf("loaded = %#v, want persona/samples from %#v", loaded, cfg)
	}
	if loaded.Weights.Compensation != 2 || loaded.Weights.Saturation != 2 || loaded.Metrics.PairwiseAccuracy != 0.75 {
		t.Fatalf("loaded = %#v, want weights and metrics round trip", loaded)
	}
}
