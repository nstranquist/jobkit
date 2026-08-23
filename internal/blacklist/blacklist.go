// Package blacklist is a user-owned do-not-apply list.
// JobKit never writes companies into the file.
package blacklist

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nstranquist/jobkit/internal/strictyaml"
)

type List struct {
	SchemaVersion int      `yaml:"schema_version" json:"schema_version"`
	Companies     []string `yaml:"companies" json:"companies"`
}

type Hit struct {
	Company string `json:"company"`
	Matched string `json:"matched"`
}

// Load reads a user-authored list. A missing file is an empty list, not an error.
func Load(path string) (List, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return List{SchemaVersion: 1}, nil
		}
		return List{}, err
	}
	var list List
	if err := strictyaml.Unmarshal(payload, &list); err != nil {
		return List{}, fmt.Errorf("blacklist %s: %w", path, err)
	}
	if list.SchemaVersion == 0 {
		list.SchemaVersion = 1
	}
	return list, nil
}

func LoadFrom(path string) (List, error) { return Load(path) }

// Match reports a hit when company matches a list entry, ignoring case and punctuation.
func (l List) Match(company string) *Hit {
	want := normalizeName(company)
	if want == "" {
		return nil
	}
	for _, entry := range l.Companies {
		got := normalizeName(entry)
		if got == "" {
			continue
		}
		if want == got || strings.Contains(want, got) || strings.Contains(got, want) {
			return &Hit{Company: company, Matched: strings.TrimSpace(entry)}
		}
	}
	return nil
}

func LookupFile(path, company string) (*Hit, error) {
	list, err := Load(path)
	if err != nil {
		return nil, err
	}
	return list.Match(company), nil
}

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
