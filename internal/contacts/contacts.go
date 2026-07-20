// Package contacts stores the relationship/referral CRM as an append-only
// JSONL ledger. Replay is the database so outreach history remains auditable.
package contacts

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

var Statuses = []string{
	"lead", "contacted", "replied", "referral-requested", "referral-offered", "referred", "closed",
}

func ValidStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

const (
	EvCreated  = "created"
	EvTouch    = "touch"
	EvNote     = "note"
	EvReferral = "referral"
)

type Event struct {
	TS      time.Time `json:"ts"`
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Status  string    `json:"status,omitempty"`
	Name    string    `json:"name,omitempty"`
	Company string    `json:"company,omitempty"`
	Role    string    `json:"role,omitempty"`
	Channel string    `json:"channel,omitempty"`
	URL     string    `json:"url,omitempty"`
	Email   string    `json:"email,omitempty"`
	Source  string    `json:"source,omitempty"`
	InboxID string    `json:"inbox_id,omitempty"`
	TrackID string    `json:"track_id,omitempty"`
	Note    string    `json:"note,omitempty"`
}

type Item struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Company     string    `json:"company,omitempty"`
	Role        string    `json:"role,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	URL         string    `json:"url,omitempty"`
	Email       string    `json:"email,omitempty"`
	Status      string    `json:"status"`
	Source      string    `json:"source,omitempty"`
	InboxID     string    `json:"inbox_id,omitempty"`
	TrackID     string    `json:"track_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastTouchAt time.Time `json:"last_touch_at,omitempty"`
	Events      []Event   `json:"events,omitempty"`
}

type Ledger struct{ Path string }

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
			item = &Item{ID: e.ID, Status: "lead", CreatedAt: e.TS}
			byID[e.ID] = item
			order = append(order, e.ID)
		}
		item.UpdatedAt = e.TS
		item.Events = append(item.Events, e)
		if e.Name != "" {
			item.Name = e.Name
		}
		if e.Company != "" {
			item.Company = e.Company
		}
		if e.Role != "" {
			item.Role = e.Role
		}
		if e.Channel != "" {
			item.Channel = e.Channel
		}
		if e.URL != "" {
			item.URL = e.URL
		}
		if e.Email != "" {
			item.Email = e.Email
		}
		if e.Source != "" {
			item.Source = e.Source
		}
		if e.InboxID != "" {
			item.InboxID = e.InboxID
		}
		if e.TrackID != "" {
			item.TrackID = e.TrackID
		}
		if e.Status != "" {
			item.Status = e.Status
		}
		if e.Type == EvTouch || e.Type == EvReferral || (e.Type == EvCreated && !e.TS.IsZero()) {
			item.LastTouchAt = e.TS
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
		return nil, fmt.Errorf("no contact matches %q", id)
	default:
		var ids []string
		for _, hit := range hits {
			ids = append(ids, hit.ID)
		}
		return nil, fmt.Errorf("ambiguous id %q: %s", id, strings.Join(ids, ", "))
	}
}

func NewID(items []*Item, name, company string) string {
	base := Slugify(strings.TrimSpace(strings.Join([]string{company, name}, " ")))
	if base == "" {
		base = "contact"
	}
	id := base
	n := 1
	for {
		taken := false
		for _, item := range items {
			if item.ID == id {
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

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = slugRe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

func statusRank(status string) int {
	for i, s := range Statuses {
		if s == status {
			return i
		}
	}
	return len(Statuses) + 1
}
