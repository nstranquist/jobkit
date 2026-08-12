// jobkit — a multi-purpose job application & resume toolkit.
//
// Surfaces: master profile (init/profile), JD parsing (jd), gap scoring
// (match), tailored resumes (resume), cover letters (letter), and an
// append-only application tracker (track). Every verb supports --json with
// the {ok, data|error:{code,message,hint}} envelope.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/calibration"
	"github.com/nstranquist/jobkit/internal/claims"
	"github.com/nstranquist/jobkit/internal/company"
	"github.com/nstranquist/jobkit/internal/contacts"
	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/formfill"
	"github.com/nstranquist/jobkit/internal/home"
	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/letter"
	"github.com/nstranquist/jobkit/internal/match"
	"github.com/nstranquist/jobkit/internal/prep"
	"github.com/nstranquist/jobkit/internal/privatefs"
	"github.com/nstranquist/jobkit/internal/profile"
	"github.com/nstranquist/jobkit/internal/resume"
	"github.com/nstranquist/jobkit/internal/searches"
	"github.com/nstranquist/jobkit/internal/telemetry"
	"github.com/nstranquist/jobkit/internal/track"
)

const version = "0.9.0"

// boolFlags take no value; everything else consumes one.
var boolFlags = map[string]bool{
	"json": true, "all": true, "full": true, "help": true, "remote": true,
	"inbox": true, "force": true, "strict": true, "compact": true,
	"relocation-open": true, "override-eligibility": true,
	"allow-unassessed-eligibility": true, "fix-permissions": true,
}

type cli struct {
	args  []string          // positionals
	flags map[string]string // --k v / --k=v; bools stored as "true"
}

func (c *cli) bool(name string) bool  { return c.flags[name] == "true" }
func (c *cli) str(name string) string { return c.flags[name] }
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
	case "search":
		return cmdSearch(c)
	case "calibrate":
		return cmdCalibrate(c)
	case "eligibility":
		return cmdEligibility(c)
	case "doctor":
		return cmdDoctor(c)
	case "claims":
		return cmdClaims(c)
	case "company":
		return cmdCompany(c)
	case "contact", "contacts":
		return cmdContact(c)
	case "jd":
		return cmdJD(c)
	case "find":
		return cmdFind(c)
	case "match":
		return cmdMatch(c)
	case "resume":
		return cmdResume(c)
	case "letter":
		return cmdLetter(c)
	case "prep":
		return cmdPrep(c)
	case "coach":
		return cmdCoach(c)
	case "apply-plan":
		return cmdApplyPlan(c)
	case "apply":
		return cmdApply(c)
	case "inbox":
		return cmdInbox(c)
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
		if c.bool("json") {
			envelope.EmitData(map[string]any{
				"version": version,
				"compact": c.bool("compact"),
				"help":    helpText(c.bool("compact")),
			})
			return nil
		}
		fmt.Print(helpText(c.bool("compact")))
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown command %q", cmd).WithHint("run `jobkit help` or `jobkit help --compact`")
	}
}

func cmdDoctor(c *cli) error {
	if _, ok := c.flags["fix"]; ok {
		return envelope.New(envelope.CodeInvalidArgs, "unknown option --fix").WithHint("use --fix-permissions")
	}
	sub := "permissions"
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	if sub != "permissions" {
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown doctor subcommand %q", sub).WithHint("permissions")
	}
	report, err := home.CheckPermissions(c.bool("fix-permissions"))
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if c.bool("json") {
		envelope.EmitData(report)
		return nil
	}
	if len(report.Issues) == 0 {
		fmt.Printf("permissions: secure (%s)\n", report.Root)
		return nil
	}
	verb := "found"
	if report.Fixed {
		verb = "fixed"
	}
	fmt.Printf("permissions: %s %d issue(s) under %s\n", verb, len(report.Issues), report.Root)
	for _, issue := range report.Issues {
		fmt.Printf("  %04o -> %04o  %s  %s\n", issue.Mode, issue.Want, issue.Kind, issue.Path)
	}
	if !report.Fixed {
		fmt.Println("run `jobkit doctor permissions --fix-permissions` to repair these paths explicitly")
	}
	return nil
}

// helpText returns the full human usage string, or a token-efficient compact
// verb map for agents and quick orientation.
func helpText(compact bool) string {
	if compact {
		return usageCompact
	}
	return usage
}

func cmdFind(c *cli) error {
	if len(c.args) < 2 {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit find <query> [--boards greenhouse:acme,lever:demo] [--targets ai-infra] [--remote] [--location X] [--eligibility actionable|eligible|review|ineligible|all] [--limit N] [--strict]")
	}
	eligibilityFilter := strings.ToLower(strings.TrimSpace(c.str("eligibility")))
	if !eligibility.ValidFilter(eligibilityFilter) {
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown eligibility filter %q", eligibilityFilter).
			WithHint("use actionable, eligible, review, ineligible, or all")
	}
	eligibilityPolicy, err := activeEligibilityPolicy()
	if err != nil {
		return err
	}
	if eligibilityFilter != "" && eligibilityPolicy == nil {
		return envelope.New(envelope.CodeInvalidArgs, "--eligibility requires an eligibility policy").
			WithHint("run `jobkit eligibility init --years N --home \"City, State\"`")
	}
	rawSpecs := searchSpecs(c.str("boards"), c.str("targets"))
	if len(rawSpecs) == 0 {
		return envelope.New(envelope.CodeInvalidArgs, "--boards or --targets is required").
			WithHint("example: jobkit find backend --targets ai-infra")
	}
	boards, expandedBoards, err := resolveBoardSpecs(rawSpecs)
	if err != nil {
		return envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	limit, err := c.int("limit", 20)
	if err != nil {
		return err
	}
	minComp, err := c.int("min-comp", 0)
	if err != nil {
		return err
	}
	weights, err := activeCalibrationWeights()
	if err != nil {
		return err
	}
	query := strings.Join(c.args[1:], " ")
	result, err := jobsearch.SearchReport(context.Background(), jobsearch.Options{
		Query:             query,
		Boards:            boards,
		Location:          c.str("location"),
		RemoteOnly:        c.bool("remote"),
		Limit:             limit,
		Strict:            c.bool("strict"),
		Sort:              c.str("sort"),
		MinComp:           minComp,
		Persona:           c.str("persona"),
		Weights:           weights,
		EligibilityPolicy: eligibilityPolicy,
		EligibilityFilter: firstNonEmpty(eligibilityFilter, "actionable"),
	})
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	jobs := result.Jobs
	if saveName := c.str("save"); saveName != "" {
		savedEligibility := ""
		if eligibilityPolicy != nil {
			savedEligibility = firstNonEmpty(eligibilityFilter, "actionable")
		}
		if err := saveSearchProfile(saveName, searches.Profile{
			Query: query, Boards: rawSpecs, RemoteOnly: c.bool("remote"), Location: c.str("location"), Limit: limit,
			Sort: c.str("sort"), MinComp: minComp, Persona: c.str("persona"), Eligibility: savedEligibility,
		}); err != nil {
			return err
		}
	}
	saveStats := inboxSaveStats{}
	if c.bool("inbox") {
		stats, err := saveJobsToInbox(jobs, query, "find")
		if err != nil {
			return err
		}
		saveStats = stats
	}
	if c.bool("json") {
		envelope.EmitData(map[string]any{"query": query, "boards": boards, "expanded_boards": expandedBoards, "jobs": jobs, "warnings": result.Warnings, "inbox_saved": saveStats.New, "inbox_seen": saveStats.Seen})
		return nil
	}
	printSearchWarnings(result.Warnings)
	if len(jobs) == 0 {
		fmt.Println("no matching jobs")
		return nil
	}
	for _, job := range jobs {
		loc := orDash(job.Location)
		fmt.Printf("[%s:%s] %s — %s\n", job.Provider, job.Board, job.Title, loc)
		if detail := jobSearchDetail(job); detail != "" {
			fmt.Printf("  %s\n", detail)
		}
		if job.Department != "" {
			fmt.Printf("  %s\n", job.Department)
		}
		if job.URL != "" {
			fmt.Printf("  %s\n", job.URL)
		}
	}
	if saveStats.New > 0 || saveStats.Seen > 0 {
		fmt.Printf("inbox: %d new, %d seen\n", saveStats.New, saveStats.Seen)
	}
	return nil
}

func searchSpecs(boards, targets string) []string {
	var specs []string
	for _, part := range strings.Split(boards, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			specs = append(specs, part)
		}
	}
	for _, part := range strings.Split(targets, ",") {
		part = strings.TrimSpace(strings.TrimPrefix(part, "#"))
		if part != "" {
			specs = append(specs, "#"+part)
		}
	}
	return specs
}

func printSearchWarnings(warnings []jobsearch.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "warning: skipped %s:%s: %s\n", warning.Provider, warning.Board, warning.Message)
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
		p, path, err := loadProfile()
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"valid": true, "path": path, "skills": len(p.Skills), "experience": len(p.Experience), "projects": len(p.Projects)})
		} else {
			fmt.Printf("OK: %s is valid\n", path)
		}
		return nil
	case "bootstrap":
		src := c.str("source")
		if src == "" && len(c.args) > 2 {
			src = c.args[2]
		}
		if src == "" {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit profile bootstrap --source resume.pdf [--out path|auto] [--force]")
		}
		res, err := profile.Bootstrap(profile.BootstrapOptions{Source: src})
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		out := c.str("out")
		if out == "" || out == "auto" {
			var err error
			out, err = home.ProfilePath()
			if err != nil {
				return envelope.New(envelope.CodeIOFailed, err.Error())
			}
		}
		if err := profile.Write(out, res.Profile, c.bool("force")); err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("use --force to overwrite, or pass --out PATH for a draft")
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": out, "source": res.Source, "extracted_bytes": res.ExtractedLen, "warnings": res.Warnings})
		} else {
			fmt.Printf("wrote bootstrap profile to %s\n", out)
			fmt.Println("next: review it, then run `jobkit profile validate`")
		}
		return nil
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit profile <show|validate|path|bootstrap>")
	}
}

// ---------- eligibility ----------

func cmdEligibility(c *cli) error {
	sub := "show"
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	path, err := home.EligibilityPath()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "init":
		if _, err := os.Stat(path); err == nil && !c.bool("force") {
			return envelope.Newf(envelope.CodeInvalidArgs, "%s already exists", path).
				WithHint("edit the policy directly or pass --force to replace it")
		} else if err != nil && !os.IsNotExist(err) {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		years, err := c.int("years", 0)
		if err != nil {
			return err
		}
		homeLocation := strings.TrimSpace(c.str("home"))
		if homeLocation == "" {
			if p, _, profileErr := loadProfile(); profileErr == nil {
				homeLocation = strings.TrimSpace(p.Location)
			}
		}
		var homeLocations []string
		if homeLocation != "" {
			homeLocations = []string{homeLocation}
		}
		config := eligibility.Template(homeLocations, years, c.bool("relocation-open"))
		if raw := c.str("countries"); raw != "" {
			config.Candidate.AllowedCountries = splitCSV(raw)
		}
		if raw := c.str("languages"); raw != "" {
			config.Candidate.Languages = splitCSV(raw)
		}
		if err := eligibility.Save(path, config); err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": path, "config": config})
		} else {
			fmt.Printf("created eligibility policy at %s\n", path)
			fmt.Println("hard constraints now gate find/search/inbox/apply; fit and opportunity scores remain independent")
		}
		return nil
	case "path":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "show":
		config, err := eligibility.Load(path)
		if os.IsNotExist(err) {
			return envelope.Newf(envelope.CodeNotFound, "no eligibility policy at %s", path).
				WithHint("run `jobkit eligibility init --years N --home \"City, State\"`")
		}
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": path, "config": config})
		} else {
			policy := config.EffectivePolicy()
			fmt.Printf("eligibility policy: %s\n", path)
			fmt.Printf("  experience: %d years  relocation: %t  max travel: %d%%\n", config.Candidate.YearsExperience, config.Candidate.RelocationOpen, policy.MaxTravelPercent)
			fmt.Printf("  homes: %s\n  countries: %s\n  languages: %s\n  role families: %s\n",
				strings.Join(config.Candidate.HomeLocations, ", "), strings.Join(config.Candidate.AllowedCountries, ", "),
				strings.Join(config.Candidate.Languages, ", "), strings.Join(policy.AllowedRoleFamilies, ", "))
		}
		return nil
	case "check":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit eligibility check <jd-file|url|-> [--role X] [--location X] [--remote]")
		}
		text, err := readInput(c.args[2])
		if err != nil {
			return err
		}
		parsed := jd.Parse(text)
		assessment, err := assessEligibility(firstNonEmpty(c.str("role"), parsed.Title), c.str("location"), c.bool("remote"), text)
		if err != nil {
			return err
		}
		if assessment == nil {
			return envelope.New(envelope.CodeNotFound, "no eligibility policy configured").
				WithHint("run `jobkit eligibility init --years N --home \"City, State\"`")
		}
		if c.bool("json") {
			envelope.EmitData(assessment)
		} else {
			fmt.Printf("%s · %s · %s\n", assessment.Status, assessment.RoleFamily, assessment.WorkMode)
			for _, reason := range assessment.Reasons {
				fmt.Printf("  %s: %s\n", reason.Code, reason.Summary)
			}
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown eligibility subcommand %q", sub).WithHint("init|show|path|check")
	}
}

