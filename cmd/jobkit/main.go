// jobkit — a multi-purpose job application & resume toolkit.
//
// Surfaces: master profile (init/profile), JD parsing (jd), gap scoring
// (match), tailored resumes (resume), cover letters (letter), and an
// append-only application tracker (track). Every verb supports --json with
// the {ok, data|error:{code,message,hint}} envelope.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/letter"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/prep"
	"github.com/nstranquist/jobkit/internal/profile"
	"github.com/nstranquist/jobkit/internal/resume"
	"github.com/nstranquist/jobkit/internal/telemetry"
	"github.com/nstranquist/jobkit/internal/track"
)

const version = "0.2.0"

// boolFlags take no value; everything else consumes one.
var boolFlags = map[string]bool{"json": true, "all": true, "full": true, "help": true}

type cli struct {
	args  []string          // positionals
	flags map[string]string // --k v / --k=v; bools stored as "true"
}

func (c *cli) bool(name string) bool   { return c.flags[name] == "true" }
func (c *cli) str(name string) string  { return c.flags[name] }
func (c *cli) int(name string, def int) (int, error) {
	v, ok := c.flags[name]
	if !ok {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, envelope.Newf(envelope.CodeInvalidArgs, "--%s must be an integer, got %q", name, v)
	}
	return n, nil
}

func parseArgs(raw []string) *cli {
	c := &cli{flags: map[string]string{}}
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if !strings.HasPrefix(a, "--") {
			c.args = append(c.args, a)
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if eq := strings.Index(name, "="); eq >= 0 {
			c.flags[name[:eq]] = name[eq+1:]
			continue
		}
		if boolFlags[name] || i+1 >= len(raw) || strings.HasPrefix(raw[i+1], "--") {
			c.flags[name] = "true"
			continue
		}
		c.flags[name] = raw[i+1]
		i++
	}
	return c
}

func main() {
	start := time.Now()
	c := parseArgs(os.Args[1:])
	cmd := "help"
	if len(c.args) > 0 {
		cmd = c.args[0]
	}
	err := dispatch(cmd, c)
	telemetry.Record(strings.Join(c.args, " "), start, err)
	if err != nil {
		if c.bool("json") {
			os.Exit(envelope.EmitError(err))
		}
		fmt.Fprintln(os.Stderr, "jobkit: "+err.Error())
		if e, ok := err.(*envelope.Err); ok && e.Hint != "" {
			fmt.Fprintln(os.Stderr, "hint: "+e.Hint)
		}
		os.Exit(envelope.ExitCode(err))
	}
}

func dispatch(cmd string, c *cli) error {
	if c.bool("help") {
		cmd = "help"
	}
	switch cmd {
	case "init":
		return cmdInit(c)
	case "profile":
		return cmdProfile(c)
	case "jd":
		return cmdJD(c)
	case "match":
		return cmdMatch(c)
	case "resume":
		return cmdResume(c)
	case "letter":
		return cmdLetter(c)
	case "prep":
		return cmdPrep(c)
	case "apply":
		return cmdApply(c)
	case "track":
		return cmdTrack(c)
	case "version":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"version": version})
		} else {
			fmt.Println("jobkit " + version)
		}
		return nil
	case "help":
		fmt.Print(usage)
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown command %q", cmd).WithHint("run `jobkit help`")
	}
}

// ---------- init / profile ----------

func cmdInit(c *cli) error {
	path, err := home.ProfilePath()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if err := profile.WriteTemplate(path); err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("edit the existing profile or move it aside first")
	}
	if c.bool("json") {
		envelope.EmitData(map[string]string{"profile": path})
	} else {
		fmt.Printf("Created starter profile at %s\nEdit it, then run `jobkit profile validate`.\n", path)
	}
	return nil
}

func loadProfile() (*profile.Profile, string, error) {
	path := os.Getenv("JOBKIT_PROFILE")
	if path == "" {
		var err error
		path, err = home.ProfilePath()
		if err != nil {
			return nil, "", envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	p, err := profile.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", envelope.Newf(envelope.CodeNotFound, "no profile at %s", path).WithHint("run `jobkit init` to create one")
		}
		return nil, "", envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	return p, path, nil
}

func cmdProfile(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	switch sub {
	case "path":
		path, err := home.ProfilePath()
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "show":
		p, path, err := loadProfile()
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(p)
			return nil
		}
		fmt.Printf("%s — %s\n%s\n", p.Name, p.Headline, path)
		fmt.Printf("skills: %d, experience: %d roles, projects: %d\n", len(p.Skills), len(p.Experience), len(p.Projects))
		return nil
	case "validate":
		_, path, err := loadProfile()
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"valid": true, "path": path})
		} else {
			fmt.Printf("OK: %s is valid\n", path)
		}
		return nil
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit profile <show|validate|path>")
	}
}

