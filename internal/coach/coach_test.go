package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

func testBundle() *SourceBundle {
	return &SourceBundle{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-08-12T18:00:00Z",
		Scope:         "public",
		Candidate: Candidate{
			Name: "Test Person", Headline: "Senior platform engineer", Skills: []string{"Go", "SQLite"},
		},
		Projects: []ProjectCard{
			{
				ID: "catalog", Name: "Catalog", Summary: "A typed Go software catalog with drift checks.",
				URL: "https://github.com/example/catalog", Skills: []string{"Go", "SQLite", "platform engineering"},
				Decisions: []string{"typed entities", "fail-closed drift"},
				Evidence:  []Evidence{{ID: "catalog-readme", Label: "Public README", URL: "https://github.com/example/catalog", ClaimIDs: []string{"claim-devs"}}},
			},
			{
				ID: "retrieval", Name: "Retrieval", Summary: "A local documentation retrieval CLI.",
				URL: "https://github.com/example/retrieval", Skills: []string{"Go", "retrieval"},
				Evidence: []Evidence{{ID: "retrieval-readme", Label: "Public README", URL: "https://github.com/example/retrieval"}},
			},
		},
		Stories: []Story{{
			ID: "platform-rollout", Title: "Platform rollout", Situation: "Teams needed one platform.",
			Task: "Own the rollout.", Actions: []string{"Built the service", "Measured adoption"},
			Result: "The platform served 1,000+ developers.", Skills: []string{"platform engineering"},
			EvidenceIDs: []string{"catalog-readme"}, ClaimIDs: []string{"claim-devs"},
		}},
		Claims:        []Claim{{ID: "claim-devs", Text: "served 1,000+ developers", Source: "https://example.com/evidence"}},
		SourceDigests: map[string]string{"resume": "sha256:" + strings.Repeat("a", 64), "github": "sha256:" + strings.Repeat("b", 64)},
	}
}

func TestSourceValidationRejectsPrivatePathsAndBrokenReferences(t *testing.T) {
	bundle := testBundle()
	if err := bundle.Validate(); err != nil {
		t.Fatalf("valid bundle: %v", err)
	}
	bundle.Projects[0].Summary = "Built in /Users/test/private"
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not public-safe") {
		t.Fatalf("private path was not rejected: %v", err)
	}
	bundle = testBundle()
	bundle.Projects[0].Evidence[0].ClaimIDs = []string{"missing"}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "unknown claim") {
		t.Fatalf("broken claim reference was not rejected: %v", err)
	}
	bundle = testBundle()
	bundle.Projects[0].URL = "http://127.0.0.1/private"
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "public URL") {
		t.Fatalf("local URL was not rejected: %v", err)
	}
	bundle = testBundle()
	bundle.SourceDigests["resume"] = "sha256:short"
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "64 hexadecimal") {
		t.Fatalf("short digest was not rejected: %v", err)
	}
	bundle = testBundle()
	bundle.Candidate.Headline = "Contact person@example.com"
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "email-address") {
		t.Fatalf("email address was not rejected: %v", err)
	}
	bundle = testBundle()
	bundle.Projects[0].Evidence[0].URL = ""
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "label and public URL") {
		t.Fatalf("missing evidence URL was not rejected: %v", err)
	}
	raw, err := json.Marshal(testBundle())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSource(append(raw, []byte(" {}")...)); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing JSON was not rejected: %v", err)
	}
}

