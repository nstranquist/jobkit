package jd

import (
	"strings"
	"testing"
)

func TestToTextExtractsStructure(t *testing.T) {
	html := `<html><head><title>x</title><script>var a=1;</script>
<style>.b{}</style></head><body>
<nav>Home | Jobs</nav>
<h1>Senior Backend Engineer</h1>
<p>Company: Initech</p>
<h2>Requirements</h2>
<ul><li>5+ years of <b>Go</b></li><li>Kubernetes</li></ul>
<footer>© Initech</footer>
</body></html>`
	text, err := ToText(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Senior Backend Engineer", "Requirements", "- 5+ years of Go", "- Kubernetes"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	for _, banned := range []string{"var a=1", ".b{}", "Home | Jobs", "© Initech"} {
		if strings.Contains(text, banned) {
			t.Fatalf("leaked %q in:\n%s", banned, text)
		}
	}
	// Structure preserved well enough for section weighting: a JD parsed
	// from this HTML must mark Go as required.
	j := Parse(text)
	for _, s := range j.Skills {
		if s.Name == "Go" {
			if !s.Required {
				t.Fatal("Go under Requirements header must be required")
			}
			return
		}
	}
	t.Fatal("Go not detected in extracted text")
}

func TestIsURL(t *testing.T) {
	if !IsURL("https://example.com/job") || IsURL("examples/jd.txt") || IsURL("-") {
		t.Fatal("IsURL misclassifies")
	}
}
