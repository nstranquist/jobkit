package contacts

import (
	"strings"
	"testing"
)

// FuzzParseLinkedInCSV drives the LinkedIn Connections.csv importer with
// arbitrary bytes. The parser tolerates a variable-width preamble, sniffs a
// header row, and projects unknown columns away, so it is a genuine external
// boundary. Invariants: no panic, and on success every returned row carries a
// non-empty Name (the only field the loop guarantees) — malformed CSV is
// expected to surface as an error, never a crash.
func FuzzParseLinkedInCSV(f *testing.F) {
	f.Add(linkedinCSVWithPreamble)
	f.Add("")
	f.Add("First Name,Last Name,URL,Email Address,Company,Position\nAda,Lovelace,,,OpenAI,Engineer\n")
	f.Add("just,some,random\ncsv,data,here\n")
	f.Add("First Name,Last Name\n,,\nGrace,Hopper\n")
	f.Add("Notes:\n\"quoted, preamble\"\nFirst Name,Last Name\nAlan,Turing\n")
	// Ragged quoting and embedded newlines exercise encoding/csv edge cases.
	f.Add("First Name,Last Name\n\"un\nterminated,x\n")
	f.Add("First Name,Last Name\nA,\"B\"\"C\",extra,fields\n")

	f.Fuzz(func(t *testing.T, data string) {
		rows, err := ParseLinkedInCSV(strings.NewReader(data))
		if err != nil {
			return // malformed CSV / missing header is a valid error path
		}
		for i, row := range rows {
			if strings.TrimSpace(row.Name) == "" {
				t.Fatalf("row %d has empty Name; parser must skip nameless rows: %+v", i, row)
			}
		}
	})
}