// ---------- jd / match ----------

func readInput(pathOrDash string) (string, error) {
	if pathOrDash == "-" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", envelope.New(envelope.CodeIOFailed, err.Error())
		}
		return string(raw), nil
	}
	if jd.IsURL(pathOrDash) {
		text, err := jd.Fetch(pathOrDash)
		if err != nil {
			return "", envelope.Newf(envelope.CodeIOFailed, "fetch %s: %v", pathOrDash, err).
				WithHint("JS-rendered boards may need copy/paste into a file; pipe it via `-`")
		}
		return text, nil
	}
	raw, err := os.ReadFile(pathOrDash)
	if err != nil {
		if os.IsNotExist(err) {
			return "", envelope.Newf(envelope.CodeNotFound, "no such file: %s", pathOrDash)
		}
		return "", envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return string(raw), nil
}

func parseJDArg(c *cli, idx int) (*jd.JD, error) {
	if len(c.args) <= idx {
		return nil, envelope.New(envelope.CodeInvalidArgs, "missing job-description file (or `-` for stdin)")
	}
	text, err := readInput(c.args[idx])
	if err != nil {
		return nil, err
	}
	return jd.Parse(text), nil
}

func cmdJD(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	if sub == "fetch" {
		if len(c.args) < 3 || !jd.IsURL(c.args[2]) {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit jd fetch <url> [--out path]")
		}
		text, err := readInput(c.args[2])
		if err != nil {
			return err
		}
		return writeArtifact(c, "jd", "txt", text, map[string]any{"url": c.args[2], "bytes": len(text)})
	}
	if sub != "parse" {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit jd <parse|fetch> <file|url|->")
	}
	j, err := parseJDArg(c, 2)
	if err != nil {
		return err
	}
	if c.bool("json") {
		envelope.EmitData(j)
		return nil
	}
	fmt.Printf("title:     %s\ncompany:   %s\nseniority: %s\n\nskills (%d):\n", orDash(j.Title), orDash(j.Company), j.Seniority, len(j.Skills))
	for _, s := range j.Skills {
		req := ""
		if s.Required {
			req = "  [required]"
		}
		fmt.Printf("  %-28s w=%.2f x%d%s\n", s.Name, s.Weight, s.Count, req)
	}
	return nil
}

func cmdMatch(c *cli) error {
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	j, err := parseJDArg(c, 1)
	if err != nil {
		return err
	}
	res := match.Score(p, j)
	if c.bool("json") {
		envelope.EmitData(res)
		return nil
	}
	fmt.Printf("match score: %.0f/100   (JD seniority: %s)\n\n", res.Score, res.Seniority)
	if len(res.Matched) > 0 {
		fmt.Printf("matched (%d):\n", len(res.Matched))
		for _, m := range res.Matched {
			extra := m.Evidence
			if m.Level != "" {
				extra += ", " + m.Level
			}
			if m.Years > 0 {
				extra += fmt.Sprintf(", %.0fy", m.Years)
			}
			req := ""
			if m.Required {
				req = " [required]"
			}
			fmt.Printf("  + %-26s w=%.2f (%s)%s\n", m.Name, m.Weight, extra, req)
		}
	}
	if len(res.Missing) > 0 {
		fmt.Printf("\nmissing (%d):\n", len(res.Missing))
		for _, m := range res.Missing {
			req := ""
			if m.Required {
				req = " [required]"
			}
			fmt.Printf("  - %-26s w=%.2f (%s)%s\n", m.Name, m.Weight, m.Category, req)
		}
	}
	fmt.Println("\nadvice:")
	for _, a := range res.Advice {
		fmt.Println("  • " + a)
	}
	return nil
}

// ---------- resume / letter ----------

