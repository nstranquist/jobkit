package coach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func matchingAnswer(practice Practice) string {
	return strings.Join(practice.ExpectedConcepts, " ") + " because the architecture and constraint made that the honest path."
}

func loadTwoModuleFixture(t *testing.T) *Curriculum {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "study-two-module.json"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := ParseCurriculum(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cur
}

func TestShippedCurriculumLoadsSixPinsInStableOrder(t *testing.T) {
	cur, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	ordered := cur.OrderedModules()
	if len(ordered) != len(RequiredPinIDs) {
		t.Fatalf("modules = %d, want %d", len(ordered), len(RequiredPinIDs))
	}
	for i, id := range RequiredPinIDs {
		if ordered[i].ID != id {
			t.Fatalf("order[%d]=%q, want %q", i, ordered[i].ID, id)
		}
		if strings.TrimSpace(ordered[i].Purpose) == "" || ordered[i].RunDemo == "" {
			t.Fatalf("module %q missing purpose or run/demo", id)
		}
		if len(ordered[i].Architecture)+len(ordered[i].Decisions) == 0 {
			t.Fatalf("module %q missing architecture/decisions", id)
		}
		if len(ordered[i].Practices) == 0 {
			t.Fatalf("module %q missing practice", id)
		}
	}
	again, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	for i := range ordered {
		if again.OrderedModules()[i].ID != ordered[i].ID {
			t.Fatal("pin order is not stable across loads")
		}
	}
}

func TestFixtureTwoModulesLoadAndKeepOrder(t *testing.T) {
	cur := loadTwoModuleFixture(t)
	if len(cur.OrderedModules()) != 2 {
		t.Fatalf("fixture modules = %d, want 2", len(cur.OrderedModules()))
	}
	if cur.Order[0] != "docs-puller" || cur.Order[1] != "nicos-catalog" {
		t.Fatalf("fixture order = %v", cur.Order)
	}
}

func TestScorePracticeMatchMismatchAndClaimReject(t *testing.T) {
	cur := loadTwoModuleFixture(t)
	module, practice, err := cur.Practice("docs-puller", "explain-local-first")
	if err != nil {
		t.Fatal(err)
	}
	match := ScorePractice(module, practice, matchingAnswer(practice), cur.ClaimMap())
	if !match.Passed || match.Verdict != "pass" || match.Score < practice.PassThreshold {
		t.Fatalf("matching attempt = %+v", match)
	}
	if len(match.MissingConcepts) != 0 {
		t.Fatalf("matching attempt still missing %v", match.MissingConcepts)
	}

	miss := ScorePractice(module, practice, "I like search products.", cur.ClaimMap())
	if miss.Passed || miss.Score >= practice.PassThreshold {
		t.Fatalf("mismatched attempt scored pass: %+v", miss)
	}
	if miss.Score >= match.Score {
		t.Fatalf("mismatch score %d was not below match score %d", miss.Score, match.Score)
	}
	if len(miss.MissingConcepts) == 0 {
		t.Fatal("mismatched attempt reported no missing concepts")
	}

	unsafe := ScorePractice(module, practice, matchingAnswer(practice)+" We improved 47% delivery.", cur.ClaimMap())
	if unsafe.Passed || unsafe.Verdict != "claim_rejected" || len(unsafe.ClaimViolations) == 0 {
		t.Fatalf("untraceable claim was not rejected: %+v", unsafe)
	}
}

func TestCurriculumRejectsUntraceableQuantifiedClaim(t *testing.T) {
	cur := loadTwoModuleFixture(t)
	cur.Modules[0].Purpose += " Improved 47% delivery."
	if err := ValidateCurriculum(cur); err == nil || !strings.Contains(err.Error(), "untraceable quantified claim") {
		t.Fatalf("expected untraceable claim rejection, got %v", err)
	}
}

func TestCurriculumRejectsFactoryPaths(t *testing.T) {
	cur := loadTwoModuleFixture(t)
	cur.Modules[0].TalkingPoints = append(cur.Modules[0].TalkingPoints, "See nicos-tools factory notes.")
	if err := ValidateCurriculum(cur); err == nil || !strings.Contains(err.Error(), "not public-safe") {
		t.Fatalf("factory path was not rejected: %v", err)
	}
}

func TestProgressAdvancesAndSecondLoadResumes(t *testing.T) {
	cur := loadTwoModuleFixture(t)
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	first := NextIncomplete(cur, nil)
	if first == nil || first.ModuleID != "docs-puller" || first.PracticeID != "explain-local-first" {
		t.Fatalf("first item = %+v", first)
	}

	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	module, practice, err := cur.Practice(first.ModuleID, first.PracticeID)
	if err != nil {
		t.Fatal(err)
	}
	report, err := LaunchCurriculum(store, cur, LaunchOptions{
		ModuleID: module.ID, PracticeID: practice.ID, Answer: matchingAnswer(practice), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Attempt == nil || !report.Attempt.Passed {
		t.Fatalf("attempt = %+v", report.Attempt)
	}
	if report.Next == nil || report.Next.PracticeID == first.PracticeID {
		t.Fatalf("next did not advance: %+v", report.Next)
	}
	if report.Next.ModuleID != "docs-puller" || report.Next.PracticeID != "run-demo" {
		t.Fatalf("next = %+v, want docs-puller/run-demo", report.Next)
	}

	reloaded, err := NewStore(store.Root)
	if err != nil {
		t.Fatal(err)
	}
	results, err := reloaded.StudyResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("reloaded results = %+v", results)
	}
	resumed := NextIncomplete(cur, results)
	if resumed == nil || resumed.PracticeID != report.Next.PracticeID {
		t.Fatalf("resumed next = %+v, want %+v", resumed, report.Next)
	}
}

func TestClaimTraceCoversExtractedTokens(t *testing.T) {
	cur, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	rows := ClaimTrace(cur)
	if len(rows) == 0 {
		t.Fatal("claim trace was empty")
	}
	for _, row := range rows {
		if row.Token == "(none)" {
			continue
		}
		if row.ClaimID == "" || row.Authority == "" {
			t.Fatalf("untraced token %q in module %s", row.Token, row.ModuleID)
		}
	}
}

func TestShippedDocsPullerPracticeScoresOnRealEntry(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	module, practice, err := cur.Practice("docs-puller", "explain-local-first")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Launch(store, LaunchOptions{
		ModuleID: "docs-puller", PracticeID: "explain-local-first", Answer: matchingAnswer(practice),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Module == nil || report.Module.Purpose != module.Purpose {
		t.Fatal("launch did not return teaching content from the shipped module")
	}
	if report.Attempt == nil || !report.Attempt.Passed {
		t.Fatalf("shipped matching attempt = %+v", report.Attempt)
	}
	if report.Next == nil || report.Next.Prompt == "" {
		t.Fatal("next step missing after a recorded result")
	}
	names := make([]string, 0, len(report.Modules))
	for _, summary := range report.Modules {
		names = append(names, summary.ID)
	}
	if strings.Join(names, ",") != strings.Join(RequiredPinIDs, ",") {
		t.Fatalf("launch module list = %v", names)
	}
}