// ---------- saved searches ----------

func searchesPath() (string, error) {
	return home.SearchesPath()
}

func loadSearchConfig() (*searches.Config, string, error) {
	path, err := searchesPath()
	if err != nil {
		return nil, "", envelope.New(envelope.CodeIOFailed, err.Error())
	}
	cfg, err := searches.LoadOrEmpty(path)
	if err != nil {
		return nil, "", envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	return cfg, path, nil
}

func resolveBoardSpecs(specs []string) ([]jobsearch.Board, []string, error) {
	cfg, _, err := loadSearchConfig()
	if err != nil {
		return nil, nil, err
	}
	return cfg.ResolveBoards(specs)
}

func saveSearchProfile(name string, p searches.Profile) error {
	cfg, path, err := loadSearchConfig()
	if err != nil {
		return err
	}
	cfg.AddSearch(name, p)
	if err := searches.Save(path, cfg); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return nil
}

func cmdSearch(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	path, err := searchesPath()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "init":
		if err := searches.WriteTemplate(path); err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("edit the existing file or move it aside")
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Printf("created saved-search config at %s\n", path)
		}
		return nil
	case "path":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "list", "":
		cfg, _, err := loadSearchConfig()
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(cfg)
			return nil
		}
		fmt.Println("board groups:")
		for _, name := range cfg.BoardNames() {
			fmt.Printf("  @%s: %s\n", name, strings.Join(cfg.Boards[name], ", "))
		}
		fmt.Println("target packs:")
		for _, name := range cfg.TargetNames() {
			t := cfg.Targets[name]
			desc := ""
			if t.Description != "" {
				desc = " — " + t.Description
			}
			fmt.Printf("  #%s: %d boards%s\n", name, len(t.Boards), desc)
		}
		fmt.Println("searches:")
		for _, name := range cfg.SearchNames() {
			p := cfg.Searches[name]
			fmt.Printf("  %s: %q on %s\n", name, p.Query, strings.Join(p.Boards, ", "))
		}
		return nil
	case "show":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit search show <name>")
		}
		cfg, _, err := loadSearchConfig()
		if err != nil {
			return err
		}
		p, ok := cfg.Searches[c.args[2]]
		if !ok {
			return envelope.Newf(envelope.CodeNotFound, "unknown saved search %q", c.args[2])
		}
		if c.bool("json") {
			envelope.EmitData(p)
		} else {
			fmt.Printf("%s: %q\nboards: %s\nremote: %v\nlocation: %s\nlimit: %d\nsort: %s\npersona: %s\nmin comp: %d\neligibility: %s\n", c.args[2], p.Query, strings.Join(p.Boards, ", "), p.RemoteOnly, p.Location, p.Limit, orDash(p.Sort), orDash(p.Persona), p.MinComp, orDash(p.Eligibility))
		}
		return nil
	case "digest":
		cfg, _, err := loadSearchConfig()
		if err != nil {
			return err
		}
		names := cfg.SearchNames()
		if len(c.args) >= 3 {
			names = []string{c.args[2]}
		}
		report, err := buildSearchDigest(c, cfg, names)
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(report)
			return nil
		}
		return writeArtifact(c, "search-digest", "md", renderSearchDigest(report), map[string]any{"searches": len(report.Searches), "jobs": report.TotalJobs})
	case "run":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit search run <name> [--inbox] [--strict] [--json]")
		}
		cfg, _, err := loadSearchConfig()
		if err != nil {
			return err
		}
		p, ok := cfg.Searches[c.args[2]]
		if !ok {
			return envelope.Newf(envelope.CodeNotFound, "unknown saved search %q", c.args[2])
		}
		boards, expanded, err := cfg.ResolveBoards(p.Boards)
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		limit := p.Limit
		if limit == 0 {
			limit = 20
		}
		opts, err := searchOptionsForProfile(c, p, boards, limit)
		if err != nil {
			return err
		}
		result, err := jobsearch.SearchReport(context.Background(), opts)
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		jobs := result.Jobs
		saveStats := inboxSaveStats{}
		if c.bool("inbox") {
			stats, err := saveJobsToInbox(jobs, p.Query, "search:"+c.args[2])
			if err != nil {
				return err
			}
			saveStats = stats
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"name": c.args[2], "query": p.Query, "expanded_boards": expanded, "jobs": jobs, "warnings": result.Warnings, "inbox_saved": saveStats.New, "inbox_seen": saveStats.Seen})
			return nil
		}
		printSearchWarnings(result.Warnings)
		for _, job := range jobs {
			fmt.Printf("[%s:%s] %s — %s\n  %s\n", job.Provider, job.Board, job.Title, orDash(job.Location), job.URL)
			if detail := jobSearchDetail(job); detail != "" {
				fmt.Printf("  %s\n", detail)
			}
		}
		if saveStats.New > 0 || saveStats.Seen > 0 {
			fmt.Printf("inbox: %d new, %d seen\n", saveStats.New, saveStats.Seen)
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown search subcommand %q", sub).WithHint("init|path|list|show|run|digest")
	}
}

type searchDigest struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Searches    []searchRunInfo `json:"searches"`
	TotalJobs   int             `json:"total_jobs"`
	TotalNew    int             `json:"total_new"`
	TotalSeen   int             `json:"total_seen"`
	Warnings    int             `json:"warnings"`
}

type searchRunInfo struct {
	Name           string              `json:"name"`
	Query          string              `json:"query"`
	ExpandedBoards []string            `json:"expanded_boards"`
	Jobs           []jobsearch.Job     `json:"jobs"`
	Warnings       []jobsearch.Warning `json:"warnings"`
	Inbox          *inboxSaveStats     `json:"inbox,omitempty"`
}

func buildSearchDigest(c *cli, cfg *searches.Config, names []string) (*searchDigest, error) {
	report := &searchDigest{GeneratedAt: time.Now().UTC()}
	for _, name := range names {
		p, ok := cfg.Searches[name]
		if !ok {
			return nil, envelope.Newf(envelope.CodeNotFound, "unknown saved search %q", name)
		}
		boards, expanded, err := cfg.ResolveBoards(p.Boards)
		if err != nil {
			return nil, envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		limit := p.Limit
		if limit == 0 {
			limit = 20
		}
		opts, err := searchOptionsForProfile(c, p, boards, limit)
		if err != nil {
			return nil, err
		}
		result, err := jobsearch.SearchReport(context.Background(), opts)
		if err != nil {
			return nil, envelope.New(envelope.CodeIOFailed, err.Error())
		}
		info := searchRunInfo{Name: name, Query: p.Query, ExpandedBoards: expanded, Jobs: result.Jobs, Warnings: result.Warnings}
		if c.bool("inbox") {
			stats, err := saveJobsToInbox(result.Jobs, p.Query, "digest:"+name)
			if err != nil {
				return nil, err
			}
			info.Inbox = &stats
			report.TotalNew += stats.New
			report.TotalSeen += stats.Seen
		}
		report.TotalJobs += len(result.Jobs)
		report.Warnings += len(result.Warnings)
		report.Searches = append(report.Searches, info)
	}
	return report, nil
}

func searchOptionsForProfile(c *cli, p searches.Profile, boards []jobsearch.Board, limit int) (jobsearch.Options, error) {
	minComp := p.MinComp
	if raw, ok := c.flags["min-comp"]; ok {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return jobsearch.Options{}, envelope.New(envelope.CodeInvalidArgs, "--min-comp must be an integer")
		}
		minComp = n
	}
	weights, err := activeCalibrationWeights()
	if err != nil {
		return jobsearch.Options{}, err
	}
	eligibilityFilter := firstNonEmpty(c.str("eligibility"), p.Eligibility)
	if !eligibility.ValidFilter(eligibilityFilter) {
		return jobsearch.Options{}, envelope.Newf(envelope.CodeInvalidArgs, "unknown eligibility filter %q in saved search", eligibilityFilter)
	}
	eligibilityPolicy, err := activeEligibilityPolicy()
	if err != nil {
		return jobsearch.Options{}, err
	}
	if eligibilityFilter != "" && eligibilityPolicy == nil {
		return jobsearch.Options{}, envelope.New(envelope.CodeInvalidArgs, "saved search requires an eligibility policy").
			WithHint("run `jobkit eligibility init --years N --home \"City, State\"`")
	}
	return jobsearch.Options{
		Query: p.Query, Boards: boards, Location: p.Location, RemoteOnly: p.RemoteOnly, Limit: limit,
		Strict: c.bool("strict"), Sort: firstNonEmpty(c.str("sort"), p.Sort), MinComp: minComp,
		Persona: firstNonEmpty(c.str("persona"), p.Persona), Weights: weights,
		EligibilityPolicy: eligibilityPolicy, EligibilityFilter: firstNonEmpty(eligibilityFilter, "actionable"),
	}, nil
}

func calibrationPath() (string, error) {
	return home.CalibrationPath()
}

func activeEligibilityPolicy() (*eligibility.Policy, error) {
	path, err := home.EligibilityPath()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	config, err := eligibility.Load(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("fix or remove " + path)
	}
	policy := config.EffectivePolicy()
	return &policy, nil
}

func assessEligibility(role, location string, remote bool, description string) (*eligibility.Result, error) {
	policy, err := activeEligibilityPolicy()
	if err != nil || policy == nil {
		return nil, err
	}
	assessment := eligibility.Evaluate(eligibility.Posting{
		Title: role, Location: location, Remote: remote, Description: description,
	}, *policy)
	return &assessment, nil
}

func activeCalibrationWeights() (jobsearch.OpportunityWeights, error) {
	path, err := calibrationPath()
	if err != nil {
		return jobsearch.OpportunityWeights{}, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	cfg, err := calibration.Load(path)
	if os.IsNotExist(err) {
		return jobsearch.DefaultOpportunityWeights(), nil
	}
	if err != nil {
		return jobsearch.OpportunityWeights{}, envelope.New(envelope.CodeInvalidArgs, err.Error()).
			WithHint("fix or remove " + path)
	}
	return jobsearch.NormalizeOpportunityWeights(cfg.Weights), nil
}

func cmdCalibrate(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	path, err := calibrationPath()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "path":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "show":
		cfg, err := calibration.Load(path)
		if os.IsNotExist(err) {
			if c.bool("json") {
				envelope.EmitData(map[string]any{"active": false, "path": path, "weights": jobsearch.DefaultOpportunityWeights()})
			} else {
				fmt.Printf("no active calibration at %s\n", path)
				fmt.Printf("default weights: %s\n", formatWeights(jobsearch.DefaultOpportunityWeights()))
			}
			return nil
		}
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("fix or remove " + path)
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"active": true, "path": path, "config": cfg})
			return nil
		}
		fmt.Printf("active calibration: %s\n", path)
		fmt.Printf("updated: %s  persona: %s  samples: %d\n", cfg.UpdatedAt.Format(time.RFC3339), orDash(cfg.Persona), cfg.Samples)
		fmt.Printf("weights: %s\n", formatWeights(cfg.Weights))
		fmt.Printf("accuracy: %.0f%% over %d pairs\n", cfg.Metrics.PairwiseAccuracy*100, cfg.Metrics.Pairs)
		return nil
	case "report", "":
		report, err := calibrationReport(c, path)
		if err != nil {
			return err
		}
		if c.bool("json") {
			envelope.EmitData(report)
			return nil
		}
		printCalibrationReport(report)
		return nil
	case "apply":
		report, err := calibrationReport(c, path)
		if err != nil {
			return err
		}
		minSamples, err := c.int("min-samples", 8)
		if err != nil {
			return err
		}
		if report.Samples < minSamples && !c.bool("force") {
			return envelope.Newf(envelope.CodeInvalidArgs, "only %d outcome sample(s), need %d", report.Samples, minSamples).
				WithHint("collect more applied outcomes, lower --min-samples, or pass --force")
		}
		if report.Suggested.Metrics.Pairs == 0 && !c.bool("force") {
			return envelope.New(envelope.CodeInvalidArgs, "no positive-vs-negative outcome pairs to calibrate").
				WithHint("mark at least one lead positive and one lead negative, or pass --force to write defaults")
		}
		cfg := &calibration.Config{
			UpdatedAt: time.Now().UTC(),
			Persona:   report.Persona,
			Samples:   report.Samples,
			Weights:   report.Suggested.Weights,
			Metrics:   report.Suggested.Metrics,
		}
		if err := calibration.Save(path, cfg); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": path, "config": cfg, "report": report})
			return nil
		}
		fmt.Printf("wrote calibration: %s\n", path)
		fmt.Printf("weights: %s\n", formatWeights(cfg.Weights))
		fmt.Printf("accuracy: %.0f%% over %d pairs\n", cfg.Metrics.PairwiseAccuracy*100, cfg.Metrics.Pairs)
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown calibrate subcommand %q", sub).WithHint("path|show|report|apply")
	}
}

