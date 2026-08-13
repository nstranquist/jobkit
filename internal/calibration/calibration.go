// Package calibration learns opportunity-ranking weights from local job-search
// outcomes. It never mutates ledgers; it reads inbox and tracker history and
// writes one optional YAML config consumed by search ranking.
package calibration

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jobsearch"
	"github.com/nstranquist/jobkit/internal/privatefs"
	"github.com/nstranquist/jobkit/internal/strictyaml"
	"github.com/nstranquist/jobkit/internal/track"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	UpdatedAt time.Time                    `yaml:"updated_at" json:"updated_at"`
	Persona   string                       `yaml:"persona" json:"persona"`
	Samples   int                          `yaml:"samples" json:"samples"`
	Weights   jobsearch.OpportunityWeights `yaml:"weights" json:"weights"`
	Metrics   Metrics                      `yaml:"metrics" json:"metrics"`
}

type Features struct {
	Base         int `json:"base" yaml:"base"`
	Freshness    int `json:"freshness" yaml:"freshness"`
	Compensation int `json:"compensation" yaml:"compensation"`
	Persona      int `json:"persona" yaml:"persona"`
	Saturation   int `json:"saturation" yaml:"saturation"`
}

type Example struct {
	ID           string   `json:"id" yaml:"id"`
	Company      string   `json:"company" yaml:"company"`
	Role         string   `json:"role" yaml:"role"`
	URL          string   `json:"url,omitempty" yaml:"url,omitempty"`
	Status       string   `json:"status" yaml:"status"`
	Source       string   `json:"source" yaml:"source"`
	Outcome      int      `json:"outcome" yaml:"outcome"`
	OutcomeLabel string   `json:"outcome_label" yaml:"outcome_label"`
	Features     Features `json:"features" yaml:"features"`
	Score        float64  `json:"score" yaml:"score"`
}

type Metrics struct {
	Samples          int     `json:"samples" yaml:"samples"`
	Positive         int     `json:"positive" yaml:"positive"`
	Negative         int     `json:"negative" yaml:"negative"`
	Pairs            int     `json:"pairs" yaml:"pairs"`
	PairwiseAccuracy float64 `json:"pairwise_accuracy" yaml:"pairwise_accuracy"`
	PositiveAverage  float64 `json:"positive_average" yaml:"positive_average"`
	NegativeAverage  float64 `json:"negative_average" yaml:"negative_average"`
	Spread           float64 `json:"spread" yaml:"spread"`
}

type WeightedMetrics struct {
	Weights jobsearch.OpportunityWeights `json:"weights" yaml:"weights"`
	Metrics Metrics                      `json:"metrics" yaml:"metrics"`
}

