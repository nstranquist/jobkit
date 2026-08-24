package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstranquist/jobkit/internal/calibration"
	"github.com/nstranquist/jobkit/internal/company"
	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/profile"
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
	if !strings.Contains(compact, "study") || !strings.Contains(compact, "mint") {
		t.Fatalf("compact help should name study mint:\n%s", compact)
	}
	if len(compact) >= len(full)/3 {
		t.Fatalf("compact help too large: compact=%d full=%d (want < 1/3)", len(compact), len(full))
	}
	if !strings.Contains(full, "--compact") {
		t.Fatalf("full help should document --compact flag")
	}
}

func TestDoctorRejectsAmbiguousFixFlag(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	err := cmdDoctor(parseArgs([]string{"doctor", "permissions", "--fix"}))
	if err == nil {
		t.Fatal("expected --fix to fail")
	}
	var cliErr *envelope.Err
	if !errors.As(err, &cliErr) {
		t.Fatalf("err = %T %v, want envelope error", err, err)
	}
	if cliErr.Code != envelope.CodeInvalidArgs || cliErr.Hint != "use --fix-permissions" {
		t.Fatalf("err = %#v, want exact --fix-permissions guidance", cliErr)
	}
}

func TestApplyPlanReusesTrackerAndOutputForSameInboxItem(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)
	profilePath, err := home.ProfilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.WriteTemplate(profilePath); err != nil {
		t.Fatal(err)
	}
	inboxPath, err := home.InboxPath()
	if err != nil {
		t.Fatal(err)
	}
	il := &inbox.Ledger{Path: inboxPath}
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Company: Acme\nTitle: Platform Engineer\nBuild reliable Go platform services.\n"))
	}))
	t.Cleanup(live.Close)
	prev := jd.HTTPClient
	jd.HTTPClient = live.Client()
	t.Cleanup(func() { jd.HTTPClient = prev })
	job := inbox.Job{
		Company: "Acme", Title: "Platform Engineer", URL: live.URL + "/jobs/platform",
		Location: "Remote - United States", Remote: true, Description: "Build reliable Go platform services.",
		JDText: "Company: Acme\nTitle: Platform Engineer\nBuild reliable Go platform services.\n",
	}
	inboxID := inbox.NewID(job)
	if err := il.Append(inbox.Event{ID: inboxID, Type: inbox.EvSaved, Status: "new", Job: &job, MatchScore: 80}); err != nil {
		t.Fatal(err)
	}
	args := []string{"apply-plan", inboxID, "--allow-unassessed-eligibility"}
	if err := cmdApplyPlan(parseArgs(args)); err != nil {
		t.Fatal(err)
	}
	if err := cmdApplyPlan(parseArgs(args)); err != nil {
		t.Fatal(err)
	}

	ledgerPath, err := home.LedgerPath()
	if err != nil {
		t.Fatal(err)
	}
	apps, err := (&track.Ledger{Path: ledgerPath}).Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("applications = %d, want one reused tracker record", len(apps))
	}
	if len(apps[0].Events) != 2 || apps[0].Tags[track.TagInboxID] != inboxID {
		t.Fatalf("application = %#v, want created + refresh note with inbox identity", apps[0])
	}
	outRoot, err := home.OutDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != apps[0].ID+"--plan" {
		t.Fatalf("output entries = %#v, want one deterministic plan directory", entries)
	}
	items, err := il.Replay()
	if err != nil {
		t.Fatal(err)
	}
	item, err := inbox.Find(items, inboxID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "planned" || len(item.Events) != 2 {
		t.Fatalf("inbox item = %#v, want one planned transition", item)
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

func TestSaveJobsToInboxTreatsATSTitleChangeAsSeen(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	job := jobsearch.Job{
		Provider: "greenhouse", Board: "acme", ID: "job-123", Company: "Acme",
		Title: "Backend Engineer", URL: "https://example.com/jobs/backend",
		ApplyURL: "https://example.com/apply/backend", Description: "Build backend services in Go.",
	}
	first, err := saveJobsToInbox([]jobsearch.Job{job}, "backend", "test:first")
	if err != nil {
		t.Fatal(err)
	}
	if first.New != 1 || first.Seen != 0 {
		t.Fatalf("first stats = %#v, want 1 new", first)
	}
	job.Title = "Senior Platform Engineer"
	job.URL = "https://example.com/jobs/platform"
	job.ApplyURL = "https://example.com/apply/platform"
	second, err := saveJobsToInbox([]jobsearch.Job{job}, "platform", "test:second")
	if err != nil {
		t.Fatal(err)
	}
	if second.New != 0 || second.Seen != 1 {
		t.Fatalf("second stats = %#v, want title change to count as seen", second)
	}
	l, err := openInboxLedger()
	if err != nil {
		t.Fatal(err)
	}
	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Events) != 2 {
		t.Fatalf("items = %#v, want one history with two events", items)
	}
	if items[0].Job.Title != "Senior Platform Engineer" {
		t.Fatalf("latest title = %q, want updated posting metadata", items[0].Job.Title)
	}
}

func TestInboxDedupeApplyPreservesWorkflowOwnerAndLatestMetadata(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	l, err := openInboxLedger()
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	oldJob := inbox.Job{Provider: "ashby", Board: "acme", ExternalID: "123", Company: "Acme", Title: "Backend Engineer"}
	newJob := oldJob
	newJob.Title = "Platform Engineer"
	if err := l.Append(inbox.Event{TS: t0, ID: "planned-old", Type: inbox.EvSaved, Status: "planned", Job: &oldJob}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(inbox.Event{TS: t0.Add(time.Hour), ID: "new-latest", Type: inbox.EvSaved, Status: "new", Job: &newJob}); err != nil {
		t.Fatal(err)
	}
	if err := cmdInbox(parseArgs([]string{"inbox", "dedupe", "--json"})); err != nil {
		t.Fatal(err)
	}
	before, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(before[0].Events)+len(before[1].Events) != 2 {
		t.Fatalf("preview changed the ledger: %#v", before)
	}
	if err := cmdInbox(parseArgs([]string{"inbox", "dedupe", "--apply", "--json"})); err != nil {
		t.Fatal(err)
	}
	after, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	winner, err := inbox.Find(after, "planned-old")
	if err != nil {
		t.Fatal(err)
	}
	loser, err := inbox.Find(after, "new-latest")
	if err != nil {
		t.Fatal(err)
	}
	if winner.Status != "planned" || winner.Job.Title != "Platform Engineer" || winner.SeenCount != 1 {
		t.Fatalf("winner = %#v, want planned workflow with latest metadata and unchanged seen count", winner)
	}
	if loser.Status != "skipped" {
		t.Fatalf("loser status = %q, want skipped", loser.Status)
	}
	if got := inbox.ActiveDuplicateGroups(after); len(got) != 0 {
		t.Fatalf("active duplicates after repair = %#v", got)
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

func TestApplicationEligibilityFailsClosedUnlessExplicitlyOverridden(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)
	path, err := home.EligibilityPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := eligibility.Save(path, eligibility.Template([]string{"St. Louis, MO"}, 7, false)); err != nil {
		t.Fatal(err)
	}

	posting := "Remote - United States. Own an enterprise sales quota."
	assessment, err := enforceApplicationEligibility(parseArgs([]string{"apply", "role.txt"}), "Account Executive", "Remote - United States", true, posting)
	if err == nil || assessment != nil {
		t.Fatalf("assessment=%#v err=%v, want a fail-closed eligibility error", assessment, err)
	}
	var cliErr *envelope.Err
	if !errors.As(err, &cliErr) || !strings.Contains(cliErr.Hint, "--override-eligibility") {
		t.Fatalf("err=%T %v, want override guidance", err, err)
	}

	assessment, err = enforceApplicationEligibility(parseArgs([]string{"apply", "role.txt", "--override-eligibility"}), "Account Executive", "Remote - United States", true, posting)
	if err != nil || assessment == nil || assessment.Status != eligibility.Ineligible {
		t.Fatalf("assessment=%#v err=%v, want reviewed ineligible override", assessment, err)
	}
}

func TestGeneratedResumeTagsCaptureArtifactAndOverride(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(path, []byte("synthetic resume"), 0o600); err != nil {
		t.Fatal(err)
	}
	tags, err := generatedResumeTags(path, "acme--platform", &eligibility.Result{Override: "override-eligibility"})
	if err != nil {
		t.Fatal(err)
	}
	if !track.ValidSHA256Digest(tags[track.TagResumeArtifactDigest]) {
		t.Fatalf("artifact digest = %q", tags[track.TagResumeArtifactDigest])
	}
	if tags[track.TagResumeVariantID] != "jobkit-tailored" || tags[track.TagTailoringReceiptID] != "jobkit:acme--platform" {
		t.Fatalf("tags = %#v", tags)
	}
	if tags[track.TagEligibilityOverride] != "override-eligibility" {
		t.Fatalf("override tag = %q", tags[track.TagEligibilityOverride])
	}
}

func TestTrackTagsFromVerifiedResumeManifest(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(artifact, []byte("verified PDF bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := sha256File(artifact)
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest := "sha256:" + strings.Repeat("c", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	path := filepath.Join(t.TempDir(), "manifest.json")
	body := `{"schema_version":1,"variant_id":"general-v1.7.3","version":"v1.7.3","lifecycle":"current","sendability":"sendable","channel":"sendable","claim_set_version":"v1.0.0","source_digest":"` + sourceDigest + `","claim_set_digest":"` + claimDigest + `","artifacts":{"pdf":"` + digest + `","docx":"` + digest + `","ats":"` + digest + `"},"gates":{"source":"pass","claims":"pass","parity":"pass","ats":"pass","lifecycle_metadata":"pass","visual_nvr":"pass"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	tags, err := trackTagsFromFlags(parseArgs([]string{"track", "set", "acme", "--resume-manifest", path, "--resume-artifact", "pdf", "--resume-artifact-file", artifact}))
	if err != nil {
		t.Fatal(err)
	}
	if tags[track.TagResumeVersion] != "v1.7.3" || tags[track.TagResumeArtifactDigest] != digest || tags[track.TagResumeSourceDigest] != sourceDigest {
		t.Fatalf("tags = %#v", tags)
	}
	if _, err := trackTagsFromFlags(parseArgs([]string{"track", "set", "acme", "--resume-manifest", path, "--resume-artifact-file", artifact, "--resume-version", "v9"})); err == nil {
		t.Fatal("expected explicit version conflict to fail closed")
	}
}

func TestWeeklySlateMarkdownSeparatesLaneHeadings(t *testing.T) {
	assessment := &eligibility.Result{Status: eligibility.Eligible}
	text := renderWeeklySlate(inbox.Slate{
		Policy: inbox.DefaultSlatePolicy(),
		Selections: []inbox.SlateSelection{
			{ID: "one", Lane: inbox.LaneStretch, Company: "Acme", Title: "Staff Engineer", Eligibility: assessment},
			{ID: "two", Lane: inbox.LanePlatform, Company: "Demo", Title: "Platform Engineer", Eligibility: assessment},
		},
	})
	if !strings.Contains(text, "eligibility eligible)\n\n## platform-devex-ai-infra") {
		t.Fatalf("lane heading must be separated from the previous list: %q", text)
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

func writeProfile(t *testing.T) {
	t.Helper()
	path, err := home.ProfilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.WriteTemplate(path); err != nil {
		t.Fatal(err)
	}
}

func TestJDFetchAndMatchFailClosedOnDeadPosting(t *testing.T) {
	t.Setenv("JOBKIT_HOME", t.TempDir())
	writeProfile(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	prev := jd.HTTPClient
	jd.HTTPClient = srv.Client()
	t.Cleanup(func() { jd.HTTPClient = prev })

	if err := cmdJD(parseArgs([]string{"jd", "fetch", srv.URL + "/jobs/1"})); err == nil {
		t.Fatal("jd fetch should fail closed on 404")
	} else if !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "404") {
		t.Fatalf("err=%v, want closed/404", err)
	}

	expired := filepath.Join(t.TempDir(), "dead.txt")
	if err := os.WriteFile(expired, []byte("Company: Acme\nTitle: Engineer\nThis position has been filled.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmdMatch(parseArgs([]string{"match", expired})); err == nil {
		t.Fatal("match should fail closed on expired body")
	}
	if err := cmdApplyPlan(parseArgs([]string{"apply-plan", expired, "--company", "Acme", "--role", "Engineer", "--allow-unassessed-eligibility"})); err == nil {
		t.Fatal("apply-plan should fail closed on expired body")
	}
	outRoot, err := home.OutDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("apply package leaked: %v", entries)
	}
}

func TestApplyPlanBlacklistGatesWithoutWritingUntilOverride(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)
	writeProfile(t)
	bl, err := home.BlacklistPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bl, []byte("schema_version: 1\ncompanies:\n  - Acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	jdPath := filepath.Join(t.TempDir(), "live.txt")
	if err := os.WriteFile(jdPath, []byte("Company: Acme\nTitle: Platform Engineer\nRequirements\n- Go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmdApplyPlan(parseArgs([]string{"apply-plan", jdPath, "--company", "Acme", "--role", "Platform Engineer", "--allow-unassessed-eligibility"})); err == nil {
		t.Fatal("expected blacklist gate")
	}
	outRoot, err := home.OutDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("package written before override: %v", entries)
	}
	if err := cmdApplyPlan(parseArgs([]string{"apply-plan", jdPath, "--company", "Acme", "--role", "Platform Engineer", "--allow-unassessed-eligibility", "--override-eligibility"})); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(outRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%v, want one package after override", entries)
	}
}

func TestInboxRecheckMarksClosedURLWithoutChangingMatchScore(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)
	path, err := home.EligibilityPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := eligibility.Save(path, eligibility.Template([]string{"St. Louis, MO"}, 7, true)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	t.Cleanup(srv.Close)
	prev := jd.HTTPClient
	jd.HTTPClient = srv.Client()
	t.Cleanup(func() { jd.HTTPClient = prev })

	inboxPath, err := home.InboxPath()
	if err != nil {
		t.Fatal(err)
	}
	il := &inbox.Ledger{Path: inboxPath}
	job := inbox.Job{
		Company: "Acme", Title: "Platform Engineer", URL: srv.URL + "/jobs/1",
		Location: "Remote - United States", Remote: true,
		Description: "Build reliable Go platform services.",
		JDText:      "Company: Acme\nTitle: Platform Engineer\nBuild reliable Go platform services.\n",
	}
	id := inbox.NewID(job)
	if err := il.Append(inbox.Event{ID: id, Type: inbox.EvSaved, Status: "new", Job: &job, MatchScore: 82}); err != nil {
		t.Fatal(err)
	}
	if err := cmdInbox(parseArgs([]string{"inbox", "recheck"})); err != nil {
		t.Fatal(err)
	}
	items, err := il.Replay()
	if err != nil {
		t.Fatal(err)
	}
	item, err := inbox.Find(items, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.MatchScore != 82 {
		t.Fatalf("match score moved: %v", item.MatchScore)
	}
	if item.Job.Liveness != "closed" {
		t.Fatalf("liveness=%q job=%#v", item.Job.Liveness, item.Job)
	}
	if item.Job.Eligibility == nil || item.Job.Eligibility.Status != eligibility.Ineligible {
		t.Fatalf("eligibility=%#v, want ineligible closed gate", item.Job.Eligibility)
	}
	if item.NextAction != "skip-ineligible" {
		t.Fatalf("next=%q", item.NextAction)
	}
}

func TestApplyPlanFailsClosedAfterInboxRecheck410(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("JOBKIT_HOME", stateDir)
	writeProfile(t)
	path, err := home.EligibilityPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := eligibility.Save(path, eligibility.Template([]string{"St. Louis, MO"}, 7, true)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	t.Cleanup(srv.Close)
	prev := jd.HTTPClient
	jd.HTTPClient = srv.Client()
	t.Cleanup(func() { jd.HTTPClient = prev })

	inboxPath, err := home.InboxPath()
	if err != nil {
		t.Fatal(err)
	}
	il := &inbox.Ledger{Path: inboxPath}
	job := inbox.Job{
		Company: "Acme", Title: "Platform Engineer", URL: srv.URL + "/jobs/1",
		Location: "Remote - United States", Remote: true,
		Description: "Build reliable Go platform services.",
		JDText:      "Company: Acme\nTitle: Platform Engineer\nBuild reliable Go platform services.\n",
	}
	id := inbox.NewID(job)
	if err := il.Append(inbox.Event{ID: id, Type: inbox.EvSaved, Status: "new", Job: &job, MatchScore: 82}); err != nil {
		t.Fatal(err)
	}
	if err := cmdInbox(parseArgs([]string{"inbox", "recheck"})); err != nil {
		t.Fatal(err)
	}
	if err := cmdApplyPlan(parseArgs([]string{"apply-plan", id, "--allow-unassessed-eligibility"})); err == nil {
		t.Fatal("apply-plan must fail closed after recheck marked the URL 410")
	}
	outRoot, err := home.OutDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("apply package leaked after closed recheck: %v", entries)
	}
}
