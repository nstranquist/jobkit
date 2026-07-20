package track

import (
	"fmt"
	"strings"
	"time"
)

type Reminder struct {
	ID          string    `json:"id"`
	Company     string    `json:"company"`
	Role        string    `json:"role"`
	URL         string    `json:"url,omitempty"`
	Status      string    `json:"status"`
	DueAt       time.Time `json:"due_at"`
	LastTouched time.Time `json:"last_touched"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
}

func BuildReminders(apps []*Application, days int, now time.Time) []Reminder {
	if days <= 0 {
		days = 7
	}
	var out []Reminder
	for _, app := range FollowUps(apps, days, now) {
		due := app.UpdatedAt.Add(time.Duration(days) * 24 * time.Hour)
		if due.Before(now) {
			due = now
		}
		summary := fmt.Sprintf("Follow up: %s - %s", app.Company, app.Role)
		desc := fmt.Sprintf("Application %s has been applied and quiet since %s.", app.ID, app.UpdatedAt.Format("2006-01-02"))
		if app.URL != "" {
			desc += " " + app.URL
		}
		out = append(out, Reminder{
			ID:          app.ID,
			Company:     app.Company,
			Role:        app.Role,
			URL:         app.URL,
			Status:      app.Status,
			DueAt:       due,
			LastTouched: app.UpdatedAt,
			Summary:     summary,
			Description: desc,
		})
	}
	return out
}

func RenderRemindersText(reminders []Reminder) string {
	if len(reminders) == 0 {
		return "no follow-up reminders due\n"
	}
	var b strings.Builder
	for _, reminder := range reminders {
		fmt.Fprintf(&b, "%s  %s\n", reminder.DueAt.Local().Format("2006-01-02 15:04"), reminder.Summary)
		if reminder.URL != "" {
			fmt.Fprintf(&b, "  %s\n", reminder.URL)
		}
	}
	return b.String()
}

func RenderRemindersICS(reminders []Reminder, now time.Time) string {
	var b strings.Builder
	writeICSLine(&b, "BEGIN:VCALENDAR")
	writeICSLine(&b, "VERSION:2.0")
	writeICSLine(&b, "PRODID:-//jobkit//followups//EN")
	writeICSLine(&b, "CALSCALE:GREGORIAN")
	writeICSLine(&b, "METHOD:PUBLISH")
	for _, reminder := range reminders {
		start := reminder.DueAt.UTC()
		end := start.Add(15 * time.Minute)
		writeICSLine(&b, "BEGIN:VEVENT")
		writeICSLine(&b, "UID:"+escapeICS("jobkit-followup-"+reminder.ID+"@jobkit"))
		writeICSLine(&b, "DTSTAMP:"+formatICSTime(now.UTC()))
		writeICSLine(&b, "DTSTART:"+formatICSTime(start))
		writeICSLine(&b, "DTEND:"+formatICSTime(end))
		writeICSLine(&b, "SUMMARY:"+escapeICS(reminder.Summary))
		writeICSLine(&b, "DESCRIPTION:"+escapeICS(reminder.Description))
		if reminder.URL != "" {
			writeICSLine(&b, "URL:"+escapeICS(reminder.URL))
		}
		writeICSLine(&b, "END:VEVENT")
	}
	writeICSLine(&b, "END:VCALENDAR")
	return b.String()
}

func writeICSLine(b *strings.Builder, line string) {
	b.WriteString(line)
	b.WriteString("\r\n")
}

func formatICSTime(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func escapeICS(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		";", `\;`,
		",", `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
	)
	return replacer.Replace(s)
}
