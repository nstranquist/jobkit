package profile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/jd"
	"gopkg.in/yaml.v3"
)

type BootstrapOptions struct {
	Source string
}

type BootstrapResult struct {
	Profile      *Profile `json:"profile"`
	Source       string   `json:"source"`
	ExtractedLen int      `json:"extracted_bytes"`
	Warnings     []string `json:"warnings,omitempty"`
}

func Bootstrap(opts BootstrapOptions) (*BootstrapResult, error) {
	text, warnings, err := ExtractText(opts.Source)
	if err != nil {
		return nil, err
	}
	p := FromResumeText(text)
	return &BootstrapResult{Profile: p, Source: opts.Source, ExtractedLen: len(text), Warnings: warnings}, nil
}

func ExtractText(path string) (string, []string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, fmt.Errorf("source is required")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".markdown":
		raw, err := os.ReadFile(path)
		return string(raw), nil, err
	case ".pdf":
		bin, err := exec.LookPath("pdftotext")
		if err != nil {
			return "", nil, fmt.Errorf("pdftotext is required for PDF bootstrap; export text first or install poppler")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-layout", path, "-")
		raw, err := cmd.Output()
		if ctx.Err() == context.DeadlineExceeded {
			return "", nil, fmt.Errorf("pdftotext %s timed out after 30s", path)
		}
		if err != nil {
			return "", nil, fmt.Errorf("pdftotext %s: %w", path, err)
		}
		return string(raw), nil, nil
	default:
		raw, err := os.ReadFile(path)
		return string(raw), []string{"unknown extension; treated source as plain text"}, err
	}
}

