package jobsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestParseBoards(t *testing.T) {
	got, err := ParseBoards("greenhouse:acme,lever:demo,https://jobs.ashbyhq.com/Ashby")
	if err != nil {
		t.Fatal(err)
	}
	want := []Board{
		{Provider: "greenhouse", Slug: "acme"},
		{Provider: "lever", Slug: "demo"},
		{Provider: "ashby", Slug: "Ashby"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("boards = %#v, want %#v", got, want)
	}
}

func TestSearchFetchesAndFiltersPublicBoards(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gh/boards/acme/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("content") != "true" {
			t.Fatalf("greenhouse content query = %q, want true", r.URL.Query().Get("content"))
		}
		writeJSON(t, w, map[string]any{"jobs": []any{
			map[string]any{
				"id": 101, "title": "Senior Backend Engineer", "absolute_url": "https://job-boards.greenhouse.io/acme/jobs/101",
				"location": map[string]any{"name": "Remote US"}, "content": "<p>Build APIs in Go and Postgres.</p>",
				"departments": []any{map[string]any{"name": "Engineering"}},
			},
			map[string]any{
				"id": 102, "title": "Product Designer", "absolute_url": "https://job-boards.greenhouse.io/acme/jobs/102",
				"location": map[string]any{"name": "New York"}, "content": "<p>Design systems.</p>",
			},
		}})
	})
	mux.HandleFunc("/lever/postings/demo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mode") != "json" {
			t.Fatalf("lever mode = %q, want json", r.URL.Query().Get("mode"))
		}
		writeJSON(t, w, []any{
			map[string]any{
				"id": "lev1", "text": "Backend Platform Engineer", "hostedUrl": "https://jobs.lever.co/demo/lev1",
				"applyUrl": "https://jobs.lever.co/demo/lev1/apply", "descriptionPlain": "Go services and observability.",
				"workplaceType": "remote", "categories": map[string]any{"team": "Platform", "location": "Remote"},
			},
		})
	})
	mux.HandleFunc("/ashby/posting-api/job-board/Ashby", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"jobs": []any{
			map[string]any{
				"id": "ash1", "title": "Backend Product Engineer", "location": "San Francisco",
				"department": "Engineering", "isRemote": false, "descriptionPlain": "TypeScript and product work.",
				"jobUrl": "https://jobs.ashbyhq.com/Ashby/ash1", "applyUrl": "https://jobs.ashbyhq.com/Ashby/ash1/application",
			},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JOBKIT_GREENHOUSE_BASE", srv.URL+"/gh")
	t.Setenv("JOBKIT_LEVER_BASE", srv.URL+"/lever")
	t.Setenv("JOBKIT_ASHBY_BASE", srv.URL+"/ashby")

	jobs, err := Search(context.Background(), Options{
		Query:      "backend go",
		Boards:     []Board{{Provider: "greenhouse", Slug: "acme"}, {Provider: "lever", Slug: "demo"}, {Provider: "ashby", Slug: "Ashby"}},
		RemoteOnly: true,
		Limit:      10,
		Client:     srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %#v, want 2 remote backend go matches", jobs)
	}
	if jobs[0].Provider != "greenhouse" || jobs[0].Title != "Senior Backend Engineer" {
		t.Fatalf("first result = %#v, want Greenhouse backend engineer ranked first", jobs[0])
	}
	if jobs[1].Provider != "lever" {
		t.Fatalf("second result = %#v, want Lever result", jobs[1])
	}
}

func TestSearchReportReturnsPartialResultsWithWarnings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gh/boards/acme/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"jobs": []any{
			map[string]any{
				"id": 101, "title": "Backend Go Engineer", "absolute_url": "https://job-boards.greenhouse.io/acme/jobs/101",
				"location": map[string]any{"name": "Remote US"}, "content": "<p>Build APIs in Go.</p>",
			},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JOBKIT_GREENHOUSE_BASE", srv.URL+"/gh")
	t.Setenv("JOBKIT_LEVER_BASE", srv.URL+"/lever")

	result, err := SearchReport(context.Background(), Options{
		Query:  "backend go",
		Boards: []Board{{Provider: "greenhouse", Slug: "acme"}, {Provider: "lever", Slug: "missing"}},
		Limit:  10,
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs = %#v, want one partial result", result.Jobs)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one skipped-board warning", result.Warnings)
	}
	if result.Warnings[0].Provider != "lever" || result.Warnings[0].Board != "missing" {
		t.Fatalf("warning = %#v, want lever:missing", result.Warnings[0])
	}
}

func TestSearchReportStrictFailsOnFirstBoardError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gh/boards/acme/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"jobs": []any{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JOBKIT_GREENHOUSE_BASE", srv.URL+"/gh")
	t.Setenv("JOBKIT_LEVER_BASE", srv.URL+"/lever")

	result, err := SearchReport(context.Background(), Options{
		Query:  "backend",
		Boards: []Board{{Provider: "greenhouse", Slug: "acme"}, {Provider: "lever", Slug: "missing"}},
		Strict: true,
		Limit:  10,
		Client: srv.Client(),
	})
	if err == nil {
		t.Fatalf("expected strict board error, got result=%#v", result)
	}
}

func TestExtractCompensationRanges(t *testing.T) {
	cases := []struct {
		text string
		min  int
		max  int
		cur  string
	}{
		{"Compensation Range: $293K - $385K USD", 293000, 385000, "USD"},
		{"Annual base salary range:\n\n$218,025 — $256,500 USD", 218025, 256500, "USD"},
		{"Pay Range\n$191,100 — $191,100 CAD", 191100, 191100, "CAD"},
	}
	for _, tc := range cases {
		got := ExtractCompensation(tc.text)
		if got == nil {
			t.Fatalf("ExtractCompensation(%q) = nil", tc.text)
		}
		if got.Min != tc.min || got.Max != tc.max || got.Currency != tc.cur {
			t.Fatalf("ExtractCompensation(%q) = %#v, want min=%d max=%d cur=%s", tc.text, got, tc.min, tc.max, tc.cur)
		}
	}
}

func TestSearchSortsAndFiltersByCompensation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/gh/boards/acme/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"jobs": []any{
			map[string]any{
				"id": 101, "title": "Backend Platform Engineer", "absolute_url": "https://example.com/high",
				"location": map[string]any{"name": "Remote US"}, "content": "<p>Build APIs in Go.</p><p>Compensation Range: $260K - $340K USD</p>",
			},
			map[string]any{
				"id": 102, "title": "Backend Platform Engineer", "absolute_url": "https://example.com/low",
				"location": map[string]any{"name": "Remote US"}, "content": "<p>Build APIs in Go.</p><p>Compensation Range: $150K - $180K USD</p>",
			},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JOBKIT_GREENHOUSE_BASE", srv.URL+"/gh")

	jobs, err := Search(context.Background(), Options{
		Query: "backend go", Boards: []Board{{Provider: "greenhouse", Slug: "acme"}},
		Limit: 10, Sort: "comp", MinComp: 200000, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %#v, want one high-comp match", jobs)
	}
	if jobs[0].URL != "https://example.com/high" {
		t.Fatalf("first URL = %q, want high comp posting", jobs[0].URL)
	}
	if jobs[0].Compensation == nil || jobs[0].Compensation.Max != 340000 {
		t.Fatalf("compensation = %#v, want max 340000", jobs[0].Compensation)
	}
}

func TestFetchAshbyParsesStructuredCompensation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ashby/posting-api/job-board/Ashby", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includeCompensation") != "true" {
			t.Fatalf("includeCompensation = %q, want true", r.URL.Query().Get("includeCompensation"))
		}
		writeJSON(t, w, map[string]any{"jobs": []any{
			map[string]any{
				"id": "ash1", "title": "Backend Platform Engineer", "location": "Remote US",
				"department": "Engineering", "isRemote": true, "descriptionPlain": "Build backend systems.",
				"jobUrl": "https://jobs.ashbyhq.com/Ashby/ash1", "applyUrl": "https://jobs.ashbyhq.com/Ashby/ash1/application",
				"compensation": map[string]any{
					"compensationTierSummary": "$220K – $310K • Offers Equity",
					"summaryComponents": []any{
						map[string]any{
							"compensationType": "Salary", "interval": "1 YEAR", "currencyCode": "USD",
							"minValue": 220000, "maxValue": 310000,
						},
					},
				},
			},
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("JOBKIT_ASHBY_BASE", srv.URL+"/ashby")

	jobs, err := Search(context.Background(), Options{
		Query: "backend", Boards: []Board{{Provider: "ashby", Slug: "Ashby"}}, Limit: 10, Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %#v, want one Ashby result", jobs)
	}
	if jobs[0].Compensation == nil || jobs[0].Compensation.Min != 220000 || jobs[0].Compensation.Max != 310000 {
		t.Fatalf("compensation = %#v, want structured Ashby salary", jobs[0].Compensation)
	}
}

func TestBuildOpportunityScoresPersonaFreshnessAndSaturation(t *testing.T) {
	job := Job{
		Title:       "Senior Software Engineer",
		Department:  "Agent Infrastructure",
		Location:    "Remote US",
		Remote:      true,
		Description: "Build agent orchestration, cloud infrastructure, observability, and distributed platform systems.",
		PublishedAt: time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
		Score:       12,
		Compensation: &Compensation{
			Min: 275000, Max: 350000, Currency: "USD", Period: "year",
		},
	}

	opp := BuildOpportunity(job, "agent-infra")
	if opp.Score <= job.Score {
		t.Fatalf("opportunity score = %d, want boosted above base score %d", opp.Score, job.Score)
	}
	if opp.FreshnessScore == 0 || opp.CompScore == 0 || opp.PersonaScore == 0 {
		t.Fatalf("opportunity = %#v, want freshness/comp/persona scores", opp)
	}
	if opp.SaturationRisk == 0 {
		t.Fatalf("saturation risk = 0, want remote/generic-title risk")
	}
	if opp.Persona != "agent-infra" {
		t.Fatalf("persona = %q, want agent-infra", opp.Persona)
	}
}

func TestBuildOpportunityWithWeightsCanSuppressSaturationPenalty(t *testing.T) {
	job := Job{
		Title:       "Senior Software Engineer",
		Location:    "Remote US",
		Remote:      true,
		Description: "Build backend platforms.",
		Score:       20,
	}

	defaultOpp := BuildOpportunity(job, "")
	weightedOpp := BuildOpportunityWithWeights(job, "", OpportunityWeights{
		Base: 1, Freshness: 1, Compensation: 1, Persona: 1, Saturation: 0,
	})
	if defaultOpp.SaturationRisk == 0 {
		t.Fatalf("default opportunity = %#v, want saturation risk fixture", defaultOpp)
	}
	if weightedOpp.Score <= defaultOpp.Score {
		t.Fatalf("weighted score = %d, default score = %d; want saturation-suppressed boost", weightedOpp.Score, defaultOpp.Score)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func TestScoreJobCoverage(t *testing.T) {
	job := Job{
		Title:       "Senior Software Engineer, Developer Platform",
		Company:     "Acme",
		Description: "Build internal developer platform tooling in Go on AWS with CI/CD golden paths.",
	}
	// Short queries (<=3 terms) stay strict: any absent term kills the match.
	if _, ok := scoreJob(job, queryTerms("engineer kubernetes")); ok {
		t.Fatal("short query with a missing term must not match")
	}
	if score, ok := scoreJob(job, queryTerms("developer platform")); !ok || score <= 0 {
		t.Fatalf("short full match: score=%d ok=%v", score, ok)
	}
	// Long queries tolerate a minority of missing terms (60%% coverage).
	long := queryTerms("senior software engineer backstage developer platform")
	if len(long) != 6 {
		t.Fatalf("terms = %v", long)
	}
	score, ok := scoreJob(job, long) // "backstage" absent, 5/6 matched
	if !ok || score <= 0 {
		t.Fatalf("long query with 5/6 coverage must match: score=%d ok=%v", score, ok)
	}
	// But below the coverage floor it still fails.
	if _, ok := scoreJob(job, queryTerms("rust kernel embedded firmware realtime graphics")); ok {
		t.Fatal("long query with near-zero coverage must not match")
	}
}
