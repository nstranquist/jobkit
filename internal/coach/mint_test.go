package coach

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMintModuleProducesValidDiscriminatingQuiz(t *testing.T) {
	cur, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 18, 0, 0, 0, time.UTC)
	drafts, err := MintModule(cur, "docs-puller", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) == 0 {
		t.Fatal("mint produced no drafts")
	}
	module, err := cur.Module("docs-puller")
	if err != nil {
		t.Fatal(err)
	}
	var quiz *Practice
	for i := range drafts {
		if drafts[i].Practice.Kind == PracticeQuiz && len(drafts[i].Practice.Choices) >= 2 {
			quiz = &drafts[i].Practice
			break
		}
	}
	if quiz == nil {
		t.Fatal("mint produced no quiz")
	}
	if err := validateMintedPractice(cur, "docs-puller", *quiz); err != nil {
		t.Fatalf("minted quiz failed curriculum validate: %v", err)
	}
	pass := ScorePractice(module, *quiz, quiz.Choices[0], cur.ClaimMap())
	if !pass.Passed {
		t.Fatalf("correct choice failed: %+v", pass)
	}
	fail := ScorePractice(module, *quiz, quiz.Choices[1], cur.ClaimMap())
	if fail.Passed {
		t.Fatalf("distractor passed: %+v", fail)
	}
}

func TestMintDryRunWritesNothing(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := MintToStore(store, "snapref", false, true, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Drafts) == 0 {
		t.Fatal("dry-run should still return drafts")
	}
	if result.Written != 0 || result.Applied != 0 {
		t.Fatalf("dry-run wrote state: %+v", result)
	}
	drafts, err := store.StudyDrafts()
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 0 {
		t.Fatalf("dry-run persisted %d drafts", len(drafts))
	}
}

func TestMintApplyScoresThroughScorePractice(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 18, 1, 0, 0, time.UTC)
	result, err := MintToStore(store, "docs-puller", true, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written == 0 || result.Applied == 0 {
		t.Fatalf("apply did not persist: %+v", result)
	}
	cur, err := LoadEffectiveCurriculum(store)
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	shippedMod, _ := shipped.Module("docs-puller")
	effMod, _ := cur.Module("docs-puller")
	if len(effMod.Practices) <= len(shippedMod.Practices) {
		t.Fatalf("overlay did not add practices: shipped=%d effective=%d", len(shippedMod.Practices), len(effMod.Practices))
	}
	var draft StudyDraft
	for _, row := range result.Drafts {
		if row.Practice.Kind == PracticeQuiz && len(row.Practice.Choices) > 0 {
			draft = row
			break
		}
	}
	if draft.ID == "" {
		t.Fatal("no quiz draft to score")
	}
	report, err := Launch(store, LaunchOptions{
		DraftID: draft.ID, Answer: draft.Practice.Choices[0], Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Attempt == nil || !report.Attempt.Passed {
		t.Fatalf("draft attempt = %+v", report.Attempt)
	}
	blob := strings.ToLower(draft.Practice.Prompt + strings.Join(draft.Practice.Choices, " "))
	if strings.Contains(blob, "github.com/nstranquist/nicos-dj") {
		t.Fatal("mint invented a nicos-dj remote")
	}
}
