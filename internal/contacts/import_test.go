package contacts

import (
	"path/filepath"
	"strings"
	"testing"
)

const linkedinCSVWithPreamble = `Notes:
"When exporting your connection data, you may notice that some email addresses are missing."

First Name,Last Name,URL,Email Address,Company,Position,Connected On
Ada,Lovelace,https://www.linkedin.com/in/ada,ada@example.com,OpenAI,Engineer,01 Jan 2024
Grace,Hopper,https://www.linkedin.com/in/grace,,Stripe,Staff Engineer,02 Feb 2023
,,,,,,
Alan,Turing,https://www.linkedin.com/in/alan,,Acme,Researcher,03 Mar 2022
`

func TestParseLinkedInCSVPreamble(t *testing.T) {
	rows, err := ParseLinkedInCSV(strings.NewReader(linkedinCSVWithPreamble))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (blank row skipped)", len(rows))
	}
	if rows[0].Name != "Ada Lovelace" || rows[0].Company != "OpenAI" || rows[0].Email != "ada@example.com" {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].Role != "Staff Engineer" {
		t.Fatalf("row1 role = %q", rows[1].Role)
	}
}

func TestParseLinkedInCSVNoHeader(t *testing.T) {
	_, err := ParseLinkedInCSV(strings.NewReader("just,some,random\ncsv,data,here\n"))
	if err == nil {
		t.Fatal("expected error for missing header")
	}
}

func TestImportDedupe(t *testing.T) {
	l := &Ledger{Path: filepath.Join(t.TempDir(), "contacts.jsonl")}
	// Pre-existing contact: same name+company as one CSV row.
	if err := l.Append(Event{ID: "openai-ada-lovelace", Type: EvCreated, Status: "contacted", Name: "Ada Lovelace", Company: "OpenAI"}); err != nil {
		t.Fatal(err)
	}
	rows, err := ParseLinkedInCSV(strings.NewReader(linkedinCSVWithPreamble))
	if err != nil {
		t.Fatal(err)
	}
	sum, err := Import(l, rows, "linkedin-export")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Parsed != 3 || sum.Imported != 2 || sum.Skipped != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	// Re-import must be a no-op (URL + name dedupe).
	sum2, err := Import(l, rows, "linkedin-export")
	if err != nil {
		t.Fatal(err)
	}
	if sum2.Imported != 0 || sum2.Skipped != 3 {
		t.Fatalf("re-import summary = %+v", sum2)
	}
	items, err := l.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("ledger items = %d, want 3", len(items))
	}
	// Existing contact status untouched.
	existing, err := Find(items, "openai-ada-lovelace")
	if err != nil || existing.Status != "contacted" {
		t.Fatalf("existing = %+v err=%v", existing, err)
	}
}
