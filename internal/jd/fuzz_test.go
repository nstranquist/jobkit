package jd

import (
	"strings"
	"testing"
)

// validSeniority is the closed set detectSeniority may return.
var validSeniority = map[string]bool{
	"staff+": true, "senior": true, "junior": true, "mid": true,
}

// FuzzParse drives the job-description text parser with arbitrary bytes. The
// parser mixes regexp section detection, byte-offset span weighting, and
// word-boundary scans over lowercased input, so it is the highest-risk
// boundary in the package. Invariants asserted are cheap and code-guaranteed:
// no panic, a non-nil result, weights bounded by maxSkillWeight, positive
// counts, a seniority drawn from the closed set, and determinism.
func FuzzParse(f *testing.F) {
	f.Add(sampleJD)
	f.Add("")
	f.Add("Senior Backend Engineer\nCompany: Initech\n")
	f.Add("Requirements\n- 5+ years of Go and PostgreSQL\nNice to have\n- Rust\n")
	f.Add("We use gRPC, ruby on rails, and react native.")
	f.Add("title: Staff SRE\nk8s, terraform, ci/cd\n")
	f.Add("Google Sheets and react to feedback")
	// Multi-byte, control, and boundary-adjacent bytes exercise the
	// byte-offset word-boundary logic (escaped so no raw bytes reach source).
	f.Add("héllo wörld go\x00golang\tk8s.\n日本語 rust")
	f.Add(strings.Repeat("go ", 500))

	f.Fuzz(func(t *testing.T, raw string) {
		j := Parse(raw)
		if j == nil {
			t.Fatal("Parse returned nil")
		}
		for _, s := range j.Skills {
			if s.Weight < 0 || s.Weight > maxSkillWeight {
				t.Fatalf("skill %q weight %v out of [0,%v]", s.Name, s.Weight, maxSkillWeight)
			}
			if s.Count < 1 {
				t.Fatalf("skill %q count %d < 1", s.Name, s.Count)
			}
		}
		if !validSeniority[j.Seniority] {
			t.Fatalf("seniority %q not in closed set", j.Seniority)
		}
		// Parsing is a pure function of the input; a second pass must agree.
		j2 := Parse(raw)
		if len(j.Skills) != len(j2.Skills) || j.Title != j2.Title ||
			j.Company != j2.Company || j.Seniority != j2.Seniority {
			t.Fatalf("non-deterministic parse for %q", raw)
		}
	})
}

// FuzzToText drives the HTML-to-text extractor (golang.org/x/net/html parse
// plus a recursive tree walk). It seeds with realistic markup and asserts the
// extractor never panics, always emits a trailing newline on success, and that
// its output flows into Parse without panicking (the real fetch pipeline).
func FuzzToText(f *testing.F) {
	f.Add(`<html><head><title>x</title><script>var a=1;</script></head><body><h1>Senior Backend Engineer</h1><p>Company: Initech</p><ul><li>5+ years of Go</li><li>Kubernetes</li></ul></body></html>`)
	f.Add(``)
	f.Add(`<p>plain</p>`)
	f.Add(`<div><nav>Home</nav><li>Go`)
	f.Add(`<table><tr><td>Rust</td></tr></table>`)
	f.Add("<p> � héllo</p>")
	f.Add(`<!-- comment --><body>text<br>more`)

	f.Fuzz(func(t *testing.T, htmlSrc string) {
		text, err := ToText(strings.NewReader(htmlSrc))
		if err != nil {
			return // read/parse errors are acceptable; only panics are bugs
		}
		if !strings.HasSuffix(text, "\n") {
			t.Fatalf("ToText output must end with newline, got %q", text)
		}
		// The extracted text feeds directly into Parse in Fetch; make sure the
		// whole pipeline is panic-free on adversarial markup.
		_ = Parse(text)
	})
}
