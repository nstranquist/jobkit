package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/coach"
	"github.com/nstranquist/jobkit/internal/envelope"
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

func TestCoachStudyCLILaunchResumeAndClaims(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JOBKIT_HOME", root)
	cur, err := coach.LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	_, practice, err := cur.Practice("docs-puller", "explain-local-first")
	if err != nil {
		t.Fatal(err)
	}
	answer := strings.Join(practice.ExpectedConcepts, " ") + " because the local-first constraint required a measured eval."
	args := []string{"coach", "study", "--module", "docs-puller", "--practice", "explain-local-first", "--answer", answer, "--json"}
	if err := cmdCoach(parseArgs(args)); err != nil {
		t.Fatalf("first launch: %v", err)
	}
	store, err := coachStore()
	if err != nil {
		t.Fatal(err)
	}
	results, err := store.StudyResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Passed || results[0].ModuleID != "docs-puller" {
		t.Fatalf("first results = %+v", results)
	}
	if err := cmdCoach(parseArgs(args)); err != nil {
		t.Fatalf("second launch: %v", err)
	}
	results, err = store.StudyResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !results[1].Passed {
		t.Fatalf("second results = %+v", results)
	}
	next := coach.NextIncomplete(cur, results)
	if next == nil || next.Prompt == "" || next.PracticeID == "explain-local-first" {
		t.Fatalf("next after resume = %+v", next)
	}
	if err := cmdCoach(parseArgs([]string{"coach", "study", "claims", "--json"})); err != nil {
		t.Fatalf("claims: %v", err)
	}
}

func TestCoachStudyMintDryRunAndApply(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JOBKIT_HOME", root)
	if err := cmdCoach(parseArgs([]string{"coach", "study", "mint", "--module", "docs-puller", "--dry-run", "--json"})); err != nil {
		t.Fatal(err)
	}
	store, err := coachStore()
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := store.StudyDrafts()
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 0 {
		t.Fatalf("dry-run persisted drafts: %d", len(drafts))
	}
	if err := cmdCoach(parseArgs([]string{"coach", "study", "mint", "--module", "docs-puller", "--apply", "--json"})); err != nil {
		t.Fatal(err)
	}
	drafts, err = store.StudyDrafts()
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) == 0 {
		t.Fatal("apply wrote no drafts")
	}
	overlay, err := store.StudyOverlay()
	if err != nil {
		t.Fatal(err)
	}
	if len(overlay.Practices) == 0 {
		t.Fatal("apply wrote no overlay")
	}
}

func TestCoachStudyListDoesNotRecordAnswer(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	if err := cmdCoach(parseArgs([]string{"coach", "study", "list", "--answer", "should not record"})); err != nil {
		t.Fatal(err)
	}
	store, err := coachStore()
	if err != nil {
		t.Fatal(err)
	}
	results, err := store.StudyResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("list recorded %d results", len(results))
	}
}

func TestCoachStudyUnknownModuleIsInvalidArgs(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	err := cmdCoach(parseArgs([]string{"coach", "study", "show", "no-such-pin"}))
	var cliErr *envelope.Err
	if !errors.As(err, &cliErr) || cliErr.Code != envelope.CodeInvalidArgs {
		t.Fatalf("err = %#v, want INVALID_ARGS", err)
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
