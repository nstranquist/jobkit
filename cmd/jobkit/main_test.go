package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/calibration"
	"github.com/nstranquist/jobkit/internal/company"
	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/track"
)

func TestHelpCompactIsTokenEfficient(t *testing.T) {
	full := helpText(false)
	compact := helpText(true)
	if !strings.Contains(compact, "--compact") {
		t.Fatalf("compact help should mention --compact:\n%s", compact)
	}
	if !strings.Contains(compact, "find <q>") {
		t.Fatalf("compact help should list core verbs:\n%s", compact)
	}
	if len(compact) >= len(full)/3 {
		t.Fatalf("compact help too large: compact=%d full=%d (want < 1/3)", len(compact), len(full))
	}
	if !strings.Contains(full, "--compact") {
		t.Fatalf("full help should document --compact flag")
	}
}

func TestSaveJobsToInboxDedupesWithinRun(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())

	job := jobsearch.Job{
		Provider:    "greenhouse",
		Board:       "acme",
		ID:          "job-123",
		Company:     "Acme",
		Title:       "Backend Engineer",
		URL:         "https://example.com/jobs/123",
		ApplyURL:    "https://example.com/apply/123",
		Description: "Build backend services in Go.",
	}
	stats, err := saveJobsToInbox([]jobsearch.Job{job, job}, "backend", "test")
	if err != nil {
		t.Fatal(err)
	}
	if stats.New != 1 || stats.Seen != 1 {
		t.Fatalf("stats = %#v, want 1 new and 1 seen", stats)
	}
	l, err := openInboxLedger()
	if err != nil {
		t.Fatal(err)
	}
	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if len(items[0].Events) != 2 {
		t.Fatalf("events = %d, want 2", len(items[0].Events))
	}
}

func TestPlanSourceReturnsCorruptInboxError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JOBKIT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "inbox.jsonl"), []byte("{not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, company, role, sourceURL, inboxID, err := planSource("acme--backend-engineer-deadbeef")
	if err == nil {
		t.Fatalf("expected corrupt inbox error, got text=%q company=%q role=%q sourceURL=%q inboxID=%q", text, company, role, sourceURL, inboxID)
	}
	var cliErr *envelope.Err
	if !errors.As(err, &cliErr) {
		t.Fatalf("err = %T %v, want envelope error", err, err)
	}
	if cliErr.Code != envelope.CodeIOFailed {
		t.Fatalf("code = %s, want %s", cliErr.Code, envelope.CodeIOFailed)
	}
	if !strings.Contains(cliErr.Message, "inbox.jsonl:1") {
		t.Fatalf("message = %q, want inbox line context", cliErr.Message)
	}
}

func TestPlanSourceFileDoesNotNeedReadableInbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JOBKIT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "inbox.jsonl"), []byte("{not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	jdPath := filepath.Join(home, "jd.txt")
	jdText := "Company: Acme\nTitle: Backend Engineer\nBuild Go services.\n"
	if err := os.WriteFile(jdPath, []byte(jdText), 0o644); err != nil {
		t.Fatal(err)
	}

	text, company, role, sourceURL, inboxID, err := planSource(jdPath)
	if err != nil {
		t.Fatal(err)
	}
	if text != jdText {
		t.Fatalf("text = %q, want %q", text, jdText)
	}
	if company != "" || role != "" || sourceURL != "" || inboxID != "" {
		t.Fatalf("metadata = company:%q role:%q sourceURL:%q inboxID:%q, want empty", company, role, sourceURL, inboxID)
	}
}

func TestCompanyNoteUsesExistingMultiWordCompanyName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JOBKIT_HOME", home)

	if err := dispatch("company", parseArgs([]string{"company", "add", "Acme AI"})); err != nil {
		t.Fatal(err)
	}
	if err := dispatch("company", parseArgs([]string{"company", "note", "Acme", "AI", "strong", "fit"})); err != nil {
		t.Fatal(err)
	}

	cfg, err := company.Load(filepath.Join(home, "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Companies) != 1 {
		t.Fatalf("companies = %#v, want only Acme AI", cfg.Companies)
	}
	co, ok := cfg.Find("Acme AI")
	if !ok {
		t.Fatalf("Acme AI not found")
	}
	if len(co.Notes) != 1 || co.Notes[0].Text != "strong fit" {
		t.Fatalf("notes = %#v, want one strong fit note", co.Notes)
	}
}