func cmdResume(c *cli) error {
	if len(c.args) < 2 || c.args[1] != "build" {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit resume build [jd-file] [--format md|txt|html] [--out path] [--max-bullets N] [--full]")
	}
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	var j *jd.JD
	if len(c.args) > 2 && !c.bool("full") {
		if j, err = parseJDArg(c, 2); err != nil {
			return err
		}
	}
	maxBullets, err := c.int("max-bullets", 0)
	if err != nil {
		return err
	}
	if j != nil && maxBullets == 0 {
		maxBullets = 4 // tailored default: tight resumes
	}
	doc := resume.Build(p, j, resume.Options{MaxBulletsPerRole: maxBullets, TailorOrder: j != nil})

	format := c.str("format")
	if format == "" {
		format = "md"
	}
	var out string
	switch format {
	case "md", "markdown":
		out = resume.RenderMarkdown(doc)
		format = "md"
	case "txt", "text", "ats":
		out = resume.RenderText(doc)
		format = "txt"
	case "html":
		out = resume.RenderHTML(doc)
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown format %q (md|txt|html)", format)
	}
	return writeArtifact(c, "resume", format, out, map[string]any{
		"tailored": doc.Tailored, "target_title": doc.TargetTitle, "format": format,
	})
}

func cmdLetter(c *cli) error {
	if len(c.args) < 2 || c.args[1] != "build" {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit letter build <jd-file> [--company X] [--role Y] [--tone professional|warm|direct] [--manager NAME] [--out path]")
	}
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	j, err := parseJDArg(c, 2)
	if err != nil {
		return err
	}
	res := match.Score(p, j)
	text := letter.Build(p, j, res, letter.Options{
		Company: c.str("company"),
		Role:    c.str("role"),
		Tone:    c.str("tone"),
		Manager: c.str("manager"),
	})
	return writeArtifact(c, "letter", "txt", text, map[string]any{
		"company": firstNonEmpty(c.str("company"), j.Company),
		"role":    firstNonEmpty(c.str("role"), j.Title),
		"score":   res.Score,
	})
}

// writeArtifact prints to stdout, or writes to --out (default name under
// ~/.jobkit/out when --out is given without a value... no: --out always
// explicit; "auto" picks a dated name).
func writeArtifact(c *cli, kind, ext, content string, meta map[string]any) error {
	out := c.str("out")
	if out == "" {
		if c.bool("json") {
			meta["content"] = content
			envelope.EmitData(meta)
			return nil
		}
		fmt.Print(content)
		return nil
	}
	if out == "auto" {
		dir, err := home.OutDir()
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		out = filepath.Join(dir, fmt.Sprintf("%s-%s.%s", kind, time.Now().Format("2006-01-02-1504"), ext))
	}
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if c.bool("json") {
		meta["path"] = out
		envelope.EmitData(meta)
	} else {
		fmt.Printf("wrote %s\n", out)
	}
	return nil
}

func cmdPrep(c *cli) error {
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	j, err := parseJDArg(c, 1)
	if err != nil {
		return err
	}
	res := match.Score(p, j)
	sheet := prep.Build(p, j, res)
	return writeArtifact(c, "prep", "md", sheet, map[string]any{
		"role": j.Title, "company": j.Company, "score": res.Score,
	})
}

