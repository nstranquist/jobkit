package resume

import (
	"fmt"
	"html"
	"strings"

	"github.com/nstranquist/jobkit/internal/profile"
)

// RenderMarkdown emits a clean Markdown resume.
func RenderMarkdown(d *Doc) string {
	p := d.Profile
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", p.Name)
	if p.Headline != "" {
		fmt.Fprintf(&b, "**%s**\n\n", p.Headline)
	}
	b.WriteString(contactLine(d, " · "))
	b.WriteString("\n\n")
	if p.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(p.Summary))
	}
	if len(d.SkillOrder) > 0 {
		b.WriteString("## Skills\n\n")
		b.WriteString(skillLine(d))
		b.WriteString("\n\n")
	}
	if len(p.Experience) > 0 {
		b.WriteString("## Experience\n\n")
		for _, e := range p.Experience {
			fmt.Fprintf(&b, "### %s — %s\n", e.Role, e.Company)
			fmt.Fprintf(&b, "*%s*\n\n", dateLoc(e.Start, e.End, e.Location))
			for _, bl := range d.Bullets[Key(e)] {
				fmt.Fprintf(&b, "- %s\n", bl.Text)
			}
			b.WriteString("\n")
		}
	}
	if len(p.Projects) > 0 {
		b.WriteString("## Projects\n\n")
		for _, pr := range p.Projects {
			name := pr.Name
			if pr.URL != "" {
				name = fmt.Sprintf("[%s](%s)", pr.Name, pr.URL)
			}
			fmt.Fprintf(&b, "### %s\n", name)
			if pr.Description != "" {
				fmt.Fprintf(&b, "%s\n", pr.Description)
			}
			for _, bl := range pr.Bullets {
				fmt.Fprintf(&b, "- %s\n", bl.Text)
			}
			b.WriteString("\n")
		}
	}
	if len(p.Education) > 0 {
		b.WriteString("## Education\n\n")
		for _, ed := range p.Education {
			fmt.Fprintf(&b, "- %s\n", educationLine(ed))
		}
		b.WriteString("\n")
	}
	if len(p.Certifications) > 0 {
		b.WriteString("## Certifications\n\n")
		for _, c := range p.Certifications {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	return b.String()
}

// RenderText emits ATS-safe plain text: no markup, simple headers, hyphen
// bullets, ASCII separators.
func RenderText(d *Doc) string {
	p := d.Profile
	var b strings.Builder
	b.WriteString(p.Name + "\n")
	if p.Headline != "" {
		b.WriteString(p.Headline + "\n")
	}
	b.WriteString(contactLine(d, " | "))
	b.WriteString("\n\n")
	section := func(title string) {
		b.WriteString(strings.ToUpper(title) + "\n" + strings.Repeat("-", len(title)) + "\n")
	}
	if p.Summary != "" {
		section("Summary")
		b.WriteString(strings.TrimSpace(p.Summary) + "\n\n")
	}
	if len(d.SkillOrder) > 0 {
		section("Skills")
		b.WriteString(skillLine(d) + "\n\n")
	}
	if len(p.Experience) > 0 {
		section("Experience")
		for _, e := range p.Experience {
			fmt.Fprintf(&b, "%s, %s (%s)\n", e.Role, e.Company, dateLoc(e.Start, e.End, e.Location))
			for _, bl := range d.Bullets[Key(e)] {
				fmt.Fprintf(&b, "- %s\n", bl.Text)
			}
			b.WriteString("\n")
		}
	}
	if len(p.Projects) > 0 {
		section("Projects")
		for _, pr := range p.Projects {
			line := pr.Name
			if pr.URL != "" {
				line += " (" + pr.URL + ")"
			}
			b.WriteString(line + "\n")
			if pr.Description != "" {
				b.WriteString(pr.Description + "\n")
			}
			for _, bl := range pr.Bullets {
				fmt.Fprintf(&b, "- %s\n", bl.Text)
			}
			b.WriteString("\n")
		}
	}
	if len(p.Education) > 0 {
		section("Education")
		for _, ed := range p.Education {
			b.WriteString(educationLine(ed) + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.Certifications) > 0 {
		section("Certifications")
		for _, c := range p.Certifications {
			b.WriteString("- " + c + "\n")
		}
	}
	return b.String()
}

// RenderHTML emits a single-file, print-ready resume (US Letter friendly,
// clean typography, no external assets).
func RenderHTML(d *Doc) string {
	p := d.Profile
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>%s — Resume</title>\n", esc(p.Name))
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<style>\n" + resumeCSS + "</style>\n</head>\n<body>\n<main>\n")

	fmt.Fprintf(&b, "<header>\n<h1>%s</h1>\n", esc(p.Name))
	if p.Headline != "" {
		fmt.Fprintf(&b, "<p class=\"headline\">%s</p>\n", esc(p.Headline))
	}
	var contacts []string
	if p.Email != "" {
		contacts = append(contacts, fmt.Sprintf(`<a href="mailto:%s">%s</a>`, esc(p.Email), esc(p.Email)))
	}
	if p.Phone != "" {
		contacts = append(contacts, esc(p.Phone))
	}
	if p.Location != "" {
		contacts = append(contacts, esc(p.Location))
	}
	for _, l := range p.Links {
		contacts = append(contacts, fmt.Sprintf(`<a href="%s">%s</a>`, esc(l.URL), esc(l.Label)))
	}
	fmt.Fprintf(&b, "<p class=\"contact\">%s</p>\n</header>\n", strings.Join(contacts, " <span class=\"sep\">·</span> "))

	if p.Summary != "" {
		fmt.Fprintf(&b, "<section><h2>Summary</h2><p>%s</p></section>\n", esc(strings.TrimSpace(p.Summary)))
	}
	if len(d.SkillOrder) > 0 {
		b.WriteString("<section><h2>Skills</h2><p class=\"skills\">")
		var chips []string
		for _, s := range d.SkillOrder {
			chips = append(chips, fmt.Sprintf(`<span class="chip">%s</span>`, esc(s.Name)))
		}
		b.WriteString(strings.Join(chips, " "))
		b.WriteString("</p></section>\n")
	}
	if len(p.Experience) > 0 {
		b.WriteString("<section><h2>Experience</h2>\n")
		for _, e := range p.Experience {
			fmt.Fprintf(&b, "<div class=\"entry\"><div class=\"entry-head\"><h3>%s <span class=\"co\">— %s</span></h3><span class=\"dates\">%s</span></div>\n",
				esc(e.Role), esc(e.Company), esc(dateLoc(e.Start, e.End, e.Location)))
			b.WriteString("<ul>\n")
			for _, bl := range d.Bullets[Key(e)] {
				fmt.Fprintf(&b, "<li>%s</li>\n", esc(bl.Text))
			}
			b.WriteString("</ul></div>\n")
		}
		b.WriteString("</section>\n")
	}
	if len(p.Projects) > 0 {
		b.WriteString("<section><h2>Projects</h2>\n")
		for _, pr := range p.Projects {
			name := esc(pr.Name)
			if pr.URL != "" {
				name = fmt.Sprintf(`<a href="%s">%s</a>`, esc(pr.URL), esc(pr.Name))
			}
			fmt.Fprintf(&b, "<div class=\"entry\"><div class=\"entry-head\"><h3>%s</h3></div>\n", name)
			if pr.Description != "" {
				fmt.Fprintf(&b, "<p>%s</p>\n", esc(pr.Description))
			}
			if len(pr.Bullets) > 0 {
				b.WriteString("<ul>\n")
				for _, bl := range pr.Bullets {
					fmt.Fprintf(&b, "<li>%s</li>\n", esc(bl.Text))
				}
				b.WriteString("</ul>\n")
			}
			b.WriteString("</div>\n")
		}
		b.WriteString("</section>\n")
	}
	if len(p.Education) > 0 {
		b.WriteString("<section><h2>Education</h2><ul class=\"plain\">\n")
		for _, ed := range p.Education {
			fmt.Fprintf(&b, "<li>%s</li>\n", esc(educationLine(ed)))
		}
		b.WriteString("</ul></section>\n")
	}
	if len(p.Certifications) > 0 {
		b.WriteString("<section><h2>Certifications</h2><ul class=\"plain\">\n")
		for _, c := range p.Certifications {
			fmt.Fprintf(&b, "<li>%s</li>\n", esc(c))
		}
		b.WriteString("</ul></section>\n")
	}
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.String()
}

func contactLine(d *Doc, sep string) string {
	p := d.Profile
	var parts []string
	if p.Email != "" {
		parts = append(parts, p.Email)
	}
	if p.Phone != "" {
		parts = append(parts, p.Phone)
	}
	if p.Location != "" {
		parts = append(parts, p.Location)
	}
	for _, l := range p.Links {
		parts = append(parts, l.URL)
	}
	return strings.Join(parts, sep)
}

func skillLine(d *Doc) string {
	var names []string
	for _, s := range d.SkillOrder {
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}

func dateLoc(start, end, loc string) string {
	if end == "" {
		end = "present"
	}
	s := start + " – " + end
	if loc != "" {
		s += ", " + loc
	}
	return s
}

func educationLine(ed profile.Education) string {
	parts := []string{}
	if ed.Degree != "" {
		deg := ed.Degree
		if ed.Field != "" {
			deg += " " + ed.Field
		}
		parts = append(parts, deg)
	} else if ed.Field != "" {
		parts = append(parts, ed.Field)
	}
	parts = append(parts, ed.School)
	if ed.Year != "" {
		parts = append(parts, ed.Year)
	}
	return strings.Join(parts, ", ")
}

const resumeCSS = `
:root { --ink: #1a1d21; --muted: #5a6270; --line: #d9dde3; --accent: #1f5fbf; }
* { box-sizing: border-box; }
body { margin: 0; color: var(--ink); font: 15px/1.5 -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; background: #f3f4f6; }
main { max-width: 8.5in; margin: 2rem auto; background: #fff; padding: 0.9in 0.85in; box-shadow: 0 2px 14px rgba(0,0,0,.08); }
header h1 { margin: 0; font-size: 28px; letter-spacing: -0.02em; }
.headline { margin: 2px 0 0; color: var(--muted); font-size: 16px; }
.contact { margin: 6px 0 0; font-size: 13px; color: var(--muted); }
.contact a { color: var(--accent); text-decoration: none; }
.sep { color: var(--line); }
h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.12em; color: var(--accent); border-bottom: 1px solid var(--line); padding-bottom: 4px; margin: 22px 0 10px; }
h3 { font-size: 15px; margin: 0; }
.co { color: var(--muted); font-weight: 500; }
.entry { margin-bottom: 12px; }
.entry-head { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; }
.dates { color: var(--muted); font-size: 13px; white-space: nowrap; }
ul { margin: 6px 0 0; padding-left: 1.15em; }
li { margin: 3px 0; }
ul.plain { list-style: none; padding-left: 0; }
.chip { display: inline-block; border: 1px solid var(--line); border-radius: 999px; padding: 1px 9px; font-size: 12.5px; margin: 1px 0; }
.skills { line-height: 1.9; }
p { margin: 4px 0; }
@media print {
  body { background: #fff; }
  main { margin: 0; box-shadow: none; padding: 0.4in 0.5in; max-width: none; }
  .chip { border-color: #bbb; }
  a { color: inherit; }
}
@page { margin: 0.5in; }
`
