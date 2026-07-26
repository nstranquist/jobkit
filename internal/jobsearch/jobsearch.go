// Package jobsearch queries public company job-board APIs and normalizes
// postings into one shape for the CLI.
package jobsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/jd"
)

type Board struct {
	Provider string `json:"provider"`
	Slug     string `json:"slug"`
}

type Job struct {
	Provider     string              `json:"provider"`
	Board        string              `json:"board"`
	ID           string              `json:"id,omitempty"`
	Title        string              `json:"title"`
	Company      string              `json:"company,omitempty"`
	Department   string              `json:"department,omitempty"`
	Location     string              `json:"location,omitempty"`
	Remote       bool                `json:"remote,omitempty"`
	URL          string              `json:"url,omitempty"`
	ApplyURL     string              `json:"apply_url,omitempty"`
	Description  string              `json:"description,omitempty"`
	PublishedAt  string              `json:"published_at,omitempty"`
	Score        int                 `json:"score,omitempty"`
	Compensation *Compensation       `json:"compensation,omitempty"`
	Opportunity  Opportunity         `json:"opportunity,omitempty"`
	Eligibility  *eligibility.Result `json:"eligibility,omitempty"`
}

type Options struct {
	Query             string
	Boards            []Board
	Location          string
	RemoteOnly        bool
	Limit             int
	Strict            bool
	Sort              string
	MinComp           int
	Persona           string
	Weights           OpportunityWeights
	EligibilityPolicy *eligibility.Policy
	EligibilityFilter string
	Client            *http.Client
}

type Compensation struct {
	Raw      string `json:"raw"`
	Currency string `json:"currency,omitempty"`
	Min      int    `json:"min,omitempty"`
	Max      int    `json:"max,omitempty"`
	Period   string `json:"period,omitempty"`
}

type ashbyCompensation struct {
	CompensationTierSummary          string                  `json:"compensationTierSummary"`
	ScrapeableCompensationSalaryText string                  `json:"scrapeableCompensationSalarySummary"`
	SummaryComponents                []ashbyCompensationPart `json:"summaryComponents"`
	CompensationTiers                []ashbyCompensationTier `json:"compensationTiers"`
}

type ashbyCompensationTier struct {
	TierSummary string                  `json:"tierSummary"`
	Components  []ashbyCompensationPart `json:"components"`
}

type ashbyCompensationPart struct {
	Summary          string   `json:"summary"`
	CompensationType string   `json:"compensationType"`
	Interval         string   `json:"interval"`
	CurrencyCode     string   `json:"currencyCode"`
	MinValue         *float64 `json:"minValue"`
	MaxValue         *float64 `json:"maxValue"`
}

type Opportunity struct {
	Score          int            `json:"score"`
	FreshnessScore int            `json:"freshness_score"`
	SaturationRisk int            `json:"saturation_risk"`
	CompScore      int            `json:"comp_score"`
	Persona        string         `json:"persona,omitempty"`
	PersonaScore   int            `json:"persona_score,omitempty"`
	Signals        []string       `json:"signals,omitempty"`
	Personas       []PersonaScore `json:"personas,omitempty"`
}

type OpportunityWeights struct {
	Base         float64 `json:"base" yaml:"base"`
	Freshness    float64 `json:"freshness" yaml:"freshness"`
	Compensation float64 `json:"compensation" yaml:"compensation"`
	Persona      float64 `json:"persona" yaml:"persona"`
	Saturation   float64 `json:"saturation" yaml:"saturation"`
}

type PersonaScore struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type Warning struct {
	Provider string `json:"provider"`
	Board    string `json:"board"`
	Message  string `json:"message"`
}

type SearchResult struct {
	Jobs     []Job     `json:"jobs"`
	Warnings []Warning `json:"warnings,omitempty"`
}

func ParseBoards(raw string) ([]Board, error) {
	var boards []Board
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		board, err := ParseBoard(part)
		if err != nil {
			return nil, err
		}
		boards = append(boards, board)
	}
	return boards, nil
}