func calibrationReport(c *cli, path string) (calibration.Report, error) {
	var active *jobsearch.OpportunityWeights
	activeCfg, err := calibration.Load(path)
	if err == nil {
		weights := activeCfg.Weights
		active = &weights
	} else if err != nil && !os.IsNotExist(err) {
		return calibration.Report{}, envelope.New(envelope.CodeInvalidArgs, err.Error()).WithHint("fix or remove " + path)
	}
	persona := firstNonEmpty(c.str("persona"))
	if persona == "" && activeCfg != nil {
		persona = activeCfg.Persona
	}
	if persona == "" {
		persona = "agent-infra"
	}
	il, err := openInboxLedger()
	if err != nil {
		return calibration.Report{}, err
	}
	items, err := il.Replay()
	if err != nil {
		return calibration.Report{}, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	tl, err := openLedger()
	if err != nil {
		return calibration.Report{}, err
	}
	apps, err := tl.Replay()
	if err != nil {
		return calibration.Report{}, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return calibration.BuildReport(items, apps, persona, active), nil
}

func printCalibrationReport(report calibration.Report) {
	fmt.Printf("persona: %s\n", report.Persona)
	fmt.Printf("samples: %d positive, %d negative, %d neutral\n", report.Positive, report.Negative, report.Neutral)
	fmt.Printf("default:   %3.0f%% accuracy over %d pairs, spread %.1f, weights %s\n",
		report.Default.Metrics.PairwiseAccuracy*100, report.Default.Metrics.Pairs, report.Default.Metrics.Spread, formatWeights(report.Default.Weights))
	if report.Active != nil {
		fmt.Printf("active:    %3.0f%% accuracy over %d pairs, spread %.1f, weights %s\n",
			report.Active.Metrics.PairwiseAccuracy*100, report.Active.Metrics.Pairs, report.Active.Metrics.Spread, formatWeights(report.Active.Weights))
	}
	fmt.Printf("suggested: %3.0f%% accuracy over %d pairs, spread %.1f, weights %s\n",
		report.Suggested.Metrics.PairwiseAccuracy*100, report.Suggested.Metrics.Pairs, report.Suggested.Metrics.Spread, formatWeights(report.Suggested.Weights))
	if report.Samples == 0 {
		fmt.Println("no outcome samples yet; mark inbox items or tracked applications to calibrate ranking")
	}
}

func formatWeights(weights jobsearch.OpportunityWeights) string {
	weights = jobsearch.NormalizeOpportunityWeights(weights)
	return fmt.Sprintf("base %.1f, fresh %.1f, comp %.1f, persona %.1f, saturation %.1f",
		weights.Base, weights.Freshness, weights.Compensation, weights.Persona, weights.Saturation)
}

func jobSearchDetail(job jobsearch.Job) string {
	var parts []string
	if job.Eligibility != nil {
		label := string(job.Eligibility.Status) + " · " + job.Eligibility.RoleFamily
		if len(job.Eligibility.Reasons) > 0 {
			label += " (" + job.Eligibility.Reasons[0].Code + ")"
		}
		parts = append(parts, label)
	}
	if job.Compensation != nil {
		parts = append(parts, "pay "+formatComp(job.Compensation))
	}
	if job.Opportunity.Score != 0 || job.Opportunity.Persona != "" || job.Opportunity.SaturationRisk != 0 {
		detail := fmt.Sprintf("opp %d", job.Opportunity.Score)
		if job.Opportunity.Persona != "" {
			detail += " · " + job.Opportunity.Persona
			if job.Opportunity.PersonaScore != 0 {
				detail += fmt.Sprintf(" %d", job.Opportunity.PersonaScore)
			}
		}
		if job.Opportunity.SaturationRisk != 0 {
			detail += fmt.Sprintf(" · saturation %d", job.Opportunity.SaturationRisk)
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, " · ")
}

func formatComp(c *jobsearch.Compensation) string {
	if c == nil {
		return ""
	}
	currency := firstNonEmpty(c.Currency, "USD")
	if c.Min > 0 && c.Max > 0 && c.Min != c.Max {
		return fmt.Sprintf("%s%d-%d/%s", currency, c.Min, c.Max, firstNonEmpty(c.Period, "year"))
	}
	amt := c.Max
	if amt == 0 {
		amt = c.Min
	}
	return fmt.Sprintf("%s%d/%s", currency, amt, firstNonEmpty(c.Period, "year"))
}

func renderSearchDigest(report *searchDigest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Jobkit Search Digest\n\nGenerated: %s\n\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Total jobs: %d\n\n", report.TotalJobs)
	if report.TotalNew > 0 || report.TotalSeen > 0 {
		fmt.Fprintf(&b, "Inbox: %d new, %d seen\n\n", report.TotalNew, report.TotalSeen)
	}
	for _, run := range report.Searches {
		fmt.Fprintf(&b, "## %s\n\n", run.Name)
		fmt.Fprintf(&b, "Query: `%s`\n\n", run.Query)
		fmt.Fprintf(&b, "Boards: %s\n\n", strings.Join(run.ExpandedBoards, ", "))
		if run.Inbox != nil {
			fmt.Fprintf(&b, "Inbox: %d new, %d seen\n\n", run.Inbox.New, run.Inbox.Seen)
		}
		if len(run.Warnings) > 0 {
			b.WriteString("Warnings:\n")
			for _, warning := range run.Warnings {
				fmt.Fprintf(&b, "- skipped `%s:%s`: %s\n", warning.Provider, warning.Board, warning.Message)
			}
			b.WriteString("\n")
		}
		if len(run.Jobs) == 0 {
			b.WriteString("No matching jobs.\n\n")
			continue
		}
		for _, job := range run.Jobs {
			fmt.Fprintf(&b, "- **%s** — %s", job.Title, orDash(job.Location))
			if detail := jobSearchDetail(job); detail != "" {
				fmt.Fprintf(&b, "  \n  %s", detail)
			}
			if job.URL != "" {
				fmt.Fprintf(&b, "  \n  %s", job.URL)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---------- hidden-market companies ----------

func companiesPath() (string, error) {
	return home.CompaniesPath()
}

func loadCompanyConfig() (*company.Config, string, error) {
	path, err := companiesPath()
	if err != nil {
		return nil, "", envelope.New(envelope.CodeIOFailed, err.Error())
	}
	cfg, err := company.LoadOrEmpty(path)
	if err != nil {
		return nil, "", envelope.New(envelope.CodeInvalidArgs, err.Error())
	}
	return cfg, path, nil
}

func cmdCompany(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	switch sub {
	case "path":
		path, err := companiesPath()
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "add":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit company add <name> [--domain D] [--stage S] [--tags a,b] [--boards provider:slug] [--target-comp N]")
		}
		targetComp, err := c.int("target-comp", 0)
		if err != nil {
			return err
		}
		cfg, path, err := loadCompanyConfig()
		if err != nil {
			return err
		}
		co := cfg.Upsert(company.Company{
			Name: strings.Join(c.args[2:], " "), Domain: c.str("domain"), Stage: c.str("stage"),
			Tags: splitCSV(c.str("tags")), Boards: splitCSV(firstNonEmpty(c.str("boards"), c.str("board"))),
			TargetComp: targetComp,
		})
		if co.Name == "" {
			return envelope.New(envelope.CodeInvalidArgs, "company name is required")
		}
		if err := company.Save(path, cfg); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(co)
		} else {
			fmt.Printf("saved company %s\n", co.Name)
		}
		return nil
	case "signal":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit company signal <name> --type funding|launch|team-growth|recruiter|backchannel [--source X] [--url U] [--note N] [--weight N]")
		}
		weight, err := c.int("weight", 0)
		if err != nil {
			return err
		}
		cfg, path, err := loadCompanyConfig()
		if err != nil {
			return err
		}
		co, err := cfg.AddSignal(strings.Join(c.args[2:], " "), company.Signal{
			Type: firstNonEmpty(c.str("type"), "manual"), Source: c.str("source"), URL: c.str("url"),
			Note: c.str("note"), Weight: weight,
		})
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if err := company.Save(path, cfg); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(co)
		} else {
			fmt.Printf("added signal to %s\n", co.Name)
		}
		return nil
	case "note":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit company note <name> <text|--note N>")
		}
		cfg, path, err := loadCompanyConfig()
		if err != nil {
			return err
		}
		name, note, err := companyNoteArgs(cfg, c)
		if err != nil {
			return err
		}
		if note == "" {
			return envelope.New(envelope.CodeInvalidArgs, "note text is required")
		}
		co, err := cfg.AddNote(name, note)
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if err := company.Save(path, cfg); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(co)
		} else {
			fmt.Printf("noted on %s\n", co.Name)
		}
		return nil
	case "show":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit company show <name>")
		}
		cfg, _, err := loadCompanyConfig()
		if err != nil {
			return err
		}
		co, ok := cfg.Find(strings.Join(c.args[2:], " "))
		if !ok {
			return envelope.Newf(envelope.CodeNotFound, "unknown company %q", strings.Join(c.args[2:], " "))
		}
		data := company.Ranked{Company: co}
		for _, ranked := range cfg.Ranked(time.Now()) {
			if company.Key(ranked.Company.Name) == company.Key(co.Name) {
				data = ranked
				break
			}
		}
		if c.bool("json") {
			envelope.EmitData(data)
			return nil
		}
		printCompany(data)
		return nil
	case "list", "":
		cfg, _, err := loadCompanyConfig()
		if err != nil {
			return err
		}
		ranked := filterCompanies(cfg.Ranked(time.Now()), c)
		if ranked == nil {
			ranked = []company.Ranked{}
		}
		if c.bool("json") {
			envelope.EmitData(ranked)
			return nil
		}
		if len(ranked) == 0 {
			fmt.Println("no target companies (use `jobkit company add <name>`)")
			return nil
		}
		for _, item := range ranked {
			fmt.Printf("%3d %-16s %-28s tags:%-20s boards:%s\n",
				item.Score, item.NextAction, item.Company.Name, strings.Join(item.Company.Tags, ","), strings.Join(item.Company.Boards, ","))
			if len(item.Signals) > 0 || !item.LastSignal.IsZero() {
				fmt.Printf("    signals:%s last:%s\n", strings.Join(item.Signals, ","), shortTime(item.LastSignal))
			}
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown company subcommand %q", sub).WithHint("add|signal|note|list|show|path")
	}
}

func companyNoteArgs(cfg *company.Config, c *cli) (string, string, error) {
	if note := c.str("note"); note != "" {
		return strings.Join(c.args[2:], " "), note, nil
	}
	parts := c.args[2:]
	for i := len(parts) - 1; i >= 1; i-- {
		name := strings.Join(parts[:i], " ")
		found, ok := cfg.Find(name)
		if ok && found.Name != "" {
			return found.Name, strings.Join(parts[i:], " "), nil
		}
	}
	if len(parts) == 0 {
		return "", "", nil
	}
	if len(parts) > 2 {
		return "", "", envelope.New(envelope.CodeInvalidArgs, "could not split company name from note").
			WithHint("for a new multi-word company, use `jobkit company note \"New Co\" --note \"text\"`")
	}
	return parts[0], strings.Join(parts[1:], " "), nil
}

func filterCompanies(items []company.Ranked, c *cli) []company.Ranked {
	tag := strings.ToLower(strings.TrimSpace(c.str("tag")))
	stage := strings.ToLower(strings.TrimSpace(c.str("stage")))
	if tag == "" && stage == "" {
		return items
	}
	var out []company.Ranked
	for _, item := range items {
		if stage != "" && strings.ToLower(item.Company.Stage) != stage {
			continue
		}
		if tag != "" && !containsFoldString(item.Company.Tags, tag) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func printCompany(item company.Ranked) {
	co := item.Company
	fmt.Printf("%s\n", co.Name)
	fmt.Printf("  score: %d  next: %s\n", item.Score, item.NextAction)
	if co.Domain != "" || co.Stage != "" || co.TargetComp > 0 {
		fmt.Printf("  domain: %s  stage: %s  target comp: %d\n", orDash(co.Domain), orDash(co.Stage), co.TargetComp)
	}
	if len(co.Tags) > 0 {
		fmt.Printf("  tags: %s\n", strings.Join(co.Tags, ", "))
	}
	if len(co.Boards) > 0 {
		fmt.Printf("  boards: %s\n", strings.Join(co.Boards, ", "))
	}
	if len(item.Signals) > 0 {
		fmt.Printf("  score signals: %s\n", strings.Join(item.Signals, ", "))
	}
	if len(co.Signals) > 0 {
		fmt.Println("  signals:")
		for _, sig := range co.Signals {
			fmt.Printf("    %s  %s", shortTime(sig.TS), sig.Type)
			if sig.Source != "" {
				fmt.Printf(" from %s", sig.Source)
			}
			if sig.Note != "" {
				fmt.Printf(" — %s", sig.Note)
			}
			if sig.URL != "" {
				fmt.Printf("  %s", sig.URL)
			}
			fmt.Println()
		}
	}
	if len(co.Notes) > 0 {
		fmt.Println("  notes:")
		for _, note := range co.Notes {
			fmt.Printf("    %s  %s\n", shortTime(note.TS), note.Text)
		}
	}
}

// ---------- contacts / referrals ----------

func openContactsLedger() (*contacts.Ledger, error) {
	path, err := home.ContactsPath()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return &contacts.Ledger{Path: path}, nil
}

func contactsPath() (string, error) {
	return home.ContactsPath()
}

// targetOverlap lists ledger contacts whose company matches a tracked target
// company (companies.yaml). Best-effort: config problems return nil.
func targetOverlap(l *contacts.Ledger) []string {
	path, err := companiesPath()
	if err != nil {
		return nil
	}
	cfg, err := company.LoadOrEmpty(path)
	if err != nil || len(cfg.Companies) == 0 {
		return nil
	}
	items, err := l.Replay()
	if err != nil {
		return nil
	}
	targets := map[string]string{} // normalized name -> display name
	for _, tc := range cfg.Companies {
		targets[strings.ToLower(strings.TrimSpace(tc.Name))] = tc.Name
	}
	var out []string
	for _, item := range items {
		if display, ok := targets[strings.ToLower(strings.TrimSpace(item.Company))]; ok {
			out = append(out, fmt.Sprintf("%s — %s (%s) [%s]", display, item.Name, firstNonEmpty(item.Role, "role unknown"), item.ID))
		}
	}
	sort.Strings(out)
	return out
}

func cmdContact(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	switch sub {
	case "path":
		path, err := contactsPath()
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	}
	l, err := openContactsLedger()
	if err != nil {
		return err
	}
	items, err := l.Replay()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "add":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact add <name> [--company X] [--role X] [--channel linkedin|email] [--url U] [--email E] [--source X] [--inbox-id ID] [--track-id ID] [--status S]")
		}
		status := firstNonEmpty(c.str("status"), "lead")
		if !contacts.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(contacts.Statuses, ", "))
		}
		name := strings.Join(c.args[2:], " ")
		id := contacts.NewID(items, name, c.str("company"))
		ev := contacts.Event{
			ID: id, Type: contacts.EvCreated, Status: status, Name: name, Company: c.str("company"), Role: c.str("role"),
			Channel: c.str("channel"), URL: c.str("url"), Email: c.str("email"), Source: c.str("source"),
			InboxID: c.str("inbox-id"), TrackID: firstNonEmpty(c.str("track-id"), c.str("track")), Note: c.str("note"),
		}
		if err := l.Append(ev); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": id, "status": status})
		} else {
			fmt.Printf("added contact %s (%s)\n", id, status)
		}
		return nil
	case "import":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact import <connections.csv> [--source X]").WithHint("export from LinkedIn: Settings → Data privacy → Get a copy of your data → Connections")
		}
		f, err := os.Open(c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		rows, err := contacts.ParseLinkedInCSV(f)
		closeErr := f.Close()
		if err != nil {
			return envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		if closeErr != nil {
			return envelope.New(envelope.CodeIOFailed, closeErr.Error())
		}
		source := firstNonEmpty(c.str("source"), "linkedin-export")
		sum, err := contacts.Import(l, rows, source)
		if err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		overlap := targetOverlap(l)
		if c.bool("json") {
			envelope.EmitData(map[string]any{"summary": sum, "target_overlap": overlap})
			return nil
		}
		fmt.Printf("parsed %d, imported %d, skipped %d duplicates\n", sum.Parsed, sum.Imported, sum.Skipped)
		if len(overlap) > 0 {
			fmt.Println("contacts at target companies (warm-referral candidates):")
			for _, line := range overlap {
				fmt.Println("  " + line)
			}
		}
		return nil
	case "list", "":
		filtered := filterContacts(items, c)
		if filtered == nil {
			filtered = []*contacts.Item{}
		}
		if c.bool("json") {
			envelope.EmitData(filtered)
			return nil
		}
		if len(filtered) == 0 {
			fmt.Println("no contacts (use `jobkit contact add <name>`)")
			return nil
		}
		for _, item := range filtered {
			fmt.Printf("%-34s %-20s %-18s %-10s %s\n", item.ID, item.Company, item.Status, shortTime(item.LastTouchAt), item.Name)
		}
		return nil
	case "show":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact show <id>")
		}
		item, err := contacts.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(item)
			return nil
		}
		printContact(item)
		return nil
	case "touch":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact touch <id> [--status S] [--note N] [--inbox-id ID] [--track-id ID]")
		}
		item, err := contacts.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		status := c.str("status")
		if status != "" && !contacts.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(contacts.Statuses, ", "))
		}
		if err := l.Append(contacts.Event{
			ID: item.ID, Type: contacts.EvTouch, Status: status, Note: firstNonEmpty(c.str("note"), strings.Join(c.args[3:], " ")),
			InboxID: c.str("inbox-id"), TrackID: firstNonEmpty(c.str("track-id"), c.str("track")),
		}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": item.ID, "status": firstNonEmpty(status, item.Status)})
		} else {
			fmt.Printf("touched %s\n", item.ID)
		}
		return nil
	case "referral":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact referral <id> [--status referral-requested|referral-offered|referred] [--note N] [--inbox-id ID] [--track-id ID]")
		}
		item, err := contacts.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		status := firstNonEmpty(c.str("status"), "referral-requested")
		if !contacts.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(contacts.Statuses, ", "))
		}
		if err := l.Append(contacts.Event{
			ID: item.ID, Type: contacts.EvReferral, Status: status, Note: firstNonEmpty(c.str("note"), strings.Join(c.args[3:], " ")),
			InboxID: c.str("inbox-id"), TrackID: firstNonEmpty(c.str("track-id"), c.str("track")),
		}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": item.ID, "status": status})
		} else {
			fmt.Printf("%s → %s\n", item.ID, status)
		}
		return nil
	case "note":
		if len(c.args) < 4 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit contact note <id> <text>")
		}
		item, err := contacts.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		note := strings.Join(c.args[3:], " ")
		if err := l.Append(contacts.Event{ID: item.ID, Type: contacts.EvNote, Note: note}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": item.ID, "note": note})
		} else {
			fmt.Printf("noted on %s\n", item.ID)
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown contact subcommand %q", sub).WithHint("add|list|show|touch|referral|note|path")
	}
}

