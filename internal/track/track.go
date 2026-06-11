// Package track is the application tracker: an append-only JSONL event
// ledger (~/.jobkit/applications.jsonl) replayed into per-application state.
// Events are never rewritten — history is the data model.
package track

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Status values, in funnel order. Terminal: rejected, withdrawn, ghosted.
var Statuses = []string{
	"discovered", "interested", "applied", "screening", "interview", "offer", "accepted",
	"rejected", "withdrawn", "ghosted",
}

// TerminalStatuses end an application's life.
var TerminalStatuses = map[string]bool{"accepted": true, "rejected": true, "withdrawn": true, "ghosted": true}

// ValidStatus reports whether s is a known status.
func ValidStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// EventType values.
const (
	EvCreated   = "created"
	EvStatus    = "status_changed"
	EvNote      = "note"
	EvContact   = "contact"
	EvInterview = "interview"
	EvFollowUp  = "followup_sent"
)

// Event is one immutable ledger line.
type Event struct {
	TS      time.Time `json:"ts"`
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Status  string    `json:"status,omitempty"`
	Company string    `json:"company,omitempty"`
	Role    string    `json:"role,omitempty"`
	URL     string    `json:"url,omitempty"`
	Note    string    `json:"note,omitempty"`
}

// Application is replayed state for one tracked job.
type Application struct {
	ID        string    `json:"id"`
	Company   string    `json:"company"`
	Role      string    `json:"role"`
	URL       string    `json:"url,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
	Events    []Event   `json:"events,omitempty"`
}

// Ledger reads/writes one JSONL file.
type Ledger struct{ Path string }

// Append writes one event line.
func (l *Ledger) Append(e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	return err
}

// Events reads every ledger line in order. Missing file = empty ledger.
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
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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

// Replay materializes applications from the ledger, sorted by UpdatedAt desc.
func (l *Ledger) Replay() ([]*Application, error) {
	events, err := l.Events()
	if err != nil {
		return nil, err
	}
	byID := map[string]*Application{}
	var order []string
	for _, e := range events {
		app, ok := byID[e.ID]
		if !ok {
			app = &Application{ID: e.ID, Status: "discovered", CreatedAt: e.TS}
			byID[e.ID] = app
			order = append(order, e.ID)
		}
		app.UpdatedAt = e.TS
		app.Events = append(app.Events, e)
		if e.Company != "" {
			app.Company = e.Company
		}
		if e.Role != "" {
			app.Role = e.Role
		}
		if e.URL != "" {
			app.URL = e.URL
		}
		if e.Status != "" {
			app.Status = e.Status
			if e.Status == "applied" && app.AppliedAt.IsZero() {
				app.AppliedAt = e.TS
			}
		}
	}
	apps := make([]*Application, 0, len(order))
	for _, id := range order {
		apps = append(apps, byID[id])
	}
	sort.SliceStable(apps, func(a, b int) bool { return apps[a].UpdatedAt.After(apps[b].UpdatedAt) })
	return apps, nil
}

// Find returns the application whose ID matches exactly, then by unique
// prefix/substring.
func Find(apps []*Application, id string) (*Application, error) {
	for _, a := range apps {
		if a.ID == id {
			return a, nil
		}
	}
	var hits []*Application
	for _, a := range apps {
		if strings.Contains(a.ID, id) {
			hits = append(hits, a)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return nil, fmt.Errorf("no application matches %q", id)
	default:
		var ids []string
		for _, h := range hits {
			ids = append(ids, h.ID)
		}
		return nil, fmt.Errorf("ambiguous id %q: %s", id, strings.Join(ids, ", "))
	}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify makes a ledger id segment.
func Slugify(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

// NewID builds company-role, suffixing -2, -3... on collision.
func NewID(apps []*Application, company, role string) string {
	base := Slugify(company) + "--" + Slugify(role)
	id := base
	n := 1
	for {
		taken := false
		for _, a := range apps {
			if a.ID == id {
				taken = true
				break
			}
		}
		if !taken {
			return id
		}
		n++
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// Stats summarizes the funnel.
type Stats struct {
	Total        int            `json:"total"`
	Active       int            `json:"active"`
	ByStatus     map[string]int `json:"by_status"`
	Applied      int            `json:"applied_ever"`
	Responded    int            `json:"responded"` // applied apps with any post-applied progress or rejection
	ResponseRate float64        `json:"response_rate"`
	AppliedLast7 int            `json:"applied_last_7d"`
}

// BuildStats computes funnel stats as of now.
func BuildStats(apps []*Application, now time.Time) *Stats {
	s := &Stats{ByStatus: map[string]int{}}
	progressed := map[string]bool{"screening": true, "interview": true, "offer": true, "accepted": true, "rejected": true}
	for _, a := range apps {
		s.Total++
		s.ByStatus[a.Status]++
		if !TerminalStatuses[a.Status] {
			s.Active++
		}
		if !a.AppliedAt.IsZero() {
			s.Applied++
			if now.Sub(a.AppliedAt) <= 7*24*time.Hour {
				s.AppliedLast7++
			}
			if progressed[a.Status] {
				s.Responded++
			}
		}
	}
	if s.Applied > 0 {
		s.ResponseRate = float64(s.Responded) / float64(s.Applied)
	}
	return s
}

// FollowUps lists applied-but-quiet applications older than days.
func FollowUps(apps []*Application, days int, now time.Time) []*Application {
	var out []*Application
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	for _, a := range apps {
		if a.Status == "applied" && a.UpdatedAt.Before(cutoff) {
			out = append(out, a)
		}
	}
	return out
}
