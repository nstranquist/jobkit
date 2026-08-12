package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoachCLIFlow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JOBKIT_HOME", root)
	source := filepath.Join(root, "source-input.json")
	jdPath := filepath.Join(root, "posting.txt")
	answersPath := filepath.Join(root, "answers.json")
	sourceJSON := `{
  "schema_version":1,
  "generated_at":"2026-08-12T18:00:00Z",
  "scope":"public",
  "candidate":{"name":"Test Person","skills":["Go"]},
  "projects":[{"id":"catalog","name":"Catalog","summary":"Typed Go catalog.","url":"https://example.com/catalog","skills":["Go"],"evidence":[{"id":"readme","label":"README","url":"https://example.com/catalog"}]}],
  "stories":[{"id":"launch","title":"Catalog launch","situation":"Metadata drifted.","task":"Create one contract.","actions":["Built the catalog."],"result":"The public fixture indexed 1,200 services.","evidence_ids":["readme"]}],
  "claims":[{"id":"scale","text":"indexed 1,200 services in the public fixture","source":"https://example.com/catalog"}],
  "source_digests":{"public":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}`
	answersJSON := `{"answers":[
  {"question_id":"q01","text":"The problem was metadata drift. I owned the architecture and built the Go catalog because warnings increased risk. The result was a typed contract."},
  {"question_id":"q02","text":"Situation: metadata drifted. Task: create one contract. Action: I built the catalog. Result: the public fixture indexed 1,200 services."},
  {"question_id":"q03","text":"Requirements lead to a typed data flow. Failure modes emit observability signals. The rollout uses a shadow index."}
]}`
	if err := os.WriteFile(source, []byte(sourceJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jdPath, []byte("Senior Go Platform Engineer\nRequirements\n- Go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(answersPath, []byte(answersJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"coach", "source", "import", source},
		{"coach", "deck", "--job", jdPath, "--mode", "mixed", "--minutes", "12"},
		{"coach", "run", "latest", "--answers", answersPath},
		{"coach", "stats"},
	} {
		c := parseArgs(args)
		if err := cmdCoach(c); err != nil {
			t.Fatalf("jobkit %v: %v", args, err)
		}
	}
	store, err := coachStore()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
}

func TestLoadCoachAnswersRejectsUnknownAndTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	for _, body := range []string{
		`{"answers":[],"unexpected":true}`,
		`{"answers":[]} {}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadCoachAnswers(path); err == nil || (!strings.Contains(err.Error(), "unknown field") && !strings.Contains(err.Error(), "trailing JSON")) {
			t.Fatalf("body %q: expected strict JSON error, got %v", body, err)
		}
	}
}