func filterContacts(items []*contacts.Item, c *cli) []*contacts.Item {
	status := strings.ToLower(strings.TrimSpace(c.str("status")))
	companyFilter := strings.ToLower(strings.TrimSpace(c.str("company")))
	inboxID := strings.TrimSpace(c.str("inbox-id"))
	trackID := strings.TrimSpace(firstNonEmpty(c.str("track-id"), c.str("track")))
	if status == "" && companyFilter == "" && inboxID == "" && trackID == "" {
		return items
	}
	var out []*contacts.Item
	for _, item := range items {
		if status != "" && strings.ToLower(item.Status) != status {
			continue
		}
		if companyFilter != "" && !strings.Contains(strings.ToLower(item.Company), companyFilter) {
			continue
		}
		if inboxID != "" && item.InboxID != inboxID {
			continue
		}
		if trackID != "" && item.TrackID != trackID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func printContact(item *contacts.Item) {
	fmt.Printf("%s\n  %s", item.ID, item.Name)
	if item.Company != "" || item.Role != "" {
		fmt.Printf(" — %s / %s", orDash(item.Company), orDash(item.Role))
	}
	fmt.Println()
	fmt.Printf("  status: %s  channel: %s  last touch: %s\n", item.Status, orDash(item.Channel), shortTime(item.LastTouchAt))
	if item.URL != "" || item.Email != "" {
		fmt.Printf("  url: %s  email: %s\n", orDash(item.URL), orDash(item.Email))
	}
	if item.InboxID != "" || item.TrackID != "" || item.Source != "" {
		fmt.Printf("  source: %s  inbox: %s  track: %s\n", orDash(item.Source), orDash(item.InboxID), orDash(item.TrackID))
	}
	fmt.Println("  history:")
	for _, ev := range item.Events {
		line := ev.Type
		if ev.Status != "" {
			line += " → " + ev.Status
		}
		if ev.Note != "" {
			line += ": " + ev.Note
		}
		fmt.Printf("    %s  %s\n", ev.TS.Local().Format("2006-01-02 15:04"), line)
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
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit resume build [jd-file] [--format md|txt|html|pdf] [--out path] [--max-bullets N] [--full]")
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
	case "pdf":
		path, err := artifactPath(c, "resume", "pdf")
		if err != nil {
			return err
		}
		if path == "" {
			return envelope.New(envelope.CodeInvalidArgs, "--out is required for --format pdf").WithHint("use --out resume.pdf or --out auto")
		}
		if err := checkGeneratedClaims("resume", resume.RenderText(doc)); err != nil {
			return err
		}
		if err := resume.RenderPDF(resume.RenderHTML(doc), path); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{
				"tailored": doc.Tailored, "target_title": doc.TargetTitle, "format": format, "path": path,
			})
		} else {
			fmt.Printf("wrote %s\n", path)
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown format %q (md|txt|html|pdf)", format)
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

// cmdClaims manages the fact-lock allowlist for generated material.
func cmdClaims(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	path, err := home.ClaimsPath()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "path":
		if c.bool("json") {
			envelope.EmitData(map[string]string{"path": path})
		} else {
			fmt.Println(path)
		}
		return nil
	case "init":
		if _, err := os.Stat(path); err == nil && !c.bool("force") {
			return envelope.Newf(envelope.CodeInvalidArgs, "%s already exists", path).WithHint("use --force to regenerate (your curated entries will be replaced)")
		}
		p, profPath, err := loadProfile()
		if err != nil {
			return err
		}
		full := resume.RenderText(resume.Build(p, nil, resume.Options{}))
		texts := []string{full}
		if extra := c.str("from"); extra != "" {
			raw, err := os.ReadFile(extra)
			if err != nil {
				return envelope.New(envelope.CodeIOFailed, err.Error())
			}
			texts = append(texts, string(raw))
		}
		f := &claims.File{
			Source:  fmt.Sprintf("bootstrapped from %s — CURATE THIS: verify every entry before trusting the gate", profPath),
			Updated: time.Now().Format("2006-01-02"),
			Allowed: claims.Bootstrap(texts...),
		}
		if err := claims.Save(path, f); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"path": path, "entries": len(f.Allowed)})
		} else {
			fmt.Printf("wrote %s with %d quantified-claim entries\n", path, len(f.Allowed))
			fmt.Println("review each entry against your fact ledger; remove anything you cannot defend")
		}
		return nil
	case "show":
		f, err := claims.Load(path)
		if err != nil {
			if os.IsNotExist(err) {
				return envelope.New(envelope.CodeNotFound, "no claims file").WithHint("run `jobkit claims init`")
			}
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(f)
			return nil
		}
		fmt.Printf("source: %s\nupdated: %s\nallowed (%d):\n", f.Source, f.Updated, len(f.Allowed))
		for _, a := range f.Allowed {
			fmt.Println("  - " + a)
		}
		return nil
	case "check":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit claims check <file|->")
		}
		text, err := readInput(c.args[2])
		if err != nil {
			return err
		}
		f, err := claims.Load(path)
		if err != nil {
			if os.IsNotExist(err) {
				return envelope.New(envelope.CodeNotFound, "no claims file").WithHint("run `jobkit claims init`")
			}
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		violations := claims.Check(text, f.Allowed)
		if c.bool("json") {
			envelope.EmitData(map[string]any{"clean": len(violations) == 0, "violations": violations})
			if len(violations) > 0 {
				return envelope.New(envelope.CodeInvalidArgs, "claims check failed")
			}
			return nil
		}
		if len(violations) == 0 {
			fmt.Println("clean: every quantified claim traces to the allowlist")
			return nil
		}
		return claimsViolationErr(violations)
	default:
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit claims init|check <file|->|show|path")
	}
}

// checkGeneratedClaims fail-closes resume/letter/outreach output on
// unverified quantified claims. No claims file = gate not configured = pass.
func checkGeneratedClaims(kind, content string) error {
	if kind != "resume" && kind != "letter" && kind != "outreach" {
		return nil
	}
	path, err := home.ClaimsPath()
	if err != nil {
		return nil
	}
	f, err := claims.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return envelope.New(envelope.CodeIOFailed, "claims file unreadable: "+err.Error())
	}
	if violations := claims.Check(content, f.Allowed); len(violations) > 0 {
		return claimsViolationErr(violations)
	}
	return nil
}

func claimsViolationErr(violations []claims.Violation) error {
	var lines []string
	for _, v := range violations {
		lines = append(lines, fmt.Sprintf("%s (…%s…)", v.Token, v.Context))
	}
	return envelope.Newf(envelope.CodeInvalidArgs, "claims gate: %d quantified claim(s) not in the allowlist:\n  %s",
		len(violations), strings.Join(lines, "\n  ")).
		WithHint("verify each claim, then add it to `jobkit claims path` — or fix the profile. The gate exists so nothing unverifiable ships in an application.")
}

