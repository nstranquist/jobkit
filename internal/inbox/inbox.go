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
	"math"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/blacklist"
	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/legitimacy"
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
	EvMerged   = "duplicate_merged"
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
	Legitimacy   *legitimacy.Result      `json:"legitimacy,omitempty"`
	Blacklist    *blacklist.Hit          `json:"blacklist,omitempty"`
	Liveness     string                  `json:"liveness,omitempty"`
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
	owner := Slugify(firstNonEmpty(job.Board, job.Company, job.Provider, "job"))
	if owner == "" {
		owner = "job"
	}
	posting := Slugify(canonicalExternalID(identityPart(job.Provider), job.ExternalID))
	if posting == "" {
		posting = "job"
	}
	sum := sha1.Sum([]byte(IdentityKey(job)))
	return owner + "--" + posting + "-" + hex.EncodeToString(sum[:])[:8]
}

// IdentityKey returns the strongest stable identity available for a posting.
// Public ATS records use provider + board + external ID, which does not change
// when an employer edits the title or posting URLs. Manual records fall back
// to a normalized posting URL, then company + title when no URL exists.
func IdentityKey(job Job) string {
	identity := postingIdentityFor(job)
	if identity.ATS != "" {
		return "ats|" + identity.ATS
	}
	if len(identity.URLs) > 0 {
		return "url|" + identity.URLs[0]
	}
	return "fallback|" + identity.Fallback
}

// SamePosting reports whether two records identify the same external job.
// Two complete ATS identities are authoritative. A URL match also supports
// legacy records that predate provider identity fields.
func SamePosting(a, b Job) bool {
	return samePostingIdentity(postingIdentityFor(a), postingIdentityFor(b))
}

// ResolveID preserves an existing ledger ID when a posting is seen again.
// This keeps old append-only histories addressable after the identity rule
// changes and after an employer edits a title. If duplicate legacy records
// exist, applied and active workflow records win over skipped history.
func ResolveID(items []*Item, job Job) string {
	want := postingIdentityFor(job)
	var best *Item
	for _, item := range items {
		if item == nil || !samePostingIdentity(want, postingIdentityFor(item.Job)) {
			continue
		}
		if best == nil || preferIdentityOwner(item, best) {
			best = item
		}
	}
	if best != nil {
		return best.ID
	}
	return NewID(job)
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

type postingIdentity struct {
	ATS      string
	URLs     []string
	Fallback string
}

func postingIdentityFor(job Job) postingIdentity {
	provider := identityPart(job.Provider)
	board := identityPart(job.Board)
	externalID := canonicalExternalID(provider, job.ExternalID)
	ats := ""
	if provider != "" && board != "" && externalID != "" {
		ats = strings.Join([]string{provider, board, externalID}, "|")
	}

	seenURLs := map[string]bool{}
	urls := make([]string, 0, 3)
	for _, raw := range []string{job.URL, job.ApplyURL, job.SourceURL} {
		normalized := normalizePostingURL(raw)
		if normalized == "" || seenURLs[normalized] {
			continue
		}
		seenURLs[normalized] = true
		urls = append(urls, normalized)
	}
	return postingIdentity{
		ATS:      ats,
		URLs:     urls,
		Fallback: identityPart(job.Company) + "|" + identityPart(job.Title),
	}
}

func samePostingIdentity(a, b postingIdentity) bool {
	if a.ATS != "" && b.ATS != "" {
		return a.ATS == b.ATS
	}
	for _, aURL := range a.URLs {
		for _, bURL := range b.URLs {
			if aURL == bURL {
				return true
			}
		}
	}
	return a.ATS == "" && b.ATS == "" && len(a.URLs) == 0 && len(b.URLs) == 0 && a.Fallback != "|" && a.Fallback == b.Fallback
}

func preferIdentityOwner(candidate, current *Item) bool {
	candidateRank := identityStatusRank(candidate.Status)
	currentRank := identityStatusRank(current.Status)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}
	if !candidate.LastSeenAt.Equal(current.LastSeenAt) {
		return candidate.LastSeenAt.After(current.LastSeenAt)
	}
	if !candidate.UpdatedAt.Equal(current.UpdatedAt) {
		return candidate.UpdatedAt.After(current.UpdatedAt)
	}
	return candidate.ID < current.ID
}

func identityStatusRank(status string) int {
	switch status {
	case "applied":
		return 0
	case "planned":
		return 1
	case "shortlisted":
		return 2
	case "new", "":
		return 3
	case "skipped":
		return 4
	case "archived":
		return 5
	default:
		return 6
	}
}

func identityPart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func canonicalExternalID(provider, value string) string {
	value = identityPart(value)
	if provider != "greenhouse" {
		return value
	}
	number, err := strconv.ParseFloat(value, 64)
	if err == nil && number >= 0 && number <= 9007199254740991 && number == math.Trunc(number) {
		return strconv.FormatInt(int64(number), 10)
	}
	return value
}

func normalizePostingURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return strings.TrimSuffix(raw, "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "gh_src" || lower == "lever-origin" || lower == "ref" || lower == "referrer" {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
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