type Report struct {
	GeneratedAt time.Time        `json:"generated_at" yaml:"generated_at"`
	Persona     string           `json:"persona" yaml:"persona"`
	Samples     int              `json:"samples" yaml:"samples"`
	Positive    int              `json:"positive" yaml:"positive"`
	Negative    int              `json:"negative" yaml:"negative"`
	Neutral     int              `json:"neutral" yaml:"neutral"`
	Default     WeightedMetrics  `json:"default" yaml:"default"`
	Active      *WeightedMetrics `json:"active,omitempty" yaml:"active,omitempty"`
	Suggested   WeightedMetrics  `json:"suggested" yaml:"suggested"`
	Examples    []Example        `json:"examples" yaml:"examples"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := strictyaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Weights = jobsearch.NormalizeOpportunityWeights(cfg.Weights)
	return &cfg, nil
}

func LoadOrDefault(path string) (*Config, error) {
	cfg, err := Load(path)
	if os.IsNotExist(err) {
		return &Config{Weights: jobsearch.DefaultOpportunityWeights()}, nil
	}
	return cfg, err
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("nil calibration config")
	}
	cfg.Weights = jobsearch.NormalizeOpportunityWeights(cfg.Weights)
	if cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = time.Now().UTC()
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return privatefs.WriteFile(path, raw)
}

func BuildReport(items []*inbox.Item, apps []*track.Application, persona string, active *jobsearch.OpportunityWeights) Report {
	examples := BuildExamples(items, apps, persona)
	defaultWeights := jobsearch.DefaultOpportunityWeights()
	report := Report{
		GeneratedAt: time.Now().UTC(),
		Persona:     strings.TrimSpace(persona),
		Default:     WeightedMetrics{Weights: defaultWeights, Metrics: Evaluate(examples, defaultWeights)},
		Suggested:   SuggestWeights(examples),
		Examples:    examples,
	}
	if active != nil {
		weights := jobsearch.NormalizeOpportunityWeights(*active)
		activeMetrics := WeightedMetrics{Weights: weights, Metrics: Evaluate(examples, weights)}
		report.Active = &activeMetrics
	}
	for _, example := range examples {
		switch {
		case example.Outcome > 0:
			report.Positive++
			report.Samples++
		case example.Outcome < 0:
			report.Negative++
			report.Samples++
		default:
			report.Neutral++
		}
	}
	return report
}

func BuildExamples(items []*inbox.Item, apps []*track.Application, persona string) []Example {
	index := buildApplicationIndex(apps)
	examples := make([]Example, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		app := index.match(item)
		status, source, outcome := outcomeFor(item, app)
		features := itemFeatures(item, persona)
		example := Example{
			ID:           item.ID,
			Company:      item.Job.Company,
			Role:         item.Job.Title,
			URL:          firstNonEmpty(item.Job.ApplyURL, item.Job.URL, item.Job.SourceURL),
			Status:       status,
			Source:       source,
			Outcome:      outcome,
			OutcomeLabel: outcomeLabel(outcome),
			Features:     features,
		}
		example.Score = scoreExample(example, jobsearch.DefaultOpportunityWeights())
		examples = append(examples, example)
	}
	sort.SliceStable(examples, func(i, j int) bool {
		if examples[i].Outcome != examples[j].Outcome {
			return examples[i].Outcome > examples[j].Outcome
		}
		if examples[i].Score != examples[j].Score {
			return examples[i].Score > examples[j].Score
		}
		return examples[i].ID < examples[j].ID
	})
	return examples
}

func Evaluate(examples []Example, weights jobsearch.OpportunityWeights) Metrics {
	weights = jobsearch.NormalizeOpportunityWeights(weights)
	var m Metrics
	var positiveSum, negativeSum float64
	var positives, negatives []float64
	for _, example := range examples {
		if example.Outcome == 0 {
			continue
		}
		score := scoreExample(example, weights)
		m.Samples++
		if example.Outcome > 0 {
			m.Positive++
			positiveSum += score
			positives = append(positives, score)
		} else {
			m.Negative++
			negativeSum += score
			negatives = append(negatives, score)
		}
	}
	if m.Positive > 0 {
		m.PositiveAverage = positiveSum / float64(m.Positive)
	}
	if m.Negative > 0 {
		m.NegativeAverage = negativeSum / float64(m.Negative)
	}
	m.Spread = m.PositiveAverage - m.NegativeAverage
	for _, pos := range positives {
		for _, neg := range negatives {
			m.Pairs++
			switch {
			case pos > neg:
				m.PairwiseAccuracy += 1
			case pos == neg:
				m.PairwiseAccuracy += 0.5
			}
		}
	}
	if m.Pairs > 0 {
		m.PairwiseAccuracy /= float64(m.Pairs)
	}
	return m
}

func SuggestWeights(examples []Example) WeightedMetrics {
	defaultWeights := jobsearch.DefaultOpportunityWeights()
	best := WeightedMetrics{Weights: defaultWeights, Metrics: Evaluate(examples, defaultWeights)}
	if best.Metrics.Pairs == 0 {
		return best
	}
	choices := struct {
		freshness    []float64
		compensation []float64
		persona      []float64
		saturation   []float64
	}{
		freshness:    []float64{0, 0.5, 1, 1.5, 2},
		compensation: []float64{0.5, 1, 1.5, 2, 3},
		persona:      []float64{0, 0.5, 1, 1.5, 2},
		saturation:   []float64{0.5, 1, 1.5, 2, 3},
	}
	for _, freshness := range choices.freshness {
		for _, compensation := range choices.compensation {
			for _, persona := range choices.persona {
				for _, saturation := range choices.saturation {
					weights := jobsearch.OpportunityWeights{
						Base:         1,
						Freshness:    freshness,
						Compensation: compensation,
						Persona:      persona,
						Saturation:   saturation,
					}
					candidate := WeightedMetrics{Weights: weights, Metrics: Evaluate(examples, weights)}
					if better(candidate, best) {
						best = candidate
					}
				}
			}
		}
	}
	return best
}

func better(candidate, incumbent WeightedMetrics) bool {
	if candidate.Metrics.PairwiseAccuracy != incumbent.Metrics.PairwiseAccuracy {
		return candidate.Metrics.PairwiseAccuracy > incumbent.Metrics.PairwiseAccuracy
	}
	if candidate.Metrics.Spread != incumbent.Metrics.Spread {
		return candidate.Metrics.Spread > incumbent.Metrics.Spread
	}
	return weightComplexity(candidate.Weights) < weightComplexity(incumbent.Weights)
}

func weightComplexity(weights jobsearch.OpportunityWeights) float64 {
	def := jobsearch.DefaultOpportunityWeights()
	return math.Abs(weights.Base-def.Base) +
		math.Abs(weights.Freshness-def.Freshness) +
		math.Abs(weights.Compensation-def.Compensation) +
		math.Abs(weights.Persona-def.Persona) +
		math.Abs(weights.Saturation-def.Saturation)
}

func scoreExample(example Example, weights jobsearch.OpportunityWeights) float64 {
	f := example.Features
	return jobsearch.ScoreOpportunityComponents(f.Base, f.Freshness, f.Compensation, f.Persona, f.Saturation, weights)
}

func itemFeatures(item *inbox.Item, persona string) Features {
	job := jobsearch.Job{
		Provider:     item.Job.Provider,
		Board:        item.Job.Board,
		ID:           item.Job.ExternalID,
		Title:        item.Job.Title,
		Company:      item.Job.Company,
		Department:   item.Job.Department,
		Location:     item.Job.Location,
		Remote:       item.Job.Remote,
		URL:          item.Job.URL,
		ApplyURL:     item.Job.ApplyURL,
		Description:  firstNonEmpty(item.Job.Description, item.Job.JDText),
		PublishedAt:  item.Job.PublishedAt,
		Score:        int(math.Round(item.MatchScore)),
		Compensation: item.Job.Compensation,
	}
	if job.Compensation == nil {
		job.Compensation = jobsearch.ExtractCompensation(firstNonEmpty(item.Job.Description, item.Job.JDText))
	}
	opp := item.Job.Opportunity
	if (persona != "" && opp.Persona != persona) ||
		(opp.Score == 0 && opp.FreshnessScore == 0 && opp.SaturationRisk == 0 && opp.CompScore == 0 && opp.PersonaScore == 0) {
		opp = jobsearch.BuildOpportunity(job, persona)
	}
	if job.Score == 0 && opp.Score != 0 {
		job.Score = opp.Score - opp.FreshnessScore - opp.CompScore - opp.PersonaScore + opp.SaturationRisk
	}
	return Features{
		Base:         job.Score,
		Freshness:    opp.FreshnessScore,
		Compensation: opp.CompScore,
		Persona:      opp.PersonaScore,
		Saturation:   opp.SaturationRisk,
	}
}

func outcomeFor(item *inbox.Item, app *track.Application) (string, string, int) {
	if app != nil {
		return app.Status, "tracker", trackOutcome(app.Status)
	}
	return item.Status, "inbox", inboxOutcome(item.Status)
}

func trackOutcome(status string) int {
	switch status {
	case "accepted", "offer":
		return 3
	case "interview", "screening":
		return 2
	case "rejected", "withdrawn", "ghosted":
		return -2
	default:
		return 0
	}
}

func inboxOutcome(status string) int {
	switch status {
	case "shortlisted", "planned", "applied":
		return 1
	case "skipped", "archived":
		return -1
	default:
		return 0
	}
}

func outcomeLabel(outcome int) string {
	switch {
	case outcome >= 3:
		return "offer"
	case outcome > 0:
		return "positive"
	case outcome < 0:
		return "negative"
	default:
		return "neutral"
	}
}

type applicationIndex struct {
	byURL  map[string]*track.Application
	byRole map[string]*track.Application
}

func buildApplicationIndex(apps []*track.Application) applicationIndex {
	index := applicationIndex{byURL: map[string]*track.Application{}, byRole: map[string]*track.Application{}}
	for _, app := range apps {
		if app == nil {
			continue
		}
		if key := normalizedURL(app.URL); key != "" {
			index.byURL[key] = app
		}
		if key := roleKey(app.Company, app.Role); key != "" {
			index.byRole[key] = app
		}
	}
	return index
}

func (index applicationIndex) match(item *inbox.Item) *track.Application {
	for _, raw := range []string{item.Job.ApplyURL, item.Job.URL, item.Job.SourceURL} {
		if app := index.byURL[normalizedURL(raw)]; app != nil {
			return app
		}
	}
	return index.byRole[roleKey(item.Job.Company, item.Job.Title)]
}

func normalizedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "gh_src" || lower == "lever-source" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return strings.TrimRight(u.String(), "/")
}

func roleKey(company, role string) string {
	company = slug(company)
	role = slug(role)
	if company == "" || role == "" {
		return ""
	}
	return company + "--" + role
}

func slug(s string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, s)
	parts := strings.FieldsFunc(mapped, func(r rune) bool { return r == '-' })
	return strings.Join(parts, "-")
}

func firstNonEmpty(vals ...string) string {
	for _, val := range vals {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}
