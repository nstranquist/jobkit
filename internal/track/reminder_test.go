package track

import (
	"strings"
	"testing"
	"time"
)

func TestBuildRemindersAndICS(t *testing.T) {
	now := time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)
	apps := []*Application{
		{
			ID: "acme--backend", Company: "Acme", Role: "Backend Engineer", URL: "https://jobs.example/acme",
			Status: "applied", UpdatedAt: now.Add(-8 * 24 * time.Hour),
		},
		{
			ID: "newco--backend", Company: "NewCo", Role: "Backend Engineer",
			Status: "applied", UpdatedAt: now.Add(-2 * 24 * time.Hour),
		},
		{
			ID: "oldco--backend", Company: "OldCo", Role: "Backend Engineer",
			Status: "rejected", UpdatedAt: now.Add(-20 * 24 * time.Hour),
		},
	}
	reminders := BuildReminders(apps, 7, now)
	if len(reminders) != 1 {
		t.Fatalf("reminders = %#v, want one due reminder", reminders)
	}
	if reminders[0].ID != "acme--backend" || reminders[0].DueAt != now {
		t.Fatalf("reminder = %#v, want acme due now", reminders[0])
	}

	ics := RenderRemindersICS(reminders, now)
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"BEGIN:VEVENT\r\n",
		"UID:jobkit-followup-acme--backend@jobkit\r\n",
		"DTSTART:20260617T150000Z\r\n",
		"SUMMARY:Follow up: Acme - Backend Engineer\r\n",
		"URL:https://jobs.example/acme\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(ics, want) {
			t.Fatalf("ICS missing %q in:\n%s", want, ics)
		}
	}
}

func TestRenderRemindersTextEmpty(t *testing.T) {
	if got := RenderRemindersText(nil); got != "no follow-up reminders due\n" {
		t.Fatalf("empty text = %q", got)
	}
}
