// LinkedIn connections-export ingestion. LinkedIn's "Get a copy of your
// data" archive contains Connections.csv with a short "Notes:" preamble
// before the real header row; ParseLinkedInCSV tolerates both shapes.
package contacts

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// ImportRow is one parsed connection.
type ImportRow struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Email   string `json:"email,omitempty"`
	Company string `json:"company,omitempty"`
	Role    string `json:"role,omitempty"`
}

// ImportSummary reports what a bulk import did.
type ImportSummary struct {
	Parsed   int      `json:"parsed"`
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"` // duplicates of existing ledger contacts
	IDs      []string `json:"ids,omitempty"`
}

// linkedinHeader maps the columns we care about to canonical names.
var linkedinHeader = map[string]string{
	"first name":    "first",
	"last name":     "last",
	"url":           "url",
	"email address": "email",
	"company":       "company",
	"position":      "role",
}

// ParseLinkedInCSV reads a LinkedIn Connections.csv export. It skips any
// preamble lines before the header row and ignores columns it doesn't know.
func ParseLinkedInCSV(r io.Reader) ([]ImportRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // preamble rows have arbitrary widths
	var colIndex map[string]int
	var rows []ImportRow
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if colIndex == nil {
			colIndex = matchHeader(record)
			continue // header row itself (or preamble) is never data
		}
		get := func(col string) string {
			i, ok := colIndex[col]
			if !ok || i >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[i])
		}
		name := strings.TrimSpace(get("first") + " " + get("last"))
		if name == "" {
			continue
		}
		rows = append(rows, ImportRow{
			Name:    name,
			URL:     get("url"),
			Email:   get("email"),
			Company: get("company"),
			Role:    get("role"),
		})
	}
	if colIndex == nil {
		return nil, fmt.Errorf("no LinkedIn header row found (expected columns like %q)", "First Name,Last Name,URL,...")
	}
	return rows, nil
}

// matchHeader returns the column index map when record looks like the
// LinkedIn header row (needs at least First Name + Last Name), nil otherwise.
func matchHeader(record []string) map[string]int {
	idx := map[string]int{}
	for i, cell := range record {
		if canonical, ok := linkedinHeader[strings.ToLower(strings.TrimSpace(cell))]; ok {
			idx[canonical] = i
		}
	}
	if _, ok := idx["first"]; !ok {
		return nil
	}
	if _, ok := idx["last"]; !ok {
		return nil
	}
	return idx
}

// Import appends created events for rows not already in the ledger.
// Duplicate detection: same slugified name+company as an existing item, or
// same non-empty profile URL. Existing contacts are never modified.
func Import(l *Ledger, rows []ImportRow, source string) (*ImportSummary, error) {
	items, err := l.Replay()
	if err != nil {
		return nil, err
	}
	seenID := map[string]bool{}
	seenURL := map[string]bool{}
	takenIDs := map[string]bool{}
	for _, item := range items {
		seenID[Slugify(strings.TrimSpace(item.Company+" "+item.Name))] = true
		takenIDs[item.ID] = true
		if item.URL != "" {
			seenURL[strings.TrimRight(item.URL, "/")] = true
		}
	}
	newID := func(name, company string) string {
		base := Slugify(strings.TrimSpace(strings.Join([]string{company, name}, " ")))
		if base == "" {
			base = "contact"
		}
		id := base
		for n := 2; takenIDs[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		takenIDs[id] = true
		return id
	}
	sum := &ImportSummary{Parsed: len(rows)}
	for _, row := range rows {
		key := Slugify(strings.TrimSpace(row.Company + " " + row.Name))
		url := strings.TrimRight(row.URL, "/")
		if seenID[key] || (url != "" && seenURL[url]) {
			sum.Skipped++
			continue
		}
		id := newID(row.Name, row.Company)
		ev := Event{
			ID: id, Type: EvCreated, Status: "lead",
			Name: row.Name, Company: row.Company, Role: row.Role,
			URL: row.URL, Email: row.Email, Source: source, Channel: "linkedin",
		}
		if err := l.Append(ev); err != nil {
			return nil, err
		}
		seenID[key] = true
		if url != "" {
			seenURL[url] = true
		}
		sum.Imported++
		sum.IDs = append(sum.IDs, id)
	}
	return sum, nil
}