// cmdApply is the golden path: match + tailored resume + letter + prep, all
// written to one per-application artifact dir, and the application tracked.
func cmdApply(c *cli) error {
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	if len(c.args) < 2 {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit apply <jd-file|url|-> [--company X] [--role Y] [--tone T] [--format html|md|txt] [--status S]")
	}
	src := c.args[1]
	text, err := readInput(src)
	if err != nil {
		return err
	}
	j := jd.Parse(text)
	company := firstNonEmpty(c.str("company"), j.Company)
	role := firstNonEmpty(c.str("role"), j.Title)
	if company == "" || role == "" {
		return envelope.New(envelope.CodeInvalidArgs, "could not detect company/role from the JD").
			WithHint("pass --company and --role explicitly")
	}
	res := match.Score(p, j)

	l, err := openLedger()
	if err != nil {
		return err
	}
	apps, err := l.Replay()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	id := track.NewID(apps, company, role)

	outRoot, err := home.OutDir()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	dir := filepath.Join(outRoot, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	format := firstNonEmpty(c.str("format"), "html")
	doc := resume.Build(p, j, resume.Options{MaxBulletsPerRole: 4})
	var resumeOut, resumeName string
	switch format {
	case "md", "markdown":
		resumeName, resumeOut = "resume.md", resume.RenderMarkdown(doc)
	case "txt", "text", "ats":
		resumeName, resumeOut = "resume.txt", resume.RenderText(doc)
	case "html":
		resumeName, resumeOut = "resume.html", resume.RenderHTML(doc)
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown format %q (md|txt|html)", format)
	}
	letterOut := letter.Build(p, j, res, letter.Options{
		Company: company, Role: role, Tone: c.str("tone"), Manager: c.str("manager"),
	})
	prepOut := prep.Build(p, j, res)
	matchJSON, _ := json.MarshalIndent(res, "", "  ")

	files := map[string]string{
		resumeName:   resumeOut,
		"letter.txt": letterOut,
		"prep.md":    prepOut,
		"jd.txt":     text,
		"match.json": string(matchJSON) + "\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}

	status := firstNonEmpty(c.str("status"), "interested")
	if !track.ValidStatus(status) {
		return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
	}
	url := c.str("url")
	if url == "" && jd.IsURL(src) {
		url = src
	}
	ev := track.Event{
		ID: id, Type: track.EvCreated, Company: company, Role: role, URL: url, Status: status,
		Note: fmt.Sprintf("match %.0f/100 · artifacts: %s", res.Score, dir),
	}
	if err := l.Append(ev); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	if c.bool("json") {
		envelope.EmitData(map[string]any{
			"id": id, "score": res.Score, "dir": dir, "status": status,
			"files": []string{resumeName, "letter.txt", "prep.md", "jd.txt", "match.json"},
		})
		return nil
	}
	fmt.Printf("application package for %s — %s\n", company, role)
	fmt.Printf("  match score: %.0f/100\n  artifacts:   %s/\n", res.Score, dir)
	fmt.Printf("    %s, letter.txt, prep.md, jd.txt, match.json\n", resumeName)
	fmt.Printf("  tracked:     %s (%s)\n", id, status)
	fmt.Printf("next: review the letter, then `jobkit track set %s --status applied`\n", id)
	return nil
}

// ---------- track ----------

func openLedger() (*track.Ledger, error) {
	path, err := home.LedgerPath()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return &track.Ledger{Path: path}, nil
}

func cmdTrack(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	l, err := openLedger()
	if err != nil {
		return err
	}
	apps, err := l.Replay()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "add":
		if len(c.args) < 4 {
			return envelope.New(envelope.CodeInvalidArgs, `usage: jobkit track add <company> <role> [--url U] [--status S] [--note N]`)
		}
		company, role := c.args[2], c.args[3]
		status := c.str("status")
		if status == "" {
			status = "interested"
		}
		if !track.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
		}
		id := track.NewID(apps, company, role)
		ev := track.Event{ID: id, Type: track.EvCreated, Company: company, Role: role, URL: c.str("url"), Status: status, Note: c.str("note")}
		if err := l.Append(ev); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": id, "status": status})
		} else {
			fmt.Printf("tracked %s (%s)\n", id, status)
		}
		return nil

	case "list", "":
		filtered := apps
		if st := c.str("status"); st != "" {
			filtered = nil
			for _, a := range apps {
				if a.Status == st {
					filtered = append(filtered, a)
				}
			}
		} else if !c.bool("all") {
			filtered = nil
			for _, a := range apps {
				if !track.TerminalStatuses[a.Status] {
					filtered = append(filtered, a)
				}
			}
		}
		if filtered == nil {
			filtered = []*track.Application{}
		}
		if c.bool("json") {
			envelope.EmitData(filtered)
			return nil
		}
		if len(filtered) == 0 {
			fmt.Println("no applications tracked (jobkit track add <company> <role>)")
			return nil
		}
		for _, a := range filtered {
			fmt.Printf("%-40s %-11s %s — %s  (updated %s)\n", a.ID, a.Status, a.Company, a.Role, a.UpdatedAt.Local().Format("Jan 02"))
		}
		return nil

	case "show":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit track show <id>")
		}
		a, err := track.Find(apps, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(a)
			return nil
		}
		fmt.Printf("%s\n  %s — %s\n  status: %s\n", a.ID, a.Company, a.Role, a.Status)
		if a.URL != "" {
			fmt.Printf("  url: %s\n", a.URL)
		}
		fmt.Println("  history:")
		for _, e := range a.Events {
			line := e.Type
			if e.Status != "" {
				line += " → " + e.Status
			}
			if e.Note != "" {
				line += ": " + e.Note
			}
			fmt.Printf("    %s  %s\n", e.TS.Local().Format("2006-01-02 15:04"), line)
		}
		return nil

	case "set":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit track set <id> --status S [--note N]")
		}
		a, err := track.Find(apps, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		status := c.str("status")
		if status == "" {
			return envelope.New(envelope.CodeInvalidArgs, "--status is required").WithHint("statuses: " + strings.Join(track.Statuses, ", "))
		}
		if !track.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
		}
		if err := l.Append(track.Event{ID: a.ID, Type: track.EvStatus, Status: status, Note: c.str("note")}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": a.ID, "status": status})
		} else {
			fmt.Printf("%s → %s\n", a.ID, status)
		}
		return nil

	case "note":
		if len(c.args) < 4 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit track note <id> <text>")
		}
		a, err := track.Find(apps, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		note := strings.Join(c.args[3:], " ")
		if err := l.Append(track.Event{ID: a.ID, Type: track.EvNote, Note: note}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": a.ID, "note": note})
		} else {
			fmt.Printf("noted on %s\n", a.ID)
		}
		return nil

	case "board":
		if c.bool("json") {
			cols := map[string][]*track.Application{}
			for _, a := range apps {
				cols[a.Status] = append(cols[a.Status], a)
			}
			envelope.EmitData(cols)
			return nil
		}
		for _, st := range track.Statuses {
			var col []*track.Application
			for _, a := range apps {
				if a.Status == st {
					col = append(col, a)
				}
			}
			if len(col) == 0 {
				continue
			}
			fmt.Printf("%s (%d)\n", strings.ToUpper(st), len(col))
			for _, a := range col {
				fmt.Printf("  %s — %s\n", a.Company, a.Role)
			}
		}
		return nil

	case "stats":
		s := track.BuildStats(apps, time.Now())
		if c.bool("json") {
			envelope.EmitData(s)
			return nil
		}
		fmt.Printf("total: %d   active: %d   applied(ever): %d   applied(7d): %d   response rate: %.0f%%\n",
			s.Total, s.Active, s.Applied, s.AppliedLast7, s.ResponseRate*100)
		var keys []string
		for k := range s.ByStatus {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %-11s %d\n", k, s.ByStatus[k])
		}
		return nil

	case "followups":
		days, err := c.int("days", 7)
		if err != nil {
			return err
		}
		due := track.FollowUps(apps, days, time.Now())
		if due == nil {
			due = []*track.Application{}
		}
		if c.bool("json") {
			envelope.EmitData(due)
			return nil
		}
		if len(due) == 0 {
			fmt.Printf("nothing needs a follow-up (applied > %dd with no movement)\n", days)
			return nil
		}
		for _, a := range due {
			fmt.Printf("%-40s applied, quiet since %s — follow up\n", a.ID, a.UpdatedAt.Local().Format("Jan 02"))
		}
		return nil

	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown track subcommand %q", sub).WithHint("add|list|show|set|note|board|stats|followups")
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

const usage = `jobkit ` + version + ` — job application & resume toolkit

USAGE
  jobkit <command> [args] [--json]

PROFILE
  init                              create the starter profile (~/.jobkit/profile.yaml)
  profile show|validate|path        inspect the master profile

JOB DESCRIPTIONS (every <jd> accepts a file, a URL, or - for stdin)
  jd parse <jd>                     extract skills/seniority from a JD
  jd fetch <url> [--out PATH]       download a posting as clean text
  match <jd>                        score your profile against a JD + gap report

ARTIFACTS
  apply <jd> [--company X] [--role Y] [--tone T] [--format html|md|txt]
                                    golden path: resume + letter + prep sheet +
                                    match report into ~/.jobkit/out/<id>/ + tracked
  resume build [jd] [--format md|txt|html] [--out PATH|auto] [--max-bullets N] [--full]
                                    tailored resume (no JD or --full = complete resume)
  letter build <jd> [--company X] [--role Y] [--tone professional|warm|direct]
                [--manager NAME] [--out PATH|auto]
  prep <jd> [--out PATH|auto]       interview-prep sheet: deep-dives, gap defense,
                                    STAR story bank, questions to ask

TRACKER (append-only ledger: ~/.jobkit/applications.jsonl)
  track add <company> <role> [--url U] [--status S] [--note N]
  track list [--status S] [--all]   active applications (--all includes closed)
  track show <id>                   full event history (id prefixes ok)
  track set <id> --status S         move through the funnel
  track note <id> <text>            append a note
  track board                       kanban-style view by status
  track stats                       funnel + response-rate stats
  track followups [--days N]        applied-but-quiet apps needing a nudge

  statuses: discovered interested applied screening interview offer accepted
            rejected withdrawn ghosted

GLOBAL
  --json          agent envelope {ok, data|error:{code,message,hint}}
                  exit codes: 0 ok, 1 error, 2 invalid args, 3 not found
  JOBKIT_HOME     state dir override (default ~/.jobkit)
  JOBKIT_PROFILE  profile path override
`
