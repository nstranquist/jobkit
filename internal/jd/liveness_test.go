package jd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchFailsClosedOn404And410(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusGone} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			t.Cleanup(srv.Close)
			_, err := FetchWithClient(srv.Client(), srv.URL+"/jobs/1")
			if !IsClosed(err) {
				t.Fatalf("err=%v, want ClosedError", err)
			}
		})
	}
}

func TestFetchFailsClosedOnExpiredBody(t *testing.T) {
	html := `<html><body><h1>Senior Go Engineer</h1><p>This position has been filled.</p></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)
	_, err := FetchWithClient(srv.Client(), srv.URL+"/jobs/1")
	if !IsClosed(err) {
		t.Fatalf("err=%v, want ClosedError for expired body", err)
	}
}

func TestRequireLiveRejectsClosedTextAndAllowsLive(t *testing.T) {
	if err := RequireLive("This job is no longer available. Apply next time."); err == nil || !IsClosed(err) {
		t.Fatalf("expected closed error, got %v", err)
	}
	live := "Senior Backend Engineer\nCompany: Initech\nRequirements\n- Go\n- PostgreSQL\nClose-knit team."
	if err := RequireLive(live); err != nil {
		t.Fatalf("live JD should pass: %v", err)
	}
}

func TestFetchLiveHTMLStillExtracts(t *testing.T) {
	html := `<html><body><h1>Senior Backend Engineer</h1><p>Requirements</p><ul><li>Go</li></ul></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(srv.Close)
	text, err := FetchWithClient(srv.Client(), srv.URL+"/jobs/1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Senior Backend Engineer") {
		t.Fatalf("text=%q", text)
	}
}
