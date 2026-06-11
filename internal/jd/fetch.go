package jd

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// IsURL reports whether s looks like a fetchable posting URL.
func IsURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// Fetch downloads a posting URL and returns readable plain text.
func Fetch(url string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; jobkit/0.2; +https://github.com/nstranquist/jobkit)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body := io.LimitReader(resp.Body, 4<<20) // 4 MiB cap
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/plain") || strings.Contains(ct, "application/json") {
		raw, err := io.ReadAll(body)
		return string(raw), err
	}
	return ToText(body)
}

// skip these subtrees entirely when extracting text.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "head": true,
	"nav": true, "footer": true, "iframe": true, "svg": true, "form": true,
	"button": true, "select": true,
}

// block-level tags get their own line.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "main": true,
	"br": true, "tr": true, "ul": true, "ol": true, "table": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "blockquote": true,
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// ToText extracts readable text from HTML, preserving enough line structure
// for the section-weighting heuristics (headers and list items on own lines).
func ToText(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if skipTags[n.Data] {
				return
			}
			if blockTags[n.Data] {
				b.WriteString("\n")
			}
			if n.Data == "li" {
				b.WriteString("\n- ")
			}
		}
		if n.Type == html.TextNode {
			text := strings.Join(strings.Fields(n.Data), " ")
			if text != "" {
				b.WriteString(text)
				b.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			b.WriteString("\n")
		}
	}
	walk(doc)

	// Tidy: trim per-line, collapse blank runs.
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	out := blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(out) + "\n", nil
}