// writeArtifact prints to stdout, or writes to --out (default name under
// ~/.jobkit/out when --out is given without a value... no: --out always
// explicit; "auto" picks a dated name).
func writeArtifact(c *cli, kind, ext, content string, meta map[string]any) error {
	if err := checkGeneratedClaims(kind, content); err != nil {
		return err
	}
	out, err := artifactPath(c, kind, ext)
	if err != nil {
		return err
	}
	if out == "" {
		if c.bool("json") {
			meta["content"] = content
			envelope.EmitData(meta)
			return nil
		}
		fmt.Print(content)
		return nil
	}
	if err := privatefs.WriteFile(out, []byte(content)); err != nil {
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

func artifactPath(c *cli, kind, ext string) (string, error) {
	out := c.str("out")
	if out == "auto" {
		dir, err := home.OutDir()
		if err != nil {
			return "", envelope.New(envelope.CodeIOFailed, err.Error())
		}
		out = filepath.Join(dir, fmt.Sprintf("%s-%s.%s", kind, time.Now().Format("2006-01-02-1504"), ext))
	}
	return out, nil
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

func enforceApplicationEligibility(c *cli, role, location string, remote bool, description string) (*eligibility.Result, error) {
	assessment, err := assessEligibility(role, location, remote, description)
	if err != nil {
		return nil, err
	}
	if assessment == nil {
		if !c.bool("allow-unassessed-eligibility") {
			return nil, envelope.New(envelope.CodeInvalidArgs, "application eligibility is unassessed because no policy is configured").
				WithHint("run `jobkit eligibility init --years N --home \"City, State\"`, or pass --allow-unassessed-eligibility only after human review")
		}
		return &eligibility.Result{
			Status: eligibility.Unassessed, Override: "allow-unassessed-eligibility",
			Reasons: []eligibility.Reason{{Code: "policy_missing", Category: "policy", Summary: "no eligibility policy was configured; human override recorded"}},
		}, nil
	}
	if assessment.Status == eligibility.Ineligible && !c.bool("override-eligibility") {
		var reasons []string
		for _, reason := range assessment.Reasons {
			reasons = append(reasons, reason.Summary)
		}
		return nil, envelope.Newf(envelope.CodeInvalidArgs, "eligibility gate blocked this role: %s", strings.Join(reasons, "; ")).
			WithHint("fix the posting metadata/policy, or pass --override-eligibility only after human review")
	}
	if assessment.Status == eligibility.Ineligible {
		assessment.Override = "override-eligibility"
	}
	return assessment, nil
}

func eligibilityJSON(assessment *eligibility.Result) string {
	if assessment == nil {
		return ""
	}
	payload, _ := json.MarshalIndent(assessment, "", "  ")
	return string(payload) + "\n"
}

func inboxPostingContext(id string) (string, bool, error) {
	if id == "" {
		return "", false, nil
	}
	ledger, err := openInboxLedger()
	if err != nil {
		return "", false, err
	}
	items, err := ledger.Replay()
	if err != nil {
		return "", false, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	item, err := inbox.Find(items, id)
	if err != nil {
		return "", false, envelope.New(envelope.CodeNotFound, err.Error())
	}
	return item.Job.Location, item.Job.Remote, nil
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
	assessment, err := enforceApplicationEligibility(c, role, c.str("location"), c.bool("remote"), text)
	if err != nil {
		return err
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
	if err := privatefs.EnsureDir(dir); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	format := firstNonEmpty(c.str("format"), "html")
	doc := resume.Build(p, j, resume.Options{MaxBulletsPerRole: 4})
	if err := checkGeneratedClaims("resume", resume.RenderText(doc)); err != nil {
		return err
	}
	var resumeOut, resumeName string
	switch format {
	case "md", "markdown":
		resumeName, resumeOut = "resume.md", resume.RenderMarkdown(doc)
	case "txt", "text", "ats":
		resumeName, resumeOut = "resume.txt", resume.RenderText(doc)
	case "html":
		resumeName, resumeOut = "resume.html", resume.RenderHTML(doc)
	case "pdf":
		resumeName = "resume.pdf"
		if err := resume.RenderPDF(resume.RenderHTML(doc), filepath.Join(dir, resumeName)); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown format %q (md|txt|html|pdf)", format)
	}
	letterOut := letter.Build(p, j, res, letter.Options{
		Company: company, Role: role, Tone: c.str("tone"), Manager: c.str("manager"),
	})
	if err := checkGeneratedClaims("letter", letterOut); err != nil {
		return err
	}
	prepOut := prep.Build(p, j, res)
	matchJSON, _ := json.MarshalIndent(res, "", "  ")

	files := map[string]string{
		"letter.txt": letterOut,
		"prep.md":    prepOut,
		"jd.txt":     text,
		"match.json": string(matchJSON) + "\n",
	}
	if assessment != nil {
		files["eligibility.json"] = eligibilityJSON(assessment)
	}
	if resumeOut != "" {
		files[resumeName] = resumeOut
	}
	for name, content := range files {
		if err := privatefs.WriteFile(filepath.Join(dir, name), []byte(content)); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	provenanceTags, err := generatedResumeTags(filepath.Join(dir, resumeName), id, assessment)
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	status := firstNonEmpty(c.str("status"), "interested")
	if !track.ValidStatus(status) {
		return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
	}
	url := c.str("url")
	if url == "" && jd.IsURL(src) {
		url = src
	}
	eligibilityNote := ""
	if assessment != nil {
		eligibilityNote = fmt.Sprintf(" · eligibility %s", assessment.Status)
		if assessment.Override != "" {
			eligibilityNote += " (human override: " + assessment.Override + ")"
		}
	}
	ev := track.Event{
		ID: id, Type: track.EvCreated, Company: company, Role: role, URL: url, Status: status,
		Note: fmt.Sprintf("match %.0f/100%s · artifacts: %s", res.Score, eligibilityNote, dir), Tags: provenanceTags,
	}
	if err := l.Append(ev); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	if c.bool("json") {
		envelope.EmitData(map[string]any{
			"id": id, "score": res.Score, "dir": dir, "status": status,
			"eligibility": assessment, "files": sortedKeys(files),
		})
		return nil
	}
	fmt.Printf("application package for %s — %s\n", company, role)
	fmt.Printf("  match score: %.0f/100\n  artifacts:   %s/\n", res.Score, dir)
	if assessment != nil {
		fmt.Printf("  eligibility: %s\n", assessment.Status)
	}
	fmt.Printf("    %s\n", strings.Join(sortedKeys(files), ", "))
	fmt.Printf("  tracked:     %s (%s)\n", id, status)
	fmt.Printf("next: review the letter, then `jobkit track set %s --status applied`\n", id)
	return nil
}

// cmdApplyPlan writes a human-in-loop application package and checklist. It
// does not submit anything; it creates reviewable artifacts, tracks the lead as
// interested, and marks an inbox source as planned when applicable.
func cmdApplyPlan(c *cli) error {
	p, _, err := loadProfile()
	if err != nil {
		return err
	}
	if len(c.args) < 2 {
		return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit apply-plan <jd-file|url|inbox-id|-> [--company X] [--role Y] [--tone T] [--format html|md|txt|pdf]")
	}
	src := c.args[1]
	text, company, role, sourceURL, inboxID, err := planSource(src)
	if err != nil {
		return err
	}
	j := jd.Parse(text)
	company = firstNonEmpty(c.str("company"), company, j.Company)
	role = firstNonEmpty(c.str("role"), role, j.Title)
	if company == "" || role == "" {
		return envelope.New(envelope.CodeInvalidArgs, "could not detect company/role from the JD").
			WithHint("pass --company and --role explicitly")
	}
	location := c.str("location")
	remote := c.bool("remote")
	if inboxID != "" && location == "" && !remote {
		inboxLocation, inboxRemote, err := inboxPostingContext(inboxID)
		if err != nil {
			return err
		}
		location, remote = inboxLocation, inboxRemote
	}
	assessment, err := enforceApplicationEligibility(c, role, location, remote, text)
	if err != nil {
		return err
	}
	res := match.Score(p, j)

	outRoot, err := home.OutDir()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	baseID := track.Slugify(company) + "--" + track.Slugify(role) + "--plan"
	dir := filepath.Join(outRoot, baseID)
	exists, err := pathExists(dir)
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if exists {
		dir = filepath.Join(outRoot, baseID+"-"+time.Now().Format("20060102-150405"))
	}
	if err := privatefs.EnsureDir(dir); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}

	format := firstNonEmpty(c.str("format"), "html")
	doc := resume.Build(p, j, resume.Options{MaxBulletsPerRole: 4, TailorOrder: true})
	if err := checkGeneratedClaims("resume", resume.RenderText(doc)); err != nil {
		return err
	}
	var resumeOut, resumeName string
	switch format {
	case "md", "markdown":
		resumeName, resumeOut, format = "resume.md", resume.RenderMarkdown(doc), "md"
	case "txt", "text", "ats":
		resumeName, resumeOut, format = "resume.txt", resume.RenderText(doc), "txt"
	case "html":
		resumeName, resumeOut = "resume.html", resume.RenderHTML(doc)
	case "pdf":
		resumeName = "resume.pdf"
		if err := resume.RenderPDF(resume.RenderHTML(doc), filepath.Join(dir, resumeName)); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown format %q (md|txt|html|pdf)", format)
	}
	letterOut := letter.Build(p, j, res, letter.Options{
		Company: company, Role: role, Tone: c.str("tone"), Manager: c.str("manager"),
	})
	if err := checkGeneratedClaims("letter", letterOut); err != nil {
		return err
	}
	prepOut := prep.Build(p, j, res)
	matchJSON, _ := json.MarshalIndent(res, "", "  ")
	outreachOut := renderOutreachDraft(p, company, role, sourceURL, res, c.str("contact"), firstNonEmpty(c.str("channel"), "email"))
	formJSON, err := renderFormPacketJSON(p, company, role, sourceURL, res)
	if err != nil {
		return envelope.New(envelope.CodeInternal, err.Error())
	}

	l, err := openLedger()
	if err != nil {
		return err
	}
	apps, err := l.Replay()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	trackID := track.NewID(apps, company, role)

	files := map[string]string{
		"plan.md":        renderPlan(company, role, sourceURL, trackID, inboxID, dir, res, assessment),
		"letter.txt":     letterOut,
		"outreach.txt":   outreachOut,
		"form-fill.json": string(formJSON) + "\n",
		"prep.md":        prepOut,
		"jd.txt":         text,
		"match.json":     string(matchJSON) + "\n",
	}
	if assessment != nil {
		files["eligibility.json"] = eligibilityJSON(assessment)
	}
	if resumeOut != "" {
		files[resumeName] = resumeOut
	}
	for name, content := range files {
		if err := privatefs.WriteFile(filepath.Join(dir, name), []byte(content)); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	provenanceTags, err := generatedResumeTags(filepath.Join(dir, resumeName), trackID, assessment)
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	eligibilityNote := ""
	if assessment != nil {
		eligibilityNote = fmt.Sprintf(" · eligibility %s", assessment.Status)
		if assessment.Override != "" {
			eligibilityNote += " (human override: " + assessment.Override + ")"
		}
	}
	if err := l.Append(track.Event{
		ID: trackID, Type: track.EvCreated, Company: company, Role: role, URL: sourceURL, Status: "interested",
		Note: fmt.Sprintf("apply-plan %.0f/100%s · artifacts: %s", res.Score, eligibilityNote, dir), Tags: provenanceTags,
	}); err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	if inboxID != "" {
		il, err := openInboxLedger()
		if err != nil {
			return err
		}
		if err := il.Append(inbox.Event{ID: inboxID, Type: inbox.EvStatus, Status: "planned", Note: "apply-plan artifacts: " + dir}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
	}
	fileNames := sortedKeys(files)
	if c.bool("json") {
		envelope.EmitData(map[string]any{
			"dir": dir, "track_id": trackID, "inbox_id": inboxID, "score": res.Score,
			"eligibility": assessment, "files": fileNames,
		})
		return nil
	}
	fmt.Printf("apply plan for %s — %s\n", company, role)
	fmt.Printf("  match score: %.0f/100\n  artifacts:   %s/\n", res.Score, dir)
	if assessment != nil {
		fmt.Printf("  eligibility: %s\n", assessment.Status)
	}
	fmt.Printf("  tracked:     %s (interested)\n", trackID)
	fmt.Println("next: open plan.md, review artifacts, submit manually, then mark applied")
	return nil
}

func planSource(src string) (text, company, role, sourceURL, inboxID string, err error) {
	if looksLikeInboxID(src) {
		il, ilErr := openInboxLedger()
		if ilErr != nil {
			return "", "", "", "", "", ilErr
		}
		items, replayErr := il.Replay()
		if replayErr != nil {
			return "", "", "", "", "", envelope.New(envelope.CodeIOFailed, replayErr.Error())
		}
		item, findErr := inbox.Find(items, src)
		if findErr != nil {
			return "", "", "", "", "", envelope.New(envelope.CodeNotFound, findErr.Error())
		}
		j := item.Job
		text = firstNonEmpty(j.JDText, inboxJDText(j), j.Description)
		return text, j.Company, j.Title, firstNonEmpty(j.ApplyURL, j.URL), item.ID, nil
	}
	text, err = readInput(src)
	if err != nil {
		return "", "", "", "", "", err
	}
	if jd.IsURL(src) {
		sourceURL = src
	}
	return text, "", "", sourceURL, "", nil
}

func looksLikeInboxID(src string) bool {
	src = strings.TrimSpace(src)
	if src == "" || src == "-" || jd.IsURL(src) {
		return false
	}
	if strings.ContainsAny(src, `/\`) || strings.Contains(src, ".") {
		return false
	}
	return strings.Contains(src, "--") || isHexPrefix(src)
}

func isHexPrefix(src string) bool {
	if len(src) < 8 {
		return false
	}
	for _, r := range src[:8] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
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

func renderPlan(company, role, sourceURL, trackID, inboxID, dir string, res *match.Result, assessment *eligibility.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Apply Plan: %s - %s\n\n", company, role)
	fmt.Fprintf(&b, "Match score: %.0f/100\n\n", res.Score)
	if assessment != nil {
		fmt.Fprintf(&b, "Eligibility: **%s** (%s, %s)\n\n", assessment.Status, assessment.RoleFamily, assessment.WorkMode)
		for _, reason := range assessment.Reasons {
			fmt.Fprintf(&b, "- Eligibility review: %s\n", reason.Summary)
		}
		if len(assessment.Reasons) > 0 {
			b.WriteString("\n")
		}
	}
	if sourceURL != "" {
		fmt.Fprintf(&b, "Posting: %s\n\n", sourceURL)
	}
	fmt.Fprintf(&b, "Artifacts: `%s`\n\n", dir)
	b.WriteString("## Human Checklist\n\n")
	b.WriteString("- Review `resume.*` for truthfulness, formatting, and role fit.\n")
	b.WriteString("- Review `letter.txt`; customize names, motivation, and any company-specific detail.\n")
	b.WriteString("- Review `prep.md` before any screen.\n")
	b.WriteString("- Submit manually in the browser; do not let jobkit auto-submit forms.\n")
	fmt.Fprintf(&b, "- After submission, run `jobkit track set %s --status applied`.\n", trackID)
	if inboxID != "" {
		fmt.Fprintf(&b, "- Then run `jobkit inbox set %s --status applied`.\n", inboxID)
	}
	b.WriteString("\n## Top Matched Skills\n\n")
	for i, m := range res.Matched {
		if i >= 8 {
			break
		}
		fmt.Fprintf(&b, "- %s (%.2f, %s)\n", m.Name, m.Weight, m.Evidence)
	}
	if len(res.Missing) > 0 {
		b.WriteString("\n## Gaps To Address Honestly\n\n")
		for i, m := range res.Missing {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "- %s (%.2f, %s)\n", m.Name, m.Weight, m.Category)
		}
	}
	return b.String()
}

// ---------- inbox ----------

func openInboxLedger() (*inbox.Ledger, error) {
	path, err := home.InboxPath()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return &inbox.Ledger{Path: path}, nil
}

type inboxSaveStats struct {
	New  int `json:"new"`
	Seen int `json:"seen"`
}

func saveJobsToInbox(jobs []jobsearch.Job, query, source string) (inboxSaveStats, error) {
	l, err := openInboxLedger()
	if err != nil {
		return inboxSaveStats{}, err
	}
	items, err := l.Replay()
	if err != nil {
		return inboxSaveStats{}, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.ID] = true
	}
	p, _ := tryLoadProfile()
	stats := inboxSaveStats{}
	for _, searchJob := range jobs {
		job := inbox.FromSearchJob(searchJob)
		id := inbox.NewID(job)
		if seen[id] {
			if err := l.Append(inbox.Event{ID: id, Type: inbox.EvSeen, Source: source, Query: query, Job: &job}); err != nil {
				return stats, envelope.New(envelope.CodeIOFailed, err.Error())
			}
			stats.Seen++
			continue
		}
		score := 0.0
		next := "profile-needed"
		if p != nil {
			score = match.Score(p, jd.Parse(job.JDText)).Score
			next = inbox.NextActionWithEligibility(score, job.Eligibility)
		} else if job.Eligibility != nil && job.Eligibility.Status != eligibility.Eligible {
			next = inbox.NextActionWithEligibility(score, job.Eligibility)
		}
		ev := inbox.Event{
			ID: id, Type: inbox.EvSaved, Status: "new", Source: source, Query: query,
			Job: &job, MatchScore: score, NextAction: next,
		}
		if err := l.Append(ev); err != nil {
			return stats, envelope.New(envelope.CodeIOFailed, err.Error())
		}
		seen[id] = true
		stats.New++
	}
	return stats, nil
}

func tryLoadProfile() (*profile.Profile, error) {
	p, _, err := loadProfile()
	if err != nil {
		return nil, err
	}
	return p, nil
}

func cmdInbox(c *cli) error {
	sub := ""
	if len(c.args) > 1 {
		sub = c.args[1]
	}
	l, err := openInboxLedger()
	if err != nil {
		return err
	}
	items, err := l.Replay()
	if err != nil {
		return envelope.New(envelope.CodeIOFailed, err.Error())
	}
	switch sub {
	case "add":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox add <jd-file|url|-> [--company X] [--role Y] [--source X]")
		}
		text, err := readInput(c.args[2])
		if err != nil {
			return err
		}
		j := jd.Parse(text)
		company := firstNonEmpty(c.str("company"), j.Company)
		role := firstNonEmpty(c.str("role"), j.Title)
		if company == "" || role == "" {
			return envelope.New(envelope.CodeInvalidArgs, "could not detect company/role").WithHint("pass --company and --role")
		}
		assessment, err := assessEligibility(role, c.str("location"), c.bool("remote"), text)
		if err != nil {
			return err
		}
		job := inbox.Job{
			Title: role, Company: company, URL: c.args[2], Description: text, JDText: text,
			Location: c.str("location"), Remote: c.bool("remote"), Eligibility: assessment,
		}
		if !jd.IsURL(c.args[2]) {
			job.URL = c.str("url")
		}
		id := inbox.NewID(job)
		if inbox.Has(items, id) {
			return envelope.Newf(envelope.CodeInvalidArgs, "inbox already contains %s", id)
		}
		p, _ := tryLoadProfile()
		score := 0.0
		next := "profile-needed"
		if p != nil {
			score = match.Score(p, j).Score
			next = inbox.NextActionWithEligibility(score, assessment)
		} else if assessment != nil && assessment.Status != eligibility.Eligible {
			next = inbox.NextActionWithEligibility(score, assessment)
		}
		if err := l.Append(inbox.Event{ID: id, Type: inbox.EvSaved, Status: "new", Source: firstNonEmpty(c.str("source"), "manual"), Job: &job, MatchScore: score, NextAction: next}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"id": id, "score": score, "next_action": next, "eligibility": assessment})
		} else {
			fmt.Printf("saved %s (%.0f/100, %s)\n", id, score, next)
		}
		return nil
	case "recheck":
		policy, err := activeEligibilityPolicy()
		if err != nil {
			return err
		}
		if policy == nil {
			return envelope.New(envelope.CodeNotFound, "no eligibility policy configured").
				WithHint("run `jobkit eligibility init --years N --home \"City, State\"`")
		}
		counts := map[eligibility.Status]int{}
		updated := 0
		for _, item := range items {
			if !c.bool("all") && inbox.TerminalStatuses[item.Status] {
				continue
			}
			assessment := eligibility.Evaluate(eligibility.Posting{
				Title: item.Job.Title, Location: item.Job.Location, Remote: item.Job.Remote,
				Description: firstNonEmpty(item.Job.Description, item.Job.JDText),
			}, *policy)
			job := item.Job
			job.Eligibility = &assessment
			next := inbox.NextActionWithEligibility(item.MatchScore, &assessment)
			if err := l.Append(inbox.Event{ID: item.ID, Type: inbox.EvAssessed, Job: &job, NextAction: next}); err != nil {
				return envelope.New(envelope.CodeIOFailed, err.Error())
			}
			counts[assessment.Status]++
			updated++
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"updated": updated, "counts": counts})
		} else {
			fmt.Printf("rechecked %d inbox job(s): %d eligible, %d review, %d ineligible\n",
				updated, counts[eligibility.Eligible], counts[eligibility.Review], counts[eligibility.Ineligible])
		}
		return nil
	case "slate":
		policy := inbox.DefaultSlatePolicy()
		for _, option := range []struct {
			flag   string
			target *int
		}{
			{"platform", &policy.Platform}, {"fullstack", &policy.Fullstack},
			{"adoption", &policy.Adoption}, {"stretch", &policy.Stretch},
			{"employer-cap", &policy.EmployerCap},
		} {
			flag, target := option.flag, option.target
			if _, ok := c.flags[flag]; !ok {
				continue
			}
			value, err := c.int(flag, *target)
			if err != nil {
				return err
			}
			if value < 0 {
				return envelope.Newf(envelope.CodeInvalidArgs, "--%s must not be negative", flag)
			}
			*target = value
		}
		slate := inbox.BuildSlate(items, policy)
		if c.bool("json") {
			envelope.EmitData(slate)
			return nil
		}
		if c.str("out") != "" {
			out, err := artifactPath(c, "weekly-slate", "md")
			if err != nil {
				return err
			}
			if err := privatefs.WriteFile(out, []byte(renderWeeklySlate(slate))); err != nil {
				return envelope.New(envelope.CodeIOFailed, err.Error())
			}
			fmt.Printf("wrote %s\n", out)
			return nil
		}
		fmt.Print(renderWeeklySlate(slate))
		return nil
	case "list", "":
		filtered := items
		if st := c.str("status"); st != "" {
			filtered = nil
			for _, item := range items {
				if item.Status == st {
					filtered = append(filtered, item)
				}
			}
		} else if !c.bool("all") {
			filtered = nil
			for _, item := range items {
				if !inbox.TerminalStatuses[item.Status] {
					filtered = append(filtered, item)
				}
			}
		}
		if filter := strings.ToLower(strings.TrimSpace(c.str("eligibility"))); filter != "" {
			if !eligibility.ValidFilter(filter) {
				return envelope.Newf(envelope.CodeInvalidArgs, "unknown eligibility filter %q", filter)
			}
			var byEligibility []*inbox.Item
			for _, item := range filtered {
				if item.Job.Eligibility == nil {
					if filter == "all" || filter == "actionable" || filter == string(eligibility.Review) {
						byEligibility = append(byEligibility, item)
					}
					continue
				}
				if eligibility.Allows(filter, item.Job.Eligibility.Status) {
					byEligibility = append(byEligibility, item)
				}
			}
			filtered = byEligibility
		}
		if filtered == nil {
			filtered = []*inbox.Item{}
		}
		if c.bool("json") {
			envelope.EmitData(filtered)
			return nil
		}
		if len(filtered) == 0 {
			fmt.Println("no inbox jobs (use `jobkit find ... --inbox` or `jobkit inbox add ...`)")
			return nil
		}
		for _, item := range filtered {
			seen := ""
			if !item.LastSeenAt.IsZero() {
				seen = item.LastSeenAt.Format("Jan 02")
			}
			eligibilityLabel := "unassessed"
			if item.Job.Eligibility != nil {
				eligibilityLabel = string(item.Job.Eligibility.Status)
			}
			fmt.Printf("%-44s %-11s %-10s %3.0f  %-18s %-7s %s — %s\n", item.ID, item.Status, eligibilityLabel, item.MatchScore, item.NextAction, seen, item.Job.Company, item.Job.Title)
		}
		return nil
	case "stale":
		days, err := c.int("days", 14)
		if err != nil {
			return err
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		var stale []*inbox.Item
		for _, item := range items {
			if inbox.TerminalStatuses[item.Status] {
				continue
			}
			seenAt := itemSeenAt(item)
			if !seenAt.IsZero() && seenAt.Before(cutoff) {
				stale = append(stale, item)
			}
		}
		if stale == nil {
			stale = []*inbox.Item{}
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"days": days, "items": stale})
			return nil
		}
		if len(stale) == 0 {
			fmt.Printf("no stale inbox jobs older than %d day(s)\n", days)
			return nil
		}
		for _, item := range stale {
			fmt.Printf("%-44s %-11s last-seen %-10s %s — %s\n", item.ID, item.Status, shortTime(itemSeenAt(item)), item.Job.Company, item.Job.Title)
		}
		return nil
	case "show":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox show <id>")
		}
		item, err := inbox.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(item)
			return nil
		}
		fmt.Printf("%s\n  %s — %s\n  status: %s\n  score: %.0f/100\n  next: %s\n", item.ID, item.Job.Company, item.Job.Title, item.Status, item.MatchScore, item.NextAction)
		if item.Source != "" || item.Query != "" {
			fmt.Printf("  source: %s  query: %s\n", orDash(item.Source), orDash(item.Query))
		}
		if !item.CreatedAt.IsZero() || !item.LastSeenAt.IsZero() {
			fmt.Printf("  first seen: %s  last seen: %s  seen: %d\n", shortTime(item.CreatedAt), shortTime(item.LastSeenAt), item.SeenCount)
		}
		if item.Job.Provider != "" || item.Job.Board != "" || item.Job.Fingerprint != "" {
			fmt.Printf("  board: %s:%s  fingerprint: %s\n", orDash(item.Job.Provider), orDash(item.Job.Board), orDash(item.Job.Fingerprint))
		}
		if item.Job.Eligibility != nil {
			fmt.Printf("  eligibility: %s  family: %s  mode: %s\n", item.Job.Eligibility.Status, item.Job.Eligibility.RoleFamily, item.Job.Eligibility.WorkMode)
			for _, reason := range item.Job.Eligibility.Reasons {
				fmt.Printf("    %s: %s\n", reason.Code, reason.Summary)
			}
		}
		if u := firstNonEmpty(item.Job.ApplyURL, item.Job.URL); u != "" {
			fmt.Printf("  url: %s\n", u)
		}
		return nil
	case "outreach":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox outreach <id> [--contact NAME] [--channel email|linkedin] [--out PATH|auto]")
		}
		item, err := inbox.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		p, _, err := loadProfile()
		if err != nil {
			return err
		}
		j := jd.Parse(inboxJDText(item.Job))
		res := match.Score(p, j)
		content := renderOutreachDraft(p, item.Job.Company, item.Job.Title, firstNonEmpty(item.Job.ApplyURL, item.Job.URL), res, c.str("contact"), firstNonEmpty(c.str("channel"), "email"))
		return writeArtifact(c, "outreach", "txt", content, map[string]any{"id": item.ID, "company": item.Job.Company, "role": item.Job.Title})
	case "form":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox form <id> [--format json|js] [--out PATH|auto]").WithHint("--format js emits a paste-in-DevTools autofill snippet (fills, highlights, never submits)")
		}
		item, err := inbox.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		p, _, err := loadProfile()
		if err != nil {
			return err
		}
		if c.str("format") == "js" {
			data := formfill.Data{FullName: p.Name, Email: p.Email, Phone: p.Phone, Location: p.Location}
			links := map[string]string{}
			for _, link := range p.Links {
				links[link.Label] = link.URL
			}
			data.PickLinks(links)
			if len(p.Experience) > 0 {
				data.Company = p.Experience[0].Company
			}
			js, err := formfill.Snippet(data)
			if err != nil {
				return envelope.New(envelope.CodeInternal, err.Error())
			}
			return writeArtifact(c, "form-fill", "js", js, map[string]any{"id": item.ID, "company": item.Job.Company, "role": item.Job.Title, "apply_url": firstNonEmpty(item.Job.ApplyURL, item.Job.URL)})
		}
		j := jd.Parse(inboxJDText(item.Job))
		res := match.Score(p, j)
		raw, err := renderFormPacketJSON(p, item.Job.Company, item.Job.Title, firstNonEmpty(item.Job.ApplyURL, item.Job.URL), res)
		if err != nil {
			return envelope.New(envelope.CodeInternal, err.Error())
		}
		return writeArtifact(c, "form-fill", "json", string(raw)+"\n", map[string]any{"id": item.ID, "company": item.Job.Company, "role": item.Job.Title})
	case "set":
		if len(c.args) < 3 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox set <id> --status S [--note N]")
		}
		item, err := inbox.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		status := c.str("status")
		if status == "" {
			return envelope.New(envelope.CodeInvalidArgs, "--status is required").WithHint("statuses: " + strings.Join(inbox.Statuses, ", "))
		}
		if !inbox.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(inbox.Statuses, ", "))
		}
		if err := l.Append(inbox.Event{ID: item.ID, Type: inbox.EvStatus, Status: status, Note: c.str("note")}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": item.ID, "status": status})
		} else {
			fmt.Printf("%s → %s\n", item.ID, status)
		}
		return nil
	case "note":
		if len(c.args) < 4 {
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit inbox note <id> <text>")
		}
		item, err := inbox.Find(items, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		note := strings.Join(c.args[3:], " ")
		if err := l.Append(inbox.Event{ID: item.ID, Type: inbox.EvNote, Note: note}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]string{"id": item.ID, "note": note})
		} else {
			fmt.Printf("noted on %s\n", item.ID)
		}
		return nil
	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown inbox subcommand %q", sub).WithHint("add|recheck|slate|list|show|stale|set|note|outreach|form")
	}
}