func TestCompanyNoteResolvesMatchedCompanyName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JOBKIT_HOME", home)

	if err := dispatch("company", parseArgs([]string{"company", "add", "Acme AI"})); err != nil {
		t.Fatal(err)
	}
	if err := dispatch("company", parseArgs([]string{"company", "note", "Acme", "strong", "fit"})); err != nil {
		t.Fatal(err)
	}

	cfg, err := company.Load(filepath.Join(home, "companies.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Companies) != 1 {
		t.Fatalf("companies = %#v, want note on existing Acme AI only", cfg.Companies)
	}
	co, ok := cfg.Find("Acme AI")
	if !ok {
		t.Fatalf("Acme AI not found")
	}
	if len(co.Notes) != 1 || co.Notes[0].Text != "strong fit" {
		t.Fatalf("notes = %#v, want one strong fit note", co.Notes)
	}
}

func TestCompanyNoteRejectsUnknownAmbiguousMultiWordName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JOBKIT_HOME", home)

	err := dispatch("company", parseArgs([]string{"company", "note", "New", "Co", "strong", "fit"}))
	if err == nil {
		t.Fatal("expected ambiguous company note error")
	}
	var cliErr *envelope.Err
	if !errors.As(err, &cliErr) {
		t.Fatalf("err = %T %v, want envelope error", err, err)
	}
	if cliErr.Code != envelope.CodeInvalidArgs {
		t.Fatalf("code = %s, want %s", cliErr.Code, envelope.CodeInvalidArgs)
	}
	if !strings.Contains(cliErr.Hint, "--note") {
		t.Fatalf("hint = %q, want --note guidance", cliErr.Hint)
	}
}

func TestCalibrateApplyWritesImprovedWeights(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)

	inboxPath, err := home.InboxPath()
	if err != nil {
		t.Fatal(err)
	}
	il := &inbox.Ledger{Path: inboxPath}
	positiveJob := inbox.Job{
		Company: "Acme", Title: "Backend Platform Engineer", URL: "https://example.com/high",
		Opportunity: jobsearch.Opportunity{Score: 75, CompScore: 35, Persona: "agent-infra"},
	}
	negativeJob := inbox.Job{
		Company: "Demo", Title: "Generalist Engineer", URL: "https://example.com/low",
		Opportunity: jobsearch.Opportunity{Score: 100, FreshnessScore: 25, Persona: "agent-infra"},
	}
	positiveID := inbox.NewID(positiveJob)
	negativeID := inbox.NewID(negativeJob)
	if err := il.Append(inbox.Event{ID: positiveID, Type: inbox.EvSaved, Status: "planned", Job: &positiveJob, MatchScore: 40}); err != nil {
		t.Fatal(err)
	}
	if err := il.Append(inbox.Event{ID: negativeID, Type: inbox.EvSaved, Status: "skipped", Job: &negativeJob, MatchScore: 75}); err != nil {
		t.Fatal(err)
	}

	trackPath, err := home.LedgerPath()
	if err != nil {
		t.Fatal(err)
	}
	tl := &track.Ledger{Path: trackPath}
	if err := tl.Append(track.Event{ID: "acme--backend-platform-engineer", Type: track.EvCreated, Company: "Acme", Role: "Backend Platform Engineer", URL: positiveJob.URL, Status: "screening"}); err != nil {
		t.Fatal(err)
	}
	if err := tl.Append(track.Event{ID: "demo--generalist-engineer", Type: track.EvCreated, Company: "Demo", Role: "Generalist Engineer", URL: negativeJob.URL, Status: "rejected"}); err != nil {
		t.Fatal(err)
	}

	if err := dispatch("calibrate", parseArgs([]string{"calibrate", "apply", "--persona", "agent-infra", "--force"})); err != nil {
		t.Fatal(err)
	}
	cfgPath, err := home.CalibrationPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := calibration.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Metrics.PairwiseAccuracy != 1 {
		t.Fatalf("metrics = %#v, want perfect pairwise accuracy", cfg.Metrics)
	}
	if cfg.Weights.Compensation <= jobsearch.DefaultOpportunityWeights().Compensation {
		t.Fatalf("weights = %#v, want compensation weight increased", cfg.Weights)
	}
}