func ParseBoard(raw string) (Board, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Board{}, fmt.Errorf("empty board")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return parseBoardURL(raw)
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 {
		return Board{}, fmt.Errorf("board %q must be provider:slug or a hosted board URL", raw)
	}
	provider := normalizeProvider(parts[0])
	if provider == "" {
		return Board{}, fmt.Errorf("unknown provider %q", parts[0])
	}
	slug := strings.Trim(strings.TrimSpace(parts[1]), "/")
	if slug == "" {
		return Board{}, fmt.Errorf("board %q has an empty slug", raw)
	}
	return Board{Provider: provider, Slug: slug}, nil
}

func Search(ctx context.Context, opts Options) ([]Job, error) {
	result, err := SearchReport(ctx, opts)
	return result.Jobs, err
}

func SearchReport(ctx context.Context, opts Options) (SearchResult, error) {
	result := SearchResult{Warnings: []Warning{}}
	if len(opts.Boards) == 0 {
		return result, fmt.Errorf("at least one board is required")
	}
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	var all []Job
	for _, board := range opts.Boards {
		jobs, err := fetchBoard(ctx, client, board)
		if err != nil {
			if opts.Strict {
				return result, err
			}
			result.Warnings = append(result.Warnings, Warning{Provider: board.Provider, Board: board.Slug, Message: err.Error()})
			continue
		}
		all = append(all, jobs...)
	}
	if len(result.Warnings) == len(opts.Boards) {
		return result, fmt.Errorf("all boards failed: %s", warningMessages(result.Warnings))
	}
	filtered := filterJobs(all, opts)
	if len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	if filtered == nil {
		filtered = []Job{}
	}
	result.Jobs = filtered
	return result, nil
}

func fetchBoard(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	switch board.Provider {
	case "greenhouse":
		return fetchGreenhouse(ctx, client, board)
	case "lever":
		return fetchLever(ctx, client, board)
	case "ashby":
		return fetchAshby(ctx, client, board)
	default:
		return nil, fmt.Errorf("unknown provider %q", board.Provider)
	}
}

func parseBoardURL(raw string) (Board, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Board{}, err
	}
	host := strings.ToLower(u.Hostname())
	parts := splitPath(u.Path)
	switch {
	case strings.Contains(host, "greenhouse.io"):
		if len(parts) == 0 {
			return Board{}, fmt.Errorf("could not infer Greenhouse board token from %q", raw)
		}
		return Board{Provider: "greenhouse", Slug: parts[0]}, nil
	case host == "jobs.lever.co" || host == "jobs.eu.lever.co":
		if len(parts) == 0 {
			return Board{}, fmt.Errorf("could not infer Lever site from %q", raw)
		}
		return Board{Provider: "lever", Slug: parts[0]}, nil
	case host == "jobs.ashbyhq.com":
		if len(parts) == 0 {
			return Board{}, fmt.Errorf("could not infer Ashby job board name from %q", raw)
		}
		return Board{Provider: "ashby", Slug: parts[0]}, nil
	default:
		return Board{}, fmt.Errorf("unsupported board URL host %q", host)
	}
}

func normalizeProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "greenhouse", "gh":
		return "greenhouse"
	case "lever":
		return "lever"
	case "ashby":
		return "ashby"
	default:
		return ""
	}
}