func renderWeeklySlate(slate inbox.Slate) string {
	var b strings.Builder
	b.WriteString("# Weekly Application Slate\n\n")
	fmt.Fprintf(&b, "Mix: %d platform/DevEx/AI infrastructure, %d full-stack product, %d technical adoption/FDE, %d stretch; max %d per employer.\n\n",
		slate.Policy.Platform, slate.Policy.Fullstack, slate.Policy.Adoption, slate.Policy.Stretch, slate.Policy.EmployerCap)
	if len(slate.Selections) == 0 {
		b.WriteString("No eligible, assessed inbox jobs currently fill the slate.\n")
	} else {
		currentLane := ""
		for _, selected := range slate.Selections {
			if selected.Lane != currentLane {
				if currentLane != "" {
					b.WriteString("\n")
				}
				currentLane = selected.Lane
				fmt.Fprintf(&b, "## %s\n\n", currentLane)
			}
			fmt.Fprintf(&b, "- `%s` — %s, %s (fit %.0f, opportunity %d, eligibility %s)",
				selected.ID, selected.Company, selected.Title, selected.MatchScore, selected.Opportunity, selected.Eligibility.Status)
			if selected.URL != "" {
				fmt.Fprintf(&b, " — %s", selected.URL)
			}
			b.WriteString("\n")
		}
	}
	if len(slate.Warnings) > 0 {
		b.WriteString("\n## Gaps\n\n")
		for _, warning := range slate.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	b.WriteString("\nHuman gate: review each posting and package; JobKit does not submit applications or send outreach.\n")
	return b.String()
}

func itemSeenAt(item *inbox.Item) time.Time {
	if item.LastSeenAt.IsZero() {
		return item.CreatedAt
	}
	return item.LastSeenAt
}

func renderOutreachDraft(p *profile.Profile, company, role, sourceURL string, res *match.Result, contact, channel string) string {
	if strings.TrimSpace(contact) == "" {
		contact = "there"
	}
	var b strings.Builder
	if channel == "linkedin" {
		fmt.Fprintf(&b, "Hi %s - I saw the %s role at %s and it lines up with my background in %s.\n\n", contact, role, company, topSkillNames(res, 3))
		fmt.Fprintf(&b, "A quick example: %s\n\n", bestEvidenceLine(p, res))
		b.WriteString("I would be glad to compare notes or route a concise application package if helpful.")
		if sourceURL != "" {
			fmt.Fprintf(&b, "\n\nPosting: %s", sourceURL)
		}
		return b.String() + "\n"
	}
	fmt.Fprintf(&b, "Subject: %s - %s\n\n", company, role)
	fmt.Fprintf(&b, "Hi %s,\n\n", contact)
	fmt.Fprintf(&b, "I saw the %s role at %s and wanted to reach out directly. The role maps closely to my work in %s.\n\n", role, company, topSkillNames(res, 3))
	fmt.Fprintf(&b, "Relevant example: %s\n\n", bestEvidenceLine(p, res))
	b.WriteString("If this looks useful, I can send the full application package or apply through the normal process.\n\n")
	b.WriteString("Best,\n")
	b.WriteString(p.Name)
	if p.Email != "" {
		fmt.Fprintf(&b, "\n%s", p.Email)
	}
	if sourceURL != "" {
		fmt.Fprintf(&b, "\n\nPosting: %s", sourceURL)
	}
	return b.String() + "\n"
}

func renderFormPacketJSON(p *profile.Profile, company, role, sourceURL string, res *match.Result) ([]byte, error) {
	packet := map[string]any{
		"candidate": map[string]string{
			"name":     p.Name,
			"headline": p.Headline,
			"email":    p.Email,
			"phone":    p.Phone,
			"location": p.Location,
		},
		"target": map[string]string{
			"company": company,
			"role":    role,
			"url":     sourceURL,
		},
		"links":              p.Links,
		"summary":            "Review before pasting: " + firstNonEmpty(p.Summary, p.Headline),
		"top_skills":         topSkillNameList(res, 8),
		"work_authorization": "REVIEW_REQUIRED",
		"sponsorship":        "REVIEW_REQUIRED",
		"salary_expectation": "REVIEW_REQUIRED",
		"availability":       "REVIEW_REQUIRED",
		"human_checklist": []string{
			"Verify every answer before pasting into a form.",
			"Never submit automatically.",
			"Use the tailored resume and letter from the same apply-plan directory.",
		},
	}
	return json.MarshalIndent(packet, "", "  ")
}

func topSkillNames(res *match.Result, n int) string {
	return strings.Join(topSkillNameList(res, n), ", ")
}

func topSkillNameList(res *match.Result, n int) []string {
	if res == nil || len(res.Matched) == 0 {
		return []string{"the requested skills"}
	}
	var names []string
	for i, skill := range res.Matched {
		if i >= n {
			break
		}
		names = append(names, skill.Name)
	}
	return names
}

func bestEvidenceLine(p *profile.Profile, res *match.Result) string {
	if p == nil {
		return "I have shipped similar production work end to end."
	}
	if res != nil {
		for _, skill := range res.Matched {
			for _, exp := range p.Experience {
				for _, bullet := range exp.Bullets {
					if strings.Contains(strings.ToLower(bullet.Text), strings.ToLower(skill.Name)) {
						return bullet.Text
					}
				}
			}
		}
	}
	for _, exp := range p.Experience {
		if len(exp.Bullets) > 0 {
			return exp.Bullets[0].Text
		}
	}
	return firstNonEmpty(p.Summary, "I have shipped similar production work end to end.")
}

func inboxJDText(j inbox.Job) string {
	if text := strings.TrimSpace(j.JDText); text != "" {
		return text + "\n"
	}
	var b strings.Builder
	if j.Title != "" {
		fmt.Fprintf(&b, "Title: %s\n", j.Title)
	}
	if j.Company != "" {
		fmt.Fprintf(&b, "Company: %s\n", j.Company)
	}
	b.WriteString("\n")
	b.WriteString(j.Description)
	return strings.TrimSpace(b.String()) + "\n"
}

func generatedResumeTags(path, receiptID string, assessment *eligibility.Result) (map[string]string, error) {
	digest, err := sha256File(path)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if kind == "txt" {
		kind = "ats"
	}
	tags := map[string]string{
		track.TagResumeVariantID:      "jobkit-tailored",
		track.TagResumeArtifactKind:   kind,
		track.TagResumeArtifactDigest: digest,
		track.TagTailoringReceiptID:   "jobkit:" + receiptID,
	}
	if assessment != nil && assessment.Override != "" {
		tags[track.TagEligibilityOverride] = assessment.Override
	}
	claimsPath, err := home.ClaimsPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(claimsPath); err == nil {
		claimDigest, err := sha256File(claimsPath)
		if err != nil {
			return nil, err
		}
		tags[track.TagClaimSetDigest] = claimDigest
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return tags, nil
}

func sha256File(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ---------- track ----------

func openLedger() (*track.Ledger, error) {
	path, err := home.LedgerPath()
	if err != nil {
		return nil, envelope.New(envelope.CodeIOFailed, err.Error())
	}
	return &track.Ledger{Path: path}, nil
}

// trackTagsFromFlags merges a verified nicos-resume package manifest,
// first-class provenance flags, and generic --tag values. Conflicting values
// fail closed instead of silently relabeling an application artifact.
func trackTagsFromFlags(c *cli) (map[string]string, error) {
	tags := map[string]string{}
	put := func(key, value, source string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if existing, ok := tags[key]; ok && existing != value {
			return envelope.Newf(envelope.CodeInvalidArgs, "%s conflicts with %s=%q", source, key, existing)
		}
		tags[key] = value
		return nil
	}
	if spec := c.str("tag"); spec != "" {
		parsed, err := track.ParseTagSpec(spec)
		if err != nil {
			return nil, envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		for k, v := range parsed {
			tags[k] = v
		}
	}
	manifestPath := c.str("resume-manifest")
	artifactKind := c.str("resume-artifact")
	artifactFile := c.str("resume-artifact-file")
	if manifestPath == "" && (artifactKind != "" || artifactFile != "") {
		return nil, envelope.New(envelope.CodeInvalidArgs, "--resume-artifact and --resume-artifact-file require --resume-manifest")
	}
	if manifestPath != "" {
		manifestTags, err := track.ResumeManifestTags(manifestPath, artifactKind, artifactFile)
		if err != nil {
			return nil, envelope.New(envelope.CodeInvalidArgs, err.Error())
		}
		for key, value := range manifestTags {
			if err := put(key, value, "--resume-manifest"); err != nil {
				return nil, err
			}
		}
	}
	for _, item := range []struct {
		flag string
		tag  string
	}{
		{"resume-version", track.TagResumeVersion},
		{"resume-variant-id", track.TagResumeVariantID},
		{"resume-artifact-digest", track.TagResumeArtifactDigest},
		{"claim-set-version", track.TagClaimSetVersion},
		{"tailoring-receipt-id", track.TagTailoringReceiptID},
		{"eligibility-override", track.TagEligibilityOverride},
		{"lane", track.TagLane},
		{"source", track.TagSource},
	} {
		if err := put(item.tag, c.str(item.flag), "--"+item.flag); err != nil {
			return nil, err
		}
	}
	if digest := tags[track.TagResumeArtifactDigest]; digest != "" && !track.ValidSHA256Digest(digest) {
		return nil, envelope.Newf(envelope.CodeInvalidArgs, "--resume-artifact-digest must be sha256:<64 lowercase hex>, got %q", digest)
	}
	if len(tags) == 0 {
		return nil, nil
	}
	return tags, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
			return envelope.New(envelope.CodeInvalidArgs, `usage: jobkit track add <company> <role> [--url U] [--status S] [--note N] [--resume-manifest PATH] [--resume-artifact pdf|docx|ats] [--resume-artifact-file PATH] [--resume-version V] [--lane L] [--source cold|referral|inbound] [--tag k=v,...]`)
		}
		company, role := c.args[2], c.args[3]
		status := c.str("status")
		if status == "" {
			status = "interested"
		}
		if !track.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
		}
		tags, err := trackTagsFromFlags(c)
		if err != nil {
			return err
		}
		id := track.NewID(apps, company, role)
		ev := track.Event{ID: id, Type: track.EvCreated, Company: company, Role: role, URL: c.str("url"), Status: status, Note: c.str("note"), Tags: tags}
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
		if len(a.Tags) > 0 {
			var pairs []string
			for _, k := range sortedKeys(a.Tags) {
				pairs = append(pairs, k+"="+a.Tags[k])
			}
			fmt.Printf("  tags: %s\n", strings.Join(pairs, " "))
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
			return envelope.New(envelope.CodeInvalidArgs, "usage: jobkit track set <id> [--status S] [--note N] [--resume-manifest PATH] [--resume-artifact pdf|docx|ats] [--resume-artifact-file PATH] [--resume-version V] [--lane L] [--source S] [--tag k=v,...]")
		}
		a, err := track.Find(apps, c.args[2])
		if err != nil {
			return envelope.New(envelope.CodeNotFound, err.Error())
		}
		status := c.str("status")
		tags, err := trackTagsFromFlags(c)
		if err != nil {
			return err
		}
		if status == "" && len(tags) == 0 {
			return envelope.New(envelope.CodeInvalidArgs, "--status or a tag flag is required").WithHint("statuses: " + strings.Join(track.Statuses, ", "))
		}
		if status != "" && !track.ValidStatus(status) {
			return envelope.Newf(envelope.CodeInvalidArgs, "invalid status %q (one of: %s)", status, strings.Join(track.Statuses, ", "))
		}
		evType := track.EvStatus
		if status == "" {
			evType = track.EvTagged
		}
		if err := l.Append(track.Event{ID: a.ID, Type: evType, Status: status, Note: c.str("note"), Tags: tags}); err != nil {
			return envelope.New(envelope.CodeIOFailed, err.Error())
		}
		if c.bool("json") {
			envelope.EmitData(map[string]any{"id": a.ID, "status": status, "tags": tags})
		} else if status != "" {
			fmt.Printf("%s → %s\n", a.ID, status)
		} else {
			fmt.Printf("%s tagged\n", a.ID)
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
		if len(s.ByTag) > 0 {
			// Canonical keys first, then any custom keys alphabetically.
			ordered := []string{track.TagLane, track.TagResumeVersion, track.TagResumeVariantID, track.TagClaimSetVersion, track.TagEligibilityOverride, track.TagSource}
			seen := map[string]bool{}
			for _, key := range ordered {
				seen[key] = true
			}
			var rest []string
			for k := range s.ByTag {
				if !seen[k] {
					rest = append(rest, k)
				}
			}
			sort.Strings(rest)
			for _, key := range append(ordered, rest...) {
				byVal := s.ByTag[key]
				if len(byVal) == 0 {
					continue
				}
				fmt.Printf("by %s:\n", key)
				for _, v := range sortedKeys(byVal) {
					row := byVal[v]
					fmt.Printf("  %-24s applied %-3d responded %-3d interviews %-3d response rate %.0f%%\n",
						v, row.Applied, row.Responded, row.Interviews, row.ResponseRate*100)
				}
			}
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

	case "remind":
		days, err := c.int("days", 7)
		if err != nil {
			return err
		}
		now := time.Now()
		reminders := track.BuildReminders(apps, days, now)
		format := firstNonEmpty(c.str("format"), "text")
		switch format {
		case "json":
			envelope.EmitData(reminders)
			return nil
		case "text", "txt":
			return writeArtifact(c, "followups", "txt", track.RenderRemindersText(reminders), map[string]any{
				"count": len(reminders), "format": "text",
			})
		case "ics":
			return writeArtifact(c, "followups", "ics", track.RenderRemindersICS(reminders, now), map[string]any{
				"count": len(reminders), "format": "ics",
			})
		default:
			return envelope.Newf(envelope.CodeInvalidArgs, "unknown reminder format %q (text|ics|json)", format)
		}

	default:
		return envelope.Newf(envelope.CodeInvalidArgs, "unknown track subcommand %q", sub).WithHint("add|list|show|set|note|board|stats|followups|remind")
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

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func containsFoldString(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func shortTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

// usageCompact is intentionally short for agent context budgets (~1/6 of full help).
const usageCompact = `jobkit ` + version + ` — offline-first job toolkit (agent-friendly)

USAGE
  jobkit <cmd> [args] [--json] [--compact]

CORE
  init | profile show|validate|path|bootstrap
  eligibility init|show|check|path
  doctor permissions [--fix-permissions]
  find <q> --boards|--targets … [--eligibility actionable] [--inbox] [--strict]
  search init|list|show|run|digest
  match <jd> | apply <jd> | apply-plan <jd|inbox-id>
  resume build [jd] | letter build <jd> | prep <jd>
  coach source|deck|run|stats|serve|path
  claims init|check|show|path
  inbox add|recheck|slate|list|show|stale|set|note|outreach|form
  track add|list|show|set|note|board|stats|followups|remind
  company add|signal|list|show | contact add|list|import|…
  calibrate report|apply | jd parse|fetch | version | help

GLOBAL
  --json     {ok,data}|{ok:false,error:{code,message,hint}}  exits 0/1/2/3
  --compact  short help (this text); full help is default
  JOBKIT_HOME  state dir (default ~/.jobkit) — never published

JD args accept file | URL | - (stdin) | inbox id (apply-plan).
Run jobkit help for the full verb reference.
`

const usage = `jobkit ` + version + ` — job application & resume toolkit

USAGE
  jobkit <command> [args] [--json] [--compact]

PROFILE
  init                              create the starter profile (~/.jobkit/profile.yaml)
  profile show|validate|path        inspect the master profile
  profile bootstrap --source resume.pdf [--out path|auto] [--force]
                                    draft a profile from a trusted resume source

SAVED SEARCHES
  search init|path|list             manage ~/.jobkit/searches.yaml board groups and target packs
  search show <name>                inspect one saved search profile
  search run <name> [--inbox] [--strict]
                                    run a saved search, optionally queue results
  search digest [name] [--inbox] [--out PATH|auto] [--strict]
                                    markdown/JSON digest across saved searches
  calibrate report [--persona NAME] inspect ranking accuracy from inbox/tracker outcomes
  calibrate apply [--persona NAME] [--min-samples N] [--force]
                                    write ~/.jobkit/calibration.yaml weights used by find/search

ELIGIBILITY (hard constraints, separate from fit and opportunity)
  eligibility init [--home "City, State"] [--countries US,...]
              [--languages English,...] [--years N] [--relocation-open] [--force]
                                    create ~/.jobkit/eligibility.yaml
  eligibility show|path             inspect the active constraint policy
  eligibility check <jd> [--role X] [--location X] [--remote]
                                    classify eligible|review|ineligible with reasons

HIDDEN MARKET
  company add <name> [--domain D] [--stage S] [--tags a,b]
              [--boards provider:slug] [--target-comp N]
                                    track target companies, ATS boards, and
                                    compensation goals in ~/.jobkit/companies.yaml
  company signal <name> --type funding|launch|team-growth|recruiter|backchannel
              [--source X] [--url U] [--note N] [--weight N]
                                    rank companies by fresh hiring signals before
                                    public postings saturate
  company list [--tag T] [--stage S] show hidden-market targets by action score
  company show <name>

CONTACTS / REFERRALS (append-only ledger: ~/.jobkit/contacts.jsonl)
  contact add <name> [--company X] [--role X] [--channel linkedin|email]
              [--url U] [--email E] [--source X] [--inbox-id ID] [--track-id ID]
                                    add a relationship tied to an inbox item or app
  contact list [--company X] [--status S] [--inbox-id ID] [--track-id ID]
  contact show <id>
  contact touch <id> [--status S] [--note N]
  contact referral <id> [--status referral-requested|referral-offered|referred]
              [--note N] [--inbox-id ID] [--track-id ID]
  contact note <id> <text>
  contact import <connections.csv> [--source X]
                                    bulk-load a LinkedIn connections export;
                                    dedupes and reports target-company overlap

JOB DESCRIPTIONS (every <jd> accepts a file, a URL, or - for stdin)
  jd parse <jd>                     extract skills/seniority from a JD
  jd fetch <url> [--out PATH]       download a posting as clean text
  find <query> --boards greenhouse:acme,lever:demo,ashby:Org
                [--targets ai-infra] [--remote] [--location X] [--limit N]
                [--sort opportunity|comp|freshness] [--persona agent-infra]
                [--min-comp N] [--eligibility actionable|eligible|review|ineligible|all]
                [--save NAME] [--inbox] [--strict]
                                    search public company board APIs; board specs
                                    may include @groups or #target-packs.
                                    Board fetch failures become warnings unless
                                    --strict is set. Results include compensation,
                                    freshness, saturation, and persona scores.
  match <jd>                        score your profile against a JD + gap report

ARTIFACTS
  apply-plan <jd|url|inbox-id> [--company X] [--role Y] [--tone T]
              [--location X] [--remote] [--override-eligibility]
              [--allow-unassessed-eligibility]
                                    human-in-loop package + checklist; no submit;
                                    ineligible roles fail closed unless reviewed
  apply <jd> [--company X] [--role Y] [--tone T] [--format html|md|txt|pdf]
              [--location X] [--remote] [--override-eligibility]
              [--allow-unassessed-eligibility]
                                    golden path: resume + letter + prep sheet +
                                    match report into ~/.jobkit/out/<id>/ + tracked
  resume build [jd] [--format md|txt|html|pdf] [--out PATH|auto] [--max-bullets N] [--full]
                                    tailored resume (no JD or --full = complete resume)
  letter build <jd> [--company X] [--role Y] [--tone professional|warm|direct]
                [--manager NAME] [--out PATH|auto]
  prep <jd> [--out PATH|auto]       interview-prep sheet: deep-dives, gap defense,
                                    STAR story bank, questions to ask

INTERVIEW COACH (private state under ~/.jobkit/coach)
  coach source import <public-bundle.json>
                                    validate and store public-safe project,
                                    story, claim, and evidence cards
  coach source show|path            inspect the source digest or source path
  coach deck --job <inbox-id|jd-file|url>
              [--mode project|behavioral|system-design|claim-defense|mixed]
              [--minutes N] [--projects id,id] [--out PATH|auto]
                                    create and save a deterministic practice deck
  coach run <deck-id|latest> [--answers FILE] [--provider NAME]
              [--provider-config FILE] [--useful yes|no]
                                    score answers and append one local session;
                                    model feedback is advisory and fail-open
  coach stats [--project ID]        show scores, claim violations, and due reviews
  coach serve [--addr 127.0.0.1:7331] [--provider-config FILE]
                                    start the localhost text and local-voice UI;
                                    non-loopback addresses fail closed
  coach path                        print the private coach state directory

CLAIMS GATE (fact lock for generated material: ~/.jobkit/claims.yaml)
  claims init [--from FILE] [--force]
                                    bootstrap the allowlist of verified
                                    quantified claims from your profile
  claims check <file|->             verify any text against the allowlist
  claims show|path                  inspect the allowlist
                                    when claims.yaml exists, resume/letter/apply
                                    generation fails closed on any quantified
                                    claim not in the allowlist

INBOX (deduped pre-application queue: ~/.jobkit/inbox.jsonl)
  inbox add <jd|url|-> [--company X] [--role Y] [--location X] [--remote] [--source X]
  inbox recheck [--all]             reassess stored jobs against the current policy
  inbox slate [--platform 5] [--fullstack 3] [--adoption 1] [--stretch 1]
              [--employer-cap 2] [--out PATH|auto]
                                    deterministic weekly mix; assessed actionable jobs only
  inbox list [--status S] [--eligibility FILTER] [--all]
                                    saved jobs, constraint status, fit, next actions
  inbox stale [--days N]            active saved jobs not seen recently
  inbox show <id>
  inbox set <id> --status S         move through new/shortlisted/planned/applied
  inbox note <id> <text>
  inbox outreach <id> [--contact N] [--channel email|linkedin] [--out PATH|auto]
  inbox form <id> [--format json|js] [--out PATH|auto]
                                    human-reviewed form-fill packet; --format js
                                    emits a paste-in-DevTools autofill snippet
                                    (fills + highlights fields, never submits)

TRACKER (append-only ledger: ~/.jobkit/applications.jsonl)
  track add <company> <role> [--url U] [--status S] [--note N]
              [--resume-version V] [--lane L] [--source cold|referral|inbound]
              [--resume-manifest PATH] [--resume-artifact pdf|docx|ats] [--resume-artifact-file PATH]
              [--resume-variant-id ID] [--resume-artifact-digest sha256:HEX]
              [--claim-set-version V] [--tailoring-receipt-id ID]
              [--eligibility-override REASON]
              [--tag k=v,...]       tag applications for funnel analysis
  track list [--status S] [--all]   active applications (--all includes closed)
  track show <id>                   full event history (id prefixes ok)
  track set <id> [--status S] [--tag k=v,...] [--resume-version V] [--lane L] [--source S]
              [--resume-manifest PATH] [--resume-artifact pdf|docx|ats] [--resume-artifact-file PATH]
                                    move through the funnel and/or retag
  track note <id> <text>            append a note
  track board                       kanban-style view by status
  track stats                       funnel + response-rate stats, with
                                    conversion breakdowns per lane/version/source
  track followups [--days N]        applied-but-quiet apps needing a nudge
  track remind [--days N] [--format text|ics|json] [--out PATH|auto]
                                    export due follow-up reminders

  statuses: discovered interested applied screening interview offer accepted
            rejected withdrawn ghosted

GLOBAL
  --json          agent envelope {ok, data|error:{code,message,hint}}
                  exit codes: 0 ok, 1 error, 2 invalid args, 3 not found
  --compact       with help: token-efficient verb map for agents
  JOBKIT_HOME     state dir override (default ~/.jobkit)
  JOBKIT_PROFILE  profile path override
`
