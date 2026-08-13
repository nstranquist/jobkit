package coach

import (
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/scoring-benchmark.json
var scoringBenchmark []byte

func TestScoringBenchmark(t *testing.T) {
	var benchmark struct {
		RubricVersion string `json:"rubric_version"`
		Cases         []struct {
			Name            string   `json:"name"`
			Mode            Mode     `json:"mode"`
			Expected        []string `json:"expected_concepts"`
			RoleKeywords    []string `json:"role_keywords"`
			TimeSeconds     int      `json:"time_seconds"`
			Answer          string   `json:"answer"`
			AllowedClaims   []string `json:"allowed_claims"`
			MinimumScore    int      `json:"minimum_score"`
			MaximumScore    int      `json:"maximum_score"`
			ClaimViolations int      `json:"claim_violations"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(scoringBenchmark, &benchmark); err != nil {
		t.Fatal(err)
	}
	if benchmark.RubricVersion != RubricVersion {
		t.Fatalf("benchmark rubric = %q, implementation = %q", benchmark.RubricVersion, RubricVersion)
	}
	for _, item := range benchmark.Cases {
		t.Run(item.Name, func(t *testing.T) {
			result := scoreAnswer(Question{
				ID: "benchmark", Mode: item.Mode, ExpectedConcepts: item.Expected,
				TimeSeconds: item.TimeSeconds,
			}, item.RoleKeywords, item.Answer, item.AllowedClaims)
			if result.Score.Total < item.MinimumScore || result.Score.Total > item.MaximumScore {
				t.Fatalf("score = %d, want %d..%d: %#v", result.Score.Total, item.MinimumScore, item.MaximumScore, result.Score)
			}
			if len(result.ClaimViolations) != item.ClaimViolations {
				t.Fatalf("claim violations = %d, want %d", len(result.ClaimViolations), item.ClaimViolations)
			}
		})
	}
}