func TestBuildDeckIsDeterministicAndRoleSpecific(t *testing.T) {
	postingText := "Senior Developer Platform Engineer\nCompany: Example\nRequirements\n- Go\n- SQLite\n- Platform engineering\n"
	posting := jd.Parse(postingText)
	now := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	first, err := BuildDeck(testBundle(), posting, postingText, DeckOptions{Mode: ModeMixed, Minutes: 20, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildDeck(testBundle(), posting, postingText, DeckOptions{Mode: ModeMixed, Minutes: 20, Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("deck id changed with clock: %s != %s", first.ID, second.ID)
	}
	if len(first.Questions) != 5 {
		t.Fatalf("question count = %d, want 5", len(first.Questions))
	}
	if first.ProjectIDs[0] != "catalog" {
		t.Fatalf("top project = %q, want catalog", first.ProjectIDs[0])
	}
	if !contains(first.RoleKeywords, "go") || !contains(first.RoleKeywords, "sqlite") {
		t.Fatalf("role keywords = %v", first.RoleKeywords)
	}
}

func TestValidateSessionInputRejectsStaleSourceAndMalformedAnswers(t *testing.T) {
	bundle := testBundle()
	deck, err := BuildDeck(bundle, &jd.JD{Title: "Senior Platform Engineer"}, "role", DeckOptions{
		Mode: ModeMixed, Minutes: 12, Now: time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	answers := make([]Answer, 0, len(deck.Questions))
	for _, question := range deck.Questions {
		answers = append(answers, Answer{QuestionID: question.ID})
	}
	if err := ValidateSessionInput(deck, bundle, answers); err != nil {
		t.Fatalf("valid input: %v", err)
	}

	stale := *bundle
	stale.Candidate.Headline = "Changed public source"
	if err := ValidateSessionInput(deck, &stale, answers); err == nil || !strings.Contains(err.Error(), "stale source") {
		t.Fatalf("expected stale source error, got %v", err)
	}
	if err := ValidateSessionInput(deck, bundle, answers[:len(answers)-1]); err == nil || !strings.Contains(err.Error(), "missing question") {
		t.Fatalf("expected missing answer error, got %v", err)
	}
	duplicate := append(append([]Answer(nil), answers...), answers[0])
	if err := ValidateSessionInput(deck, bundle, duplicate); err == nil || !strings.Contains(err.Error(), "repeat question") {
		t.Fatalf("expected duplicate answer error, got %v", err)
	}
	unknown := append([]Answer(nil), answers...)
	unknown[0].QuestionID = "unknown"
	if err := ValidateSessionInput(deck, bundle, unknown); err == nil || !strings.Contains(err.Error(), "unknown question") {
		t.Fatalf("expected unknown answer error, got %v", err)
	}
}

func TestEvaluateCapsUnsupportedClaimsAndSchedulesReview(t *testing.T) {
	postingText := "Senior Go Platform Engineer\nRequirements\n- Go\n- Platform engineering\n"
	deck, err := BuildDeck(testBundle(), jd.Parse(postingText), postingText, DeckOptions{Mode: ModeProject, Minutes: 8})
	if err != nil {
		t.Fatal(err)
	}
	answer := "The problem affected 50,000 users. I owned the architecture and built the Go service because the alternative increased risk. The result improved observability and rollout safety."
	answers := make([]Answer, len(deck.Questions))
	for i, question := range deck.Questions {
		answers[i] = Answer{QuestionID: question.ID, Text: answer}
	}
	completed := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	session := Evaluate(deck, testBundle(), answers, completed.Add(-10*time.Minute), completed)
	if session.ClaimViolations == 0 {
		t.Fatal("unsupported quantified claim was not detected")
	}
	for _, result := range session.Results {
		if result.Score.Total > 59 {
			t.Fatalf("unsafe answer score = %d, want cap at 59", result.Score.Total)
		}
	}
	if got := session.NextReviewAt.Sub(completed); got != 24*time.Hour {
		t.Fatalf("review interval = %s, want 24h", got)
	}
}

func TestEvaluateScopesAllowedClaimsToQuestionEvidence(t *testing.T) {
	bundle := testBundle()
	postingText := "Senior Go Engineer\nRequirements\n- Go\n"
	deck, err := BuildDeck(bundle, jd.Parse(postingText), postingText, DeckOptions{
		Mode: ModeProject, Minutes: 8, ProjectIDs: []string{"retrieval"},
	})
	if err != nil {
		t.Fatal(err)
	}
	answers := make([]Answer, 0, len(deck.Questions))
	for _, question := range deck.Questions {
		answers = append(answers, Answer{QuestionID: question.ID, Text: "I served 1,000+ developers."})
	}
	session := Evaluate(deck, bundle, answers, time.Now(), time.Now())
	if session.ClaimViolations != len(deck.Questions) {
		t.Fatalf("claim violations = %d, want %d; unrelated project evidence must not allow the metric", session.ClaimViolations, len(deck.Questions))
	}
}

func TestStoreRoundTripStatsAndPrivateModes(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSource(testBundle()); err != nil {
		t.Fatal(err)
	}
	postingText := "Senior Go Engineer\nRequirements\n- Go\n"
	deck, err := BuildDeck(testBundle(), jd.Parse(postingText), postingText, DeckOptions{Mode: ModeProject, Minutes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDeck(deck); err != nil {
		t.Fatal(err)
	}
	answers := []Answer{{QuestionID: deck.Questions[0].ID, Text: "I owned the problem and architecture. I built the Go service because the alternative increased risk. The result served 1,000+ developers."}}
	session := Evaluate(deck, testBundle(), answers, time.Now().Add(-time.Minute), time.Now())
	if err := store.AppendSession(session); err != nil {
		t.Fatal(err)
	}
	report, err := store.Stats(session.NextReviewAt.Add(time.Minute), "catalog")
	if err != nil {
		t.Fatal(err)
	}
	if report.Sessions != 1 || report.DueReviews != 1 {
		t.Fatalf("stats = %+v", report)
	}
	for _, path := range []string{store.SourcePath(), store.SessionsPath(), filepath.Join(store.DecksDir(), deck.ID+".json")} {
		private, observed, err := privatefs.Inspect(path, privatefs.FileMode)
		if err != nil {
			t.Fatal(err)
		}
		if !private {
			t.Fatalf("%s protection is not private (mode %o, want 600)", path, observed)
		}
	}
}

func TestProviderFeedbackIsAdvisory(t *testing.T) {
	if os.Getenv("JOBKIT_COACH_HELPER") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		_, _ = os.Stdout.WriteString(`{"schema_version":1,"advisory":true,"summary":"Name the rollback boundary.","model":"test","tokens_in":10,"tokens_out":5}`)
		os.Exit(0)
	}
	t.Setenv("JOBKIT_COACH_HELPER", "1")
	bundle := testBundle()
	postingText := "Senior Go Engineer\nRequirements\n- Go\n"
	deck, err := BuildDeck(bundle, jd.Parse(postingText), postingText, DeckOptions{Mode: ModeProject, Minutes: 8})
	if err != nil {
		t.Fatal(err)
	}
	session := Evaluate(deck, bundle, nil, time.Now(), time.Now())
	cfg := &ProviderConfig{SchemaVersion: 1, Providers: map[string]Command{"test": {Argv: []string{os.Args[0], "-test.run=TestProviderFeedbackIsAdvisory"}}}}
	feedback, err := RunFeedback(context.Background(), cfg, "test", bundle, deck, session)
	if err != nil {
		t.Fatal(err)
	}
	if !feedback.Advisory || feedback.Model != "test" || feedback.Summary == "" {
		t.Fatalf("feedback = %+v", feedback)
	}
}

func TestProviderLimitsAndTranscriberPlaceholder(t *testing.T) {
	buffer := newLimitedBuffer(4)
	if n, err := buffer.Write([]byte("12345")); n != 5 || err == nil || !buffer.exceeded || buffer.String() != "1234" {
		t.Fatalf("limited buffer n=%d err=%v exceeded=%t body=%q", n, err, buffer.exceeded, buffer.String())
	}
	cfg := &ProviderConfig{SchemaVersion: SchemaVersion, Transcriber: &Command{Argv: []string{"transcriber"}}}
	if _, err := RunTranscriber(context.Background(), cfg, "answer.wav"); err == nil || !strings.Contains(err.Error(), "{file}") {
		t.Fatalf("expected transcriber placeholder error, got %v", err)
	}
}

func TestServerScoresSessionsAndRejectsPublicBind(t *testing.T) {
	if err := validateLoopbackAddress("0.0.0.0:7331"); err == nil {
		t.Fatal("public bind was accepted")
	}
	if err := validateLoopbackAddress("127.0.0.1:7331"); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := testBundle()
	if err := store.SaveSource(bundle); err != nil {
		t.Fatal(err)
	}
	postingText := "Senior Go Engineer\nRequirements\n- Go\n"
	deck, err := BuildDeck(bundle, jd.Parse(postingText), postingText, DeckOptions{Mode: ModeProject, Minutes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDeck(deck); err != nil {
		t.Fatal(err)
	}
	answers := make([]Answer, 0, len(deck.Questions))
	for _, question := range deck.Questions {
		answers = append(answers, Answer{QuestionID: question.ID, Text: "I owned the problem and built the Go architecture because it reduced risk."})
	}
	payload, _ := json.Marshal(sessionRequest{DeckID: deck.ID, Answers: answers})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(payload))
	req.Host = "127.0.0.1:7331"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	(&Server{Store: store, Token: "test-token"}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if sessions, err := store.Sessions(); err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %d, err = %v", len(sessions), err)
	}
}

func TestServerConfigListsAdvisoryProvidersInStableOrder(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: store, Token: "test-token", Providers: &ProviderConfig{
		SchemaVersion: SchemaVersion,
		Providers: map[string]Command{
			"ndev-openai-hosted": {Argv: []string{"adapter"}},
			"ndev-local":         {Argv: []string{"adapter"}},
		},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Host = "localhost:7331"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Providers []string `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if strings.Join(body.Providers, ",") != "ndev-local,ndev-openai-hosted" {
		t.Fatalf("providers = %v", body.Providers)
	}
}

func TestServerRejectsDNSRebindingAndCrossOriginWrites(t *testing.T) {
	server := (&Server{Token: "test-token"}).Handler()
	for _, test := range []struct {
		host   string
		origin string
	}{
		{host: "attacker.example:7331"},
		{host: "127.0.0.1:7331", origin: "https://attacker.example"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(`{}`))
		req.Host = test.host
		if test.origin != "" {
			req.Header.Set("Origin", test.origin)
		}
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("host=%q origin=%q status=%d, want 403", test.host, test.origin, rec.Code)
		}
	}
	if !loopbackHost("[::1]:7331") || loopbackHost("127.0.0.1.evil.example:7331") {
		t.Fatal("loopback host classification failed")
	}
}

func TestServerStudyUsesSameScoreAndProgress(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	_, practice, err := cur.Practice("docs-puller", "explain-local-first")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(studyAttemptRequest{
		ModuleID: "docs-puller", PracticeID: "explain-local-first",
		Text: matchingAnswer(practice),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := (&Server{Store: store, Token: "test-token"}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/study/attempt", bytes.NewReader(payload))
	req.Host = "127.0.0.1:7331"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var report StudyReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Attempt == nil || !report.Attempt.Passed {
		t.Fatalf("attempt = %+v", report.Attempt)
	}
	if report.Module == nil || !strings.Contains(report.Module.Purpose, "Local-first") {
		t.Fatalf("teaching content missing: %+v", report.Module)
	}
	if report.Next == nil || report.Next.Prompt == "" {
		t.Fatal("next step missing")
	}
	results, err := store.StudyResults()
	if err != nil || len(results) != 1 || !results[0].Passed {
		t.Fatalf("store results = %+v err=%v", results, err)
	}
}

func TestServerStudyUnknownModuleIsNotFound(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "coach"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/study/modules/no-such-pin", nil)
	req.Host = "127.0.0.1:7331"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	(&Server{Store: store, Token: "test-token"}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestServerRequiresTokenAndBootstrapsHttpOnlyCookie(t *testing.T) {
	server := (&Server{Token: "test-token"}).Handler()
	unauthorized := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	unauthorized.Host = "localhost:7331"
	unauthorizedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResponse.Code)
	}

	bootstrap := httptest.NewRequest(http.MethodGet, "/?token=test-token", nil)
	bootstrap.Host = "localhost:7331"
	bootstrapResponse := httptest.NewRecorder()
	server.ServeHTTP(bootstrapResponse, bootstrap)
	if bootstrapResponse.Code != http.StatusSeeOther {
		t.Fatalf("bootstrap status = %d", bootstrapResponse.Code)
	}
	cookies := bootstrapResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("bootstrap cookies = %#v", cookies)
	}

	index := httptest.NewRequest(http.MethodGet, "/", nil)
	index.Host = "localhost:7331"
	index.AddCookie(cookies[0])
	indexResponse := httptest.NewRecorder()
	server.ServeHTTP(indexResponse, index)
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("index status = %d", indexResponse.Code)
	}
	csp := indexResponse.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("CSP = %q", csp)
	}
	if strings.Contains(indexResponse.Body.String(), "<script>") || strings.Contains(indexResponse.Body.String(), "<style>") {
		t.Fatal("index contains inline executable assets")
	}
}