func Write(path string, p *Profile, overwrite bool) error {
	if !overwrite {
		exists, err := pathExists(path)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%s already exists", path)
		}
	}
	raw, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func pathExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info != nil, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe    = regexp.MustCompile(`(?:\+?1[\s.-]?)?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}`)
	urlRe      = regexp.MustCompile(`(?i)\b(?:https?://)?(?:www\.)?(?:github\.com/[A-Za-z0-9_.-]+|linkedin\.com/in/[A-Za-z0-9_.-]+|[A-Za-z0-9_.-]+\.[A-Za-z]{2,}(?:/[^\s]*)?)`)
	dateLineRe = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4}\s*-\s*(Present|Current|Now|(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{4})`)
)

func FromResumeText(text string) *Profile {
	lines := cleanLines(text)
	p := &Profile{
		Name:     inferName(lines),
		Headline: "Software Engineer",
		Email:    firstMatch(emailRe, text),
		Phone:    firstMatch(phoneRe, text),
		Location: inferLocation(lines),
		Links:    inferLinks(text),
		Summary:  inferSummary(lines),
		Skills:   inferSkills(text),
	}
	p.Experience = inferExperience(lines)
	p.Projects = inferProjects(lines)
	p.Education = inferEducation(lines)
	if p.Name == "" {
		p.Name = "Your Name"
	}
	if p.Summary == "" {
		p.Summary = "Software engineer. Review and refine this profile after bootstrap."
	}
	return p
}

func cleanLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var lines []string
	for _, line := range raw {
		line = strings.TrimSpace(strings.Trim(line, "\f"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func inferName(lines []string) string {
	for _, line := range lines {
		if emailRe.MatchString(line) || phoneRe.MatchString(line) || strings.Contains(line, "|") {
			continue
		}
		if len(line) <= 40 && strings.ToUpper(line) == line && strings.Contains(line, " ") {
			return titleWords(strings.ToLower(line))
		}
	}
	if len(lines) > 0 && len(lines[0]) <= 60 {
		return titleWords(strings.ToLower(lines[0]))
	}
	return ""
}

func inferLocation(lines []string) string {
	for _, line := range lines[:min(len(lines), 6)] {
		if strings.Contains(line, "St. Louis") || strings.Contains(line, "Saint Louis") {
			return "St. Louis, MO"
		}
	}
	return ""
}

func inferLinks(text string) []Link {
	seen := map[string]bool{}
	var links []Link
	scanText := emailRe.ReplaceAllString(text, "")
	for _, raw := range urlRe.FindAllString(scanText, -1) {
		u := strings.Trim(raw, ".,)")
		label := "Website"
		if strings.Contains(strings.ToLower(u), "github.com") {
			label = "GitHub"
		} else if strings.Contains(strings.ToLower(u), "linkedin.com") {
			label = "LinkedIn"
		}
		if !strings.HasPrefix(strings.ToLower(u), "http") {
			u = "https://" + u
		}
		key := strings.ToLower(u)
		if !seen[key] {
			seen[key] = true
			links = append(links, Link{Label: label, URL: u})
		}
	}
	return links
}

func inferSummary(lines []string) string {
	start := indexFold(lines, "OBJECTIVE")
	if start < 0 {
		return ""
	}
	var parts []string
	for _, line := range lines[start+1:] {
		if isSection(line) {
			break
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, " ")
}

func inferSkills(text string) []Skill {
	lower := strings.ToLower(text)
	type hit struct {
		name  string
		count int
	}
	var hits []hit
	for _, e := range jd.Lexicon() {
		count := 0
		for _, alias := range e.Aliases {
			if containsTerm(lower, strings.ToLower(alias)) {
				count++
			}
		}
		if count > 0 {
			hits = append(hits, hit{name: e.Canonical, count: count})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].count != hits[j].count {
			return hits[i].count > hits[j].count
		}
		return hits[i].name < hits[j].name
	})
	limit := min(len(hits), 24)
	out := make([]Skill, 0, limit)
	for _, h := range hits[:limit] {
		out = append(out, Skill{Name: h.name, Level: "proficient"})
	}
	return out
}

func inferExperience(lines []string) []Experience {
	start := indexFold(lines, "EXPERIENCE")
	if start < 0 {
		return nil
	}
	end := len(lines)
	for _, marker := range []string{"SOFTWARE PROJECTS", "PROJECTS", "EDUCATION"} {
		if i := indexFold(lines[start+1:], marker); i >= 0 {
			end = min(end, start+1+i)
		}
	}
	var out []Experience
	for i := start + 1; i < end; i++ {
		line := lines[i]
		loc := dateLineRe.FindStringIndex(line)
		if loc == nil {
			continue
		}
		company := strings.TrimSpace(line[:loc[0]])
		role := ""
		if i+1 < end && !dateLineRe.MatchString(lines[i+1]) && !isSection(lines[i+1]) {
			role = lines[i+1]
			i++
		}
		dr := dateLineRe.FindString(line)
		startDate, endDate := parseDateRange(dr)
		var bullets []Bullet
		for i+1 < end {
			next := lines[i+1]
			if dateLineRe.MatchString(next) || isSection(next) {
				break
			}
			if isResumeBullet(next) || looksLikeSentence(next) {
				text := strings.TrimLeft(strings.TrimSpace(next), "-• ")
				if len(bullets) > 0 && strings.HasSuffix(bullets[len(bullets)-1].Text, ",") {
					bullets[len(bullets)-1].Text += " " + text
					bullets[len(bullets)-1].Tags = tagsFor(bullets[len(bullets)-1].Text)
					i++
					continue
				}
				bullets = append(bullets, Bullet{Text: text, Tags: tagsFor(text)})
			}
			i++
		}
		out = append(out, Experience{Company: company, Role: role, Start: startDate, End: endDate, Bullets: bullets})
	}
	return out
}

func inferProjects(lines []string) []Project {
	start := firstIndexFold(lines, "SOFTWARE PROJECTS", "PROJECTS")
	if start < 0 {
		return nil
	}
	end := len(lines)
	if i := indexFold(lines[start+1:], "EDUCATION"); i >= 0 {
		end = start + 1 + i
	}
	var out []Project
	var cur *Project
	for _, line := range lines[start+1 : end] {
		if isResumeBullet(line) {
			if cur != nil {
				text := strings.TrimLeft(line, "-• ")
				if len(cur.Bullets) > 0 && strings.HasSuffix(cur.Bullets[len(cur.Bullets)-1].Text, ",") {
					cur.Bullets[len(cur.Bullets)-1].Text += " " + text
					cur.Bullets[len(cur.Bullets)-1].Tags = tagsFor(cur.Bullets[len(cur.Bullets)-1].Text)
					continue
				}
				cur.Bullets = append(cur.Bullets, Bullet{Text: text, Tags: tagsFor(text)})
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "utilized:") {
			if cur != nil {
				cur.Bullets = append(cur.Bullets, Bullet{Text: line, Tags: tagsFor(line)})
			}
			continue
		}
		if cur != nil && cur.Description == "" && looksLikeSentence(line) && !looksLikeProjectHeading(line) {
			cur.Description = line
			continue
		}
		name, url := splitProjectLine(line)
		out = append(out, Project{Name: name, URL: url})
		cur = &out[len(out)-1]
	}
	return out
}

func inferEducation(lines []string) []Education {
	start := indexFold(lines, "EDUCATION")
	if start < 0 {
		return nil
	}
	var out []Education
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "University") || strings.Contains(line, "College") || strings.Contains(line, "Saint Louis") {
			ed := Education{School: stripDateRange(line)}
			if i+1 < len(lines) && strings.Contains(strings.ToLower(lines[i+1]), "bachelor") {
				ed.Degree = lines[i+1]
				i++
			}
			out = append(out, ed)
		}
	}
	return out
}

func tagsFor(text string) []string {
	lower := strings.ToLower(text)
	seen := map[string]bool{}
	var tags []string
	for _, e := range jd.Lexicon() {
		for _, alias := range e.Aliases {
			if containsTerm(lower, strings.ToLower(alias)) && !seen[strings.ToLower(e.Canonical)] {
				seen[strings.ToLower(e.Canonical)] = true
				tags = append(tags, strings.ToLower(e.Canonical))
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func parseDateRange(raw string) (string, string) {
	parts := strings.SplitN(raw, "-", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parseMonthYear(parts[0]), parseMonthYear(parts[1])
}

func parseMonthYear(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.EqualFold(raw, "present") || strings.EqualFold(raw, "current") || strings.EqualFold(raw, "now") {
		return "present"
	}
	months := map[string]string{"january": "01", "february": "02", "march": "03", "april": "04", "may": "05", "june": "06", "july": "07", "august": "08", "september": "09", "october": "10", "november": "11", "december": "12"}
	fields := strings.Fields(strings.ToLower(raw))
	if len(fields) < 2 {
		return raw
	}
	if m, ok := months[fields[0]]; ok {
		return fields[1] + "-" + m
	}
	return raw
}

func splitProjectLine(line string) (string, string) {
	if strings.Contains(line, "(") && strings.Contains(line, ")") {
		name := strings.TrimSpace(line[:strings.Index(line, "(")])
		url := strings.TrimSpace(line[strings.Index(line, "(")+1 : strings.LastIndex(line, ")")])
		if looksLikeURL(url) {
			if !strings.HasPrefix(strings.ToLower(url), "http") {
				url = "https://" + url
			}
			return name, url
		}
		return strings.TrimSpace(line), ""
	}
	if raw := urlRe.FindString(line); raw != "" {
		name := strings.TrimSpace(strings.TrimSuffix(strings.Replace(line, raw, "", 1), ":"))
		url := strings.Trim(raw, ".,)")
		if !strings.HasPrefix(strings.ToLower(url), "http") {
			url = "https://" + url
		}
		return firstNonEmptyLocal(name, raw), url
	}
	return line, ""
}

func stripDateRange(line string) string {
	if loc := dateLineRe.FindStringIndex(line); loc != nil {
		return strings.TrimSpace(line[:loc[0]])
	}
	return line
}

func isSection(line string) bool {
	switch strings.ToUpper(strings.TrimSpace(line)) {
	case "OBJECTIVE", "EXPERIENCE", "SOFTWARE PROJECTS", "PROJECTS", "EDUCATION", "SKILLS":
		return true
	default:
		return false
	}
}

func isResumeBullet(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•")
}

func looksLikeSentence(line string) bool {
	return len(line) > 20 && (strings.Contains(line, " ") || strings.Contains(line, ","))
}

func looksLikeProjectHeading(line string) bool {
	return urlRe.MatchString(line) || strings.Contains(line, " - ")
}

func looksLikeURL(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.Contains(l, ".")
}

func containsTerm(text, term string) bool {
	if term == "" {
		return false
	}
	start := 0
	for {
		i := strings.Index(text[start:], term)
		if i < 0 {
			return false
		}
		pos := start + i
		end := pos + len(term)
		beforeOK := pos == 0 || !isTermChar(text[pos-1])
		afterOK := end >= len(text) || !isTermChar(text[end])
		if beforeOK && afterOK {
			return true
		}
		start = end
	}
}

func isTermChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '#'
}

func indexFold(lines []string, needle string) int {
	for i, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), needle) {
			return i
		}
	}
	return -1
}

func firstIndexFold(lines []string, needles ...string) int {
	for _, needle := range needles {
		if i := indexFold(lines, needle); i >= 0 {
			return i
		}
	}
	return -1
}

func firstMatch(re *regexp.Regexp, text string) string {
	return re.FindString(text)
}

func titleWords(s string) string {
	var b bytes.Buffer
	for i, word := range strings.Fields(s) {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	return b.String()
}

func firstNonEmptyLocal(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
