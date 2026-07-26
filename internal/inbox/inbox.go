// Package inbox stores a deduped queue of saved jobs before they become
// applications. It is deliberately append-only JSONL: replay is the database.
package inbox

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

var Statuses = []string{"new", "shortlisted", "planned", "applied", "skipped", "archived"}
var TerminalStatuses = map[string]bool{"applied": true, "skipped": true, "archived": true}

func ValidStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

const (
	EvSaved    = "saved"
	EvSeen     = "seen"
	EvAssessed = "eligibility_assessed"
	EvStatus   = "status_changed"
	EvNote     = "note"
)

type Event struct {
	TS         time.Time `json:"ts"`
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status,omitempty"`
	Note       string    `json:"note,omitempty"`
	Source     string    `json:"source,omitempty"`
	Query      string    `json:"query,omitempty"`
	NextAction string    `json:"next_action,omitempty"`
	MatchScore float64   `json:"match_score,omitempty"`
	Job        *Job      `json:"job,omitempty"`
}

type Job struct {
	Provider     string                  `json:"provider,omitempty"`
	Board        string                  `json:"board,omitempty"`
	ExternalID   string                  `json:"external_id,omitempty"`
	Title        string                  `json:"title"`
	Company      string                  `json:"company,omitempty"`
	Department   string                  `json:"department,omitempty"`
	Location     string                  `json:"location,omitempty"`
	Remote       bool                    `json:"remote,omitempty"`
	URL          string                  `json:"url,omitempty"`
	ApplyURL     string                  `json:"apply_url,omitempty"`
	Description  string                  `json:"description,omitempty"`
	PublishedAt  string                  `json:"published_at,omitempty"`
	Compensation *jobsearch.Compensation `json:"compensation,omitempty"`
	Opportunity  jobsearch.Opportunity   `json:"opportunity,omitempty"`
	Eligibility  *eligibility.Result     `json:"eligibility,omitempty"`
	JDText       string                  `json:"jd_text,omitempty"`
	Fingerprint  string                  `json:"fingerprint,omitempty"`
	SourceURL    string                  `json:"source_url,omitempty"`
}

type Item struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Source     string    `json:"source,omitempty"`
	Query      string    `json:"query,omitempty"`
	NextAction string    `json:"next_action,omitempty"`
	MatchScore float64   `json:"match_score,omitempty"`
	Job        Job       `json:"job"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	SeenCount  int       `json:"seen_count,omitempty"`
	Events     []Event   `json:"events,omitempty"`
}

type Ledger struct{ Path string }

func FromSearchJob(job jobsearch.Job) Job {
	return Job{
		Provider: job.Provider, Board: job.Board, ExternalID: job.ID,
		Title: job.Title, Company: job.Company, Department: job.Department,
		Location: job.Location, Remote: job.Remote, URL: job.URL, ApplyURL: job.ApplyURL,
		Description: job.Description, PublishedAt: job.PublishedAt, Compensation: job.Compensation, Opportunity: job.Opportunity,
		Eligibility: job.Eligibility,
		JDText:      jdText(job.Title, job.Company, job.Description),
		Fingerprint: fingerprint(job), SourceURL: firstNonEmpty(job.ApplyURL, job.URL),
	}
}

func NewID(job Job) string {
	base := Slugify(firstNonEmpty(job.Company, job.Board, job.Provider, "job")) + "--" + Slugify(firstNonEmpty(job.Title, job.ExternalID, "role"))
	sum := sha1.Sum([]byte(dedupeKey(job)))
	return strings.Trim(base, "-") + "-" + hex.EncodeToString(sum[:])[:8]
}

func NextAction(score float64) string {
	switch {
	case score >= 75:
		return "apply-plan"
	case score >= 50:
		return "review-gaps"
	default:
		return "skip-or-research"
	}
}

// NextActionWithEligibility keeps hard constraints independent from the fit
// score. An ineligible role is never promoted merely because its keywords
// match, while an ambiguous role remains visible for a human decision.
func NextActionWithEligibility(score float64, assessment *eligibility.Result) string {
	if assessment != nil {
		switch assessment.Status {
		case eligibility.Ineligible:
			return "skip-ineligible"
		case eligibility.Review:
			return "review-eligibility"
		}
	}
	return NextAction(score)
}

func (l *Ledger) Append(e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	f, err := privatefs.OpenAppend(l.Path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return fmt.Errorf("%w; close: %v", err, closeErr)
		}
		return err
	}
	buf := append(raw, '\n')
	n, writeErr := f.Write(buf)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if n != len(buf) {
		return io.ErrShortWrite
	}
	return closeErr
}

func (l *Ledger) Events() ([]Event, error) {
	f, err := os.Open(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", l.Path, lineNo, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (l *Ledger) Replay() ([]*Item, error) {
	events, err := l.Events()
	if err != nil {
		return nil, err
	}
	byID := map[string]*Item{}
	var order []string
	for _, e := range events {
		item, ok := byID[e.ID]
		if !ok {
			item = &Item{ID: e.ID, Status: "new", CreatedAt: e.TS}
			byID[e.ID] = item
			order = append(order, e.ID)
		}
		item.UpdatedAt = e.TS
		item.Events = append(item.Events, e)
		if e.Type == EvSaved || e.Type == EvSeen {
			item.LastSeenAt = e.TS
			item.SeenCount++
		}
		if e.Job != nil {
			item.Job = *e.Job
		}
		if e.Status != "" {
			item.Status = e.Status
		}
		if e.Source != "" {
			item.Source = e.Source
		}
		if e.Query != "" {
			item.Query = e.Query
		}
		if e.NextAction != "" {
			item.NextAction = e.NextAction
		}
		if e.MatchScore != 0 {
			item.MatchScore = e.MatchScore
		}
	}
	items := make([]*Item, 0, len(order))
	for _, id := range order {
		items = append(items, byID[id])
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Status != items[j].Status {
			return statusRank(items[i].Status) < statusRank(items[j].Status)
		}
		if items[i].MatchScore != items[j].MatchScore {
			return items[i].MatchScore > items[j].MatchScore
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func Find(items []*Item, id string) (*Item, error) {
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	var hits []*Item
	for _, item := range items {
		if strings.Contains(item.ID, id) {
			hits = append(hits, item)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return nil, fmt.Errorf("no inbox item matches %q", id)
	default:
		var ids []string
		for _, hit := range hits {
			ids = append(ids, hit.ID)
		}
		return nil, fmt.Errorf("ambiguous id %q: %s", id, strings.Join(ids, ", "))
	}
}

func Has(items []*Item, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func fingerprint(job jobsearch.Job) string {
	sum := sha1.Sum([]byte(strings.Join([]string{job.Provider, job.Board, job.ID, job.Title, job.Company, job.URL, job.ApplyURL, job.Description}, "\x00")))
	return hex.EncodeToString(sum[:])[:12]
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

func statusRank(status string) int {
	for i, st := range Statuses {
		if st == status {
			return i
		}
	}
	return len(Statuses)
}

func dedupeKey(job Job) string {
	return strings.ToLower(strings.Join([]string{
		firstNonEmpty(job.ApplyURL, job.URL),
		job.Provider, job.Board, job.ExternalID, job.Company, job.Title,
	}, "|"))
}

func jdText(title, company, desc string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString("Title: ")
		b.WriteString(title)
		b.WriteString("\n")
	}
	if company != "" {
		b.WriteString("Company: ")
		b.WriteString(company)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(desc)
	return strings.TrimSpace(b.String()) + "\n"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
