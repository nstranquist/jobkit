// Package track is the application tracker: an append-only JSONL event
// ledger (~/.jobkit/applications.jsonl) replayed into per-application state.
// Events are never rewritten — history is the data model.
package track

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
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
	EvTagged    = "tagged"
	EvContact   = "contact"
	EvInterview = "interview"
	EvFollowUp  = "followup_sent"
)

// Event is one immutable ledger line.
type Event struct {
	TS      time.Time         `json:"ts"`
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Status  string            `json:"status,omitempty"`
	Company string            `json:"company,omitempty"`
	Role    string            `json:"role,omitempty"`
	URL     string            `json:"url,omitempty"`
	Note    string            `json:"note,omitempty"`
	Tags    map[string]string `json:"tags,omitempty"`
}

// Canonical tag keys for funnel analysis. Any key is allowed; these three
// get first-class breakdowns in stats.
const (
	TagResumeVersion = "resume_version"
	TagLane          = "lane"
	TagSource        = "source"
)

// Application is replayed state for one tracked job.
type Application struct {
	ID        string            `json:"id"`
	Company   string            `json:"company"`
	Role      string            `json:"role"`
	URL       string            `json:"url,omitempty"`
	Status    string            `json:"status"`
	Tags      map[string]string `json:"tags,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	AppliedAt time.Time         `json:"applied_at,omitempty"`
	Events    []Event           `json:"events,omitempty"`
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
		if len(e.Tags) > 0 {
			if app.Tags == nil {
				app.Tags = map[string]string{}
			}
			for k, v := range e.Tags {
				app.Tags[k] = v
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
	// ByTag holds per-tag-value conversion rows, keyed tag key -> tag value.
	// Only tag keys that appear on at least one application are present.
	ByTag map[string]map[string]*TagStats `json:"by_tag,omitempty"`
}

// TagStats is the conversion row for one tag value (e.g. lane=ai-platform).
type TagStats struct {
	Total        int     `json:"total"`
	Applied      int     `json:"applied"`
	Responded    int     `json:"responded"`
	Interviews   int     `json:"interviews"` // reached interview or beyond
	ResponseRate float64 `json:"response_rate"`
}

// BuildStats computes funnel stats as of now.
func BuildStats(apps []*Application, now time.Time) *Stats {
	s := &Stats{ByStatus: map[string]int{}, ByTag: map[string]map[string]*TagStats{}}
	progressed := map[string]bool{"screening": true, "interview": true, "offer": true, "accepted": true, "rejected": true}
	interviewed := map[string]bool{"interview": true, "offer": true, "accepted": true}
	for _, a := range apps {
		s.Total++
		s.ByStatus[a.Status]++
		if !TerminalStatuses[a.Status] {
			s.Active++
		}
		applied := !a.AppliedAt.IsZero()
		if applied {
			s.Applied++
			if now.Sub(a.AppliedAt) <= 7*24*time.Hour {
				s.AppliedLast7++
			}
			if progressed[a.Status] {
				s.Responded++
			}
		}
		for k, v := range a.Tags {
			byVal := s.ByTag[k]
			if byVal == nil {
				byVal = map[string]*TagStats{}
				s.ByTag[k] = byVal
			}
			row := byVal[v]
			if row == nil {
				row = &TagStats{}
				byVal[v] = row
			}
			row.Total++
			if applied {
				row.Applied++
				if progressed[a.Status] {
					row.Responded++
				}
				if interviewed[a.Status] {
					row.Interviews++
				}
			}
		}
	}
	if s.Applied > 0 {
		s.ResponseRate = float64(s.Responded) / float64(s.Applied)
	}
	for _, byVal := range s.ByTag {
		for _, row := range byVal {
			if row.Applied > 0 {
				row.ResponseRate = float64(row.Responded) / float64(row.Applied)
			}
		}
	}
	if len(s.ByTag) == 0 {
		s.ByTag = nil
	}
	return s
}

// ParseTagSpec parses "k=v" or "k1=v1,k2=v2" into a tag map. Keys are
// lowercased and hyphens normalized to underscores so --tag Lane=x and
// --lane x land on the same key.
func ParseTagSpec(spec string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.Index(part, "=")
		if eq <= 0 || eq == len(part)-1 {
			return nil, fmt.Errorf("tag %q must be key=value", part)
		}
		key := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(part[:eq])), "-", "_")
		out[key] = strings.TrimSpace(part[eq+1:])
	}
	return out, nil
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