func splitPath(path string) []string {
	var out []string
	for _, part := range strings.Split(path, "/") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func fetchGreenhouse(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	var payload struct {
		Jobs []struct {
			ID          any    `json:"id"`
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			Content     string `json:"content"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
			Departments []struct {
				Name string `json:"name"`
			} `json:"departments"`
			Offices []struct {
				Name     string `json:"name"`
				Location string `json:"location"`
			} `json:"offices"`
		} `json:"jobs"`
	}
	endpoint := joinURL(envDefault("JOBKIT_GREENHOUSE_BASE", "https://boards-api.greenhouse.io/v1"), "boards", board.Slug, "jobs") + "?content=true"
	if err := getJSON(ctx, client, endpoint, &payload); err != nil {
		return nil, fmt.Errorf("greenhouse %s: %w", board.Slug, err)
	}
	jobs := make([]Job, 0, len(payload.Jobs))
	for _, raw := range payload.Jobs {
		location := raw.Location.Name
		if location == "" {
			location = joinOfficeLocations(raw.Offices)
		}
		desc := htmlToText(raw.Content)
		jobs = append(jobs, Job{
			Provider:    "greenhouse",
			Board:       board.Slug,
			ID:          fmt.Sprint(raw.ID),
			Title:       raw.Title,
			Company:     board.Slug,
			Department:  firstDepartment(raw.Departments),
			Location:    location,
			Remote:      containsFold(location, "remote"),
			URL:         raw.AbsoluteURL,
			ApplyURL:    raw.AbsoluteURL,
			Description: desc,
		})
	}
	return jobs, nil
}

func fetchLever(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	var payload []struct {
		ID               string `json:"id"`
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		ApplyURL         string `json:"applyUrl"`
		Description      string `json:"description"`
		DescriptionPlain string `json:"descriptionPlain"`
		WorkplaceType    string `json:"workplaceType"`
		Categories       struct {
			Team       string `json:"team"`
			Department string `json:"department"`
			Location   string `json:"location"`
			Commitment string `json:"commitment"`
		} `json:"categories"`
		Lists []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
	}
	endpoint := joinURL(envDefault("JOBKIT_LEVER_BASE", "https://api.lever.co/v0"), "postings", board.Slug) + "?mode=json"
	if err := getJSON(ctx, client, endpoint, &payload); err != nil {
		return nil, fmt.Errorf("lever %s: %w", board.Slug, err)
	}
	jobs := make([]Job, 0, len(payload))
	for _, raw := range payload {
		desc := firstNonEmpty(raw.DescriptionPlain, htmlToText(raw.Description), leverListsText(raw.Lists))
		dept := firstNonEmpty(raw.Categories.Department, raw.Categories.Team)
		remote := containsFold(raw.WorkplaceType, "remote") || containsFold(raw.Categories.Location, "remote")
		jobs = append(jobs, Job{
			Provider:    "lever",
			Board:       board.Slug,
			ID:          raw.ID,
			Title:       raw.Text,
			Company:     board.Slug,
			Department:  dept,
			Location:    raw.Categories.Location,
			Remote:      remote,
			URL:         raw.HostedURL,
			ApplyURL:    raw.ApplyURL,
			Description: desc,
		})
	}
	return jobs, nil
}

func fetchAshby(ctx context.Context, client *http.Client, board Board) ([]Job, error) {
	var payload struct {
		Jobs []struct {
			ID               string            `json:"id"`
			Title            string            `json:"title"`
			Location         string            `json:"location"`
			Department       string            `json:"department"`
			Team             string            `json:"team"`
			IsRemote         bool              `json:"isRemote"`
			WorkplaceType    string            `json:"workplaceType"`
			DescriptionHTML  string            `json:"descriptionHtml"`
			DescriptionPlain string            `json:"descriptionPlain"`
			PublishedAt      string            `json:"publishedAt"`
			JobURL           string            `json:"jobUrl"`
			ApplyURL         string            `json:"applyUrl"`
			Compensation     ashbyCompensation `json:"compensation"`
			Secondary        []struct {
				Location string `json:"location"`
			} `json:"secondaryLocations"`
		} `json:"jobs"`
	}
	endpoint := joinURL(envDefault("JOBKIT_ASHBY_BASE", "https://api.ashbyhq.com"), "posting-api", "job-board", board.Slug) + "?includeCompensation=true"
	if err := getJSON(ctx, client, endpoint, &payload); err != nil {
		return nil, fmt.Errorf("ashby %s: %w", board.Slug, err)
	}
	jobs := make([]Job, 0, len(payload.Jobs))
	for _, raw := range payload.Jobs {
		location := firstNonEmpty(raw.Location, joinSecondaryLocations(raw.Secondary))
		dept := firstNonEmpty(raw.Department, raw.Team)
		remote := raw.IsRemote || containsFold(raw.WorkplaceType, "remote") || containsFold(location, "remote")
		desc := firstNonEmpty(raw.DescriptionPlain, htmlToText(raw.DescriptionHTML))
		jobs = append(jobs, Job{
			Provider:     "ashby",
			Board:        board.Slug,
			ID:           raw.ID,
			Title:        raw.Title,
			Company:      board.Slug,
			Department:   dept,
			Location:     location,
			Remote:       remote,
			URL:          raw.JobURL,
			ApplyURL:     raw.ApplyURL,
			Description:  desc,
			PublishedAt:  raw.PublishedAt,
			Compensation: ashbyCompensationRange(raw.Compensation, desc),
		})
	}
	return jobs, nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "jobkit/0.8.0 (+https://github.com/nstranquist/jobkit)")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s", endpoint, resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	return dec.Decode(dest)
}

func filterJobs(jobs []Job, opts Options) []Job {
	terms := queryTerms(opts.Query)
	location := strings.ToLower(strings.TrimSpace(opts.Location))
	var out []Job
	for _, job := range jobs {
		if opts.RemoteOnly && !job.Remote {
			continue
		}
		if location != "" && !strings.Contains(strings.ToLower(job.Location), location) {
			continue
		}
		score, ok := scoreJob(job, terms)
		if !ok {
			continue
		}
		job.Score = score
		if job.Compensation == nil {
			job.Compensation = ExtractCompensation(job.Description)
		}
		if opts.MinComp > 0 && compensationCeiling(job.Compensation) < opts.MinComp {
			continue
		}
		job.Opportunity = BuildOpportunityWithWeights(job, opts.Persona, opts.Weights)
		if opts.EligibilityPolicy != nil {
			assessment := eligibility.Evaluate(eligibility.Posting{
				Title: job.Title, Location: job.Location, Remote: job.Remote, Description: job.Description,
			}, *opts.EligibilityPolicy)
			job.Eligibility = &assessment
			if !eligibility.Allows(opts.EligibilityFilter, assessment.Status) {
				continue
			}
		}
		out = append(out, job)
	}
	sortJobs(out, opts.Sort)
	return out
}

func sortJobs(jobs []Job, mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Eligibility != nil && jobs[j].Eligibility != nil && jobs[i].Eligibility.Status != jobs[j].Eligibility.Status {
			return eligibility.Rank(jobs[i].Eligibility.Status) < eligibility.Rank(jobs[j].Eligibility.Status)
		}
		switch mode {
		case "comp", "pay", "salary":
			if compensationCeiling(jobs[i].Compensation) != compensationCeiling(jobs[j].Compensation) {
				return compensationCeiling(jobs[i].Compensation) > compensationCeiling(jobs[j].Compensation)
			}
		case "fresh", "freshness":
			if jobs[i].Opportunity.FreshnessScore != jobs[j].Opportunity.FreshnessScore {
				return jobs[i].Opportunity.FreshnessScore > jobs[j].Opportunity.FreshnessScore
			}
		case "opportunity", "opp":
			if jobs[i].Opportunity.Score != jobs[j].Opportunity.Score {
				return jobs[i].Opportunity.Score > jobs[j].Opportunity.Score
			}
		}
		if jobs[i].Score != jobs[j].Score {
			return jobs[i].Score > jobs[j].Score
		}
		if jobs[i].Provider != jobs[j].Provider {
			return jobs[i].Provider < jobs[j].Provider
		}
		return jobs[i].Title < jobs[j].Title
	})
}

func BuildOpportunity(job Job, persona string) Opportunity {
	return BuildOpportunityWithWeights(job, persona, DefaultOpportunityWeights())
}

func BuildOpportunityWithWeights(job Job, persona string, weights OpportunityWeights) Opportunity {
	fresh := freshnessScore(job.PublishedAt)
	risk, signals := saturationRisk(job)
	comp := compScore(job.Compensation)
	personas := scorePersonas(job)
	selectedPersona, personaScore := selectPersona(personas, persona)
	total := ScoreOpportunityComponents(job.Score, fresh, comp, personaScore, risk, weights)
	return Opportunity{
		Score: int(math.Round(total)), FreshnessScore: fresh, SaturationRisk: risk, CompScore: comp,
		Persona: selectedPersona, PersonaScore: personaScore, Signals: signals, Personas: personas,
	}
}

func DefaultOpportunityWeights() OpportunityWeights {
	return OpportunityWeights{Base: 1, Freshness: 1, Compensation: 1, Persona: 1, Saturation: 1}
}

func NormalizeOpportunityWeights(weights OpportunityWeights) OpportunityWeights {
	if weights.Base == 0 && weights.Freshness == 0 && weights.Compensation == 0 && weights.Persona == 0 && weights.Saturation == 0 {
		return DefaultOpportunityWeights()
	}
	return weights
}

func ScoreOpportunityComponents(base, freshness, compensation, persona, saturation int, weights OpportunityWeights) float64 {
	weights = NormalizeOpportunityWeights(weights)
	return float64(base)*weights.Base +
		float64(freshness)*weights.Freshness +
		float64(compensation)*weights.Compensation +
		float64(persona)*weights.Persona -
		float64(saturation)*weights.Saturation
}

func ExtractCompensation(text string) *Compensation {
	for _, re := range compensationRegexes {
		match := re.FindStringSubmatch(text)
		if len(match) == 0 {
			continue
		}
		min := parseAmount(match[1])
		max := min
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			max = parseAmount(match[2])
		}
		if min == 0 && max == 0 {
			continue
		}
		currency := "USD"
		if strings.Contains(strings.ToUpper(match[0]), "CAD") {
			currency = "CAD"
		}
		period := "year"
		if strings.Contains(strings.ToLower(match[0]), "hour") {
			period = "hour"
		}
		return &Compensation{Raw: compactSpace(match[0]), Currency: currency, Min: min, Max: max, Period: period}
	}
	return nil
}

func ashbyCompensationRange(raw ashbyCompensation, description string) *Compensation {
	var selected *Compensation
	for _, part := range raw.SummaryComponents {
		if comp := ashbyCompensationPartRange(part); comp != nil {
			selected = betterCompensation(selected, comp)
		}
	}
	for _, tier := range raw.CompensationTiers {
		for _, part := range tier.Components {
			if comp := ashbyCompensationPartRange(part); comp != nil {
				if tier.TierSummary != "" {
					comp.Raw = tier.TierSummary
				}
				selected = betterCompensation(selected, comp)
			}
		}
	}
	if selected != nil {
		if selected.Raw == "" {
			selected.Raw = firstNonEmpty(raw.ScrapeableCompensationSalaryText, raw.CompensationTierSummary)
		}
		return selected
	}
	for _, text := range []string{raw.ScrapeableCompensationSalaryText, raw.CompensationTierSummary, description} {
		if comp := ExtractCompensation(text); comp != nil {
			return comp
		}
	}
	return nil
}

func ashbyCompensationPartRange(part ashbyCompensationPart) *Compensation {
	if !strings.EqualFold(part.CompensationType, "Salary") && !strings.Contains(strings.ToLower(part.Summary), "$") {
		return nil
	}
	min := 0
	max := 0
	if part.MinValue != nil {
		min = int(*part.MinValue)
	}
	if part.MaxValue != nil {
		max = int(*part.MaxValue)
	}
	if min == 0 && max == 0 {
		return ExtractCompensation(part.Summary)
	}
	period := "year"
	if strings.Contains(strings.ToLower(part.Interval), "hour") {
		period = "hour"
	}
	return &Compensation{
		Raw:      compactSpace(firstNonEmpty(part.Summary, fmt.Sprintf("%d-%d", min, max))),
		Currency: firstNonEmpty(part.CurrencyCode, "USD"), Min: min, Max: max, Period: period,
	}
}

func betterCompensation(a, b *Compensation) *Compensation {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if compensationCeiling(b) > compensationCeiling(a) {
		return b
	}
	return a
}

var compensationRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:compensation range|pay range|annual base salary range|base salary range|salary range)[\s\S]{0,120}?\$([0-9][0-9,]*(?:\.[0-9]+)?\s*[kK]?)(?:\s*(?:—|-|to)\s*\$?([0-9][0-9,]*(?:\.[0-9]+)?\s*[kK]?))?\s*(?:USD|CAD)?`),
	regexp.MustCompile(`(?i)\$([0-9][0-9,]*(?:\.[0-9]+)?\s*[kK]?)\s*(?:—|-|to)\s*\$?([0-9][0-9,]*(?:\.[0-9]+)?\s*[kK]?)\s*(?:USD|CAD)?`),
}

func parseAmount(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	mult := 1.0
	if strings.HasSuffix(raw, "k") {
		mult = 1000
		raw = strings.TrimSuffix(raw, "k")
	}
	raw = strings.ReplaceAll(raw, ",", "")
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return int(v * mult)
}

func compensationCeiling(c *Compensation) int {
	if c == nil {
		return 0
	}
	if c.Max > 0 {
		return c.Max
	}
	return c.Min
}

func compScore(c *Compensation) int {
	top := compensationCeiling(c)
	switch {
	case top >= 450000:
		return 35
	case top >= 350000:
		return 28
	case top >= 275000:
		return 22
	case top >= 225000:
		return 16
	case top >= 175000:
		return 10
	case top > 0:
		return 4
	default:
		return 0
	}
}

func freshnessScore(publishedAt string) int {
	if strings.TrimSpace(publishedAt) == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		return 0
	}
	age := time.Since(t)
	switch {
	case age < 0:
		return 10
	case age <= 7*24*time.Hour:
		return 25
	case age <= 30*24*time.Hour:
		return 18
	case age <= 90*24*time.Hour:
		return 8
	case age <= 180*24*time.Hour:
		return 2
	default:
		return -8
	}
}

func saturationRisk(job Job) (int, []string) {
	var risk int
	var signals []string
	title := strings.ToLower(job.Title)
	loc := strings.ToLower(job.Location)
	desc := strings.ToLower(job.Description)
	if job.Remote {
		risk += 18
		signals = append(signals, "remote-crowding")
	}
	if strings.Contains(loc, "remote") && (strings.Contains(loc, "us") || strings.Contains(loc, "usa") || strings.Contains(loc, "north america")) {
		risk += 10
		signals = append(signals, "broad-remote-region")
	}
	if title == "software engineer" || title == "senior software engineer" || title == "fullstack engineer" {
		risk += 12
		signals = append(signals, "generic-title")
	}
	if strings.Contains(desc, "application limit") || strings.Contains(desc, "high volume") {
		risk += 8
		signals = append(signals, "high-volume-process")
	}
	if strings.Contains(desc, "referral") || strings.Contains(desc, "employee referral") {
		risk -= 5
		signals = append(signals, "referral-channel")
	}
	if risk < 0 {
		risk = 0
	}
	if risk > 100 {
		risk = 100
	}
	return risk, signals
}

func scorePersonas(job Job) []PersonaScore {
	text := strings.ToLower(strings.Join([]string{job.Title, job.Department, job.Description}, " "))
	defs := map[string][]string{
		"agent-infra":       {"agent", "agents", "orchestration", "sandbox", "cloud", "infrastructure", "distributed", "platform", "observability"},
		"ai-product":        {"ai", "llm", "rag", "product", "fullstack", "react", "typescript", "workflow", "eval"},
		"devtools":          {"developer", "sdk", "api", "tooling", "docs", "dx", "platform", "open source"},
		"backend-platform":  {"backend", "platform", "go", "distributed", "api", "kubernetes", "reliability", "systems"},
		"fintech-platform":  {"payments", "ledger", "risk", "financial", "crypto", "settlement", "compliance"},
		"fullstack-product": {"fullstack", "react", "typescript", "product", "graphql", "postgres", "node", "ux"},
	}
	out := make([]PersonaScore, 0, len(defs))
	for name, terms := range defs {
		score := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				score += 5
			}
		}
		if score > 0 {
			out = append(out, PersonaScore{Name: name, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func selectPersona(scores []PersonaScore, persona string) (string, int) {
	persona = strings.TrimSpace(persona)
	if persona != "" {
		for _, score := range scores {
			if score.Name == persona {
				return score.Name, score.Score
			}
		}
		return persona, 0
	}
	if len(scores) == 0 {
		return "", 0
	}
	return scores[0].Name, scores[0].Score
}

func queryTerms(query string) []string {
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(query)) {
		term = strings.Trim(term, ".,:;()[]{}\"'")
		if len(term) >= 2 {
			terms = append(terms, term)
		}
	}
	return terms
}

func scoreJob(job Job, terms []string) (int, bool) {
	if len(terms) == 0 {
		return 1, true
	}
	title := strings.ToLower(job.Title)
	meta := strings.ToLower(strings.Join([]string{job.Company, job.Department, job.Location}, " "))
	desc := strings.ToLower(job.Description)
	score := 0
	matched := 0
	for _, term := range terms {
		termScore := 0
		if strings.Contains(title, term) {
			termScore += 5
		}
		if strings.Contains(meta, term) {
			termScore += 2
		}
		if strings.Contains(desc, term) {
			termScore++
		}
		if termScore > 0 {
			matched++
		}
		score += termScore
	}
	// Queries of up to 3 terms stay strict: every term must appear.
	// Longer queries need 60% coverage so one rare word ("backstage")
	// can't zero an otherwise strong result set; the score still rewards
	// full matches, and ranking keeps precision.
	need := len(terms)
	if len(terms) > 3 {
		need = (len(terms)*3 + 4) / 5 // ceil(0.6 * n)
	}
	if matched < need {
		return 0, false
	}
	return score, true
}

func htmlToText(raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	text, err := jd.ToText(strings.NewReader(raw))
	if err == nil {
		return strings.TrimSpace(text)
	}
	return strings.Join(strings.Fields(raw), " ")
}

func leverListsText(lists []struct {
	Text    string `json:"text"`
	Content string `json:"content"`
}) string {
	var parts []string
	for _, list := range lists {
		if list.Text != "" {
			parts = append(parts, list.Text)
		}
		if list.Content != "" {
			parts = append(parts, htmlToText(list.Content))
		}
	}
	return strings.Join(parts, "\n")
}

func firstDepartment(deps []struct {
	Name string `json:"name"`
}) string {
	if len(deps) == 0 {
		return ""
	}
	return deps[0].Name
}

func joinOfficeLocations(offices []struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}) string {
	var parts []string
	for _, office := range offices {
		parts = append(parts, firstNonEmpty(office.Location, office.Name))
	}
	return strings.Join(uniqueNonEmpty(parts), ", ")
}

func joinSecondaryLocations(locs []struct {
	Location string `json:"location"`
}) string {
	var parts []string
	for _, loc := range locs {
		parts = append(parts, loc.Location)
	}
	return strings.Join(uniqueNonEmpty(parts), ", ")
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func joinURL(base string, parts ...string) string {
	base = strings.TrimRight(base, "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, url.PathEscape(strings.Trim(part, "/")))
	}
	return base + "/" + strings.Join(escaped, "/")
}

func envDefault(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return fallback
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func warningMessages(warnings []Warning) string {
	parts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		parts = append(parts, fmt.Sprintf("%s:%s: %s", warning.Provider, warning.Board, warning.Message))
	}
	return strings.Join(parts, "; ")
}
