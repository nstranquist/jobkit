// Package company stores target-company intelligence for hidden-market job
// search: companies, public ATS boards, compensation targets, and dated hiring
// signals that suggest when outreach should happen before a posting saturates.
package company

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/privatefs"
	"github.com/nstranquist/jobkit/internal/strictyaml"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Companies map[string]Company `yaml:"companies" json:"companies"`
}

type Company struct {
	Name       string   `yaml:"name" json:"name"`
	Domain     string   `yaml:"domain,omitempty" json:"domain,omitempty"`
	Stage      string   `yaml:"stage,omitempty" json:"stage,omitempty"`
	Tags       []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Boards     []string `yaml:"boards,omitempty" json:"boards,omitempty"`
	TargetComp int      `yaml:"target_comp,omitempty" json:"target_comp,omitempty"`
	Notes      []Note   `yaml:"notes,omitempty" json:"notes,omitempty"`
	Signals    []Signal `yaml:"signals,omitempty" json:"signals,omitempty"`
}

type Signal struct {
	TS     time.Time `yaml:"ts" json:"ts"`
	Type   string    `yaml:"type" json:"type"`
	Source string    `yaml:"source,omitempty" json:"source,omitempty"`
	URL    string    `yaml:"url,omitempty" json:"url,omitempty"`
	Note   string    `yaml:"note,omitempty" json:"note,omitempty"`
	Weight int       `yaml:"weight,omitempty" json:"weight,omitempty"`
}

type Note struct {
	TS   time.Time `yaml:"ts" json:"ts"`
	Text string    `yaml:"text" json:"text"`
}

type Ranked struct {
	Company    Company   `json:"company"`
	Score      int       `json:"score"`
	Signals    []string  `json:"signals,omitempty"`
	NextAction string    `json:"next_action"`
	LastSignal time.Time `json:"last_signal,omitempty"`
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
	normalize(&cfg)
	return &cfg, nil
}

func LoadOrEmpty(path string) (*Config, error) {
	cfg, err := Load(path)
	if os.IsNotExist(err) {
		return &Config{Companies: map[string]Company{}}, nil
	}
	return cfg, err
}

func Save(path string, cfg *Config) error {
	normalize(cfg)
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return privatefs.WriteFile(path, raw)
}

func (cfg *Config) Upsert(c Company) Company {
	normalize(cfg)
	key := Key(c.Name)
	if key == "" {
		return Company{}
	}
	existing := cfg.Companies[key]
	if existing.Name == "" {
		existing.Name = c.Name
	}
	if c.Domain != "" {
		existing.Domain = c.Domain
	}
	if c.Stage != "" {
		existing.Stage = c.Stage
	}
	if len(c.Tags) > 0 {
		existing.Tags = mergeStrings(existing.Tags, c.Tags)
	}
	if len(c.Boards) > 0 {
		existing.Boards = mergeStrings(existing.Boards, c.Boards)
	}
	if c.TargetComp > 0 {
		existing.TargetComp = c.TargetComp
	}
	cfg.Companies[key] = existing
	return existing
}

func (cfg *Config) AddSignal(name string, signal Signal) (Company, error) {
	normalize(cfg)
	key := Key(name)
	c, ok := cfg.Companies[key]
	if !ok {
		c = Company{Name: name}
	}
	if signal.TS.IsZero() {
		signal.TS = time.Now().UTC()
	}
	if signal.Type == "" {
		signal.Type = "manual"
	}
	c.Signals = append(c.Signals, signal)
	cfg.Companies[key] = c
	return c, nil
}

func (cfg *Config) AddNote(name, text string) (Company, error) {
	normalize(cfg)
	key := Key(name)
	c, ok := cfg.Companies[key]
	if !ok {
		c = Company{Name: name}
	}
	c.Notes = append(c.Notes, Note{TS: time.Now().UTC(), Text: text})
	cfg.Companies[key] = c
	return c, nil
}

func (cfg *Config) Find(name string) (Company, bool) {
	normalize(cfg)
	if c, ok := cfg.Companies[Key(name)]; ok {
		return c, true
	}
	for _, c := range cfg.Companies {
		if strings.Contains(strings.ToLower(c.Name), strings.ToLower(name)) {
			return c, true
		}
	}
	return Company{}, false
}

func (cfg *Config) Ranked(now time.Time) []Ranked {
	normalize(cfg)
	out := make([]Ranked, 0, len(cfg.Companies))
	for _, c := range cfg.Companies {
		score, signals, last := HiddenScore(c, now)
		out = append(out, Ranked{Company: c, Score: score, Signals: signals, LastSignal: last, NextAction: nextAction(score, c)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Company.Name < out[j].Company.Name
	})
	return out
}

func HiddenScore(c Company, now time.Time) (int, []string, time.Time) {
	score := 0
	var signals []string
	var last time.Time
	if c.TargetComp >= 400000 {
		score += 25
		signals = append(signals, "top-comp-target")
	} else if c.TargetComp >= 275000 {
		score += 15
		signals = append(signals, "high-comp-target")
	}
	for _, tag := range c.Tags {
		switch strings.ToLower(tag) {
		case "ai", "agents", "ai-infra", "devtools", "platform":
			score += 5
		}
	}
	for _, s := range c.Signals {
		if s.TS.After(last) {
			last = s.TS
		}
		weight := s.Weight
		if weight == 0 {
			weight = signalWeight(s.Type)
		}
		ageDays := int(now.Sub(s.TS).Hours() / 24)
		switch {
		case ageDays <= 14:
			weight += 15
		case ageDays <= 45:
			weight += 8
		case ageDays > 120:
			weight -= 8
		}
		if weight > 0 {
			score += weight
			signals = append(signals, s.Type)
		}
	}
	if len(c.Boards) == 0 {
		score += 8
		signals = append(signals, "no-public-board")
	}
	if score < 0 {
		score = 0
	}
	return score, unique(signals), last
}

func Key(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

func signalWeight(kind string) int {
	switch strings.ToLower(kind) {
	case "funding":
		return 22
	case "launch", "product-launch":
		return 18
	case "team-growth", "hiring-manager", "recruiter":
		return 16
	case "backchannel", "referral":
		return 20
	default:
		return 8
	}
}

func nextAction(score int, c Company) string {
	switch {
	case score >= 45:
		return "warm-intro-now"
	case score >= 25:
		return "research-contact"
	case len(c.Boards) > 0:
		return "watch-board"
	default:
		return "add-signal"
	}
}

func normalize(cfg *Config) {
	if cfg.Companies == nil {
		cfg.Companies = map[string]Company{}
	}
	for key, c := range cfg.Companies {
		if c.Name == "" {
			c.Name = key
		}
		c.Tags = unique(c.Tags)
		c.Boards = unique(c.Boards)
		cfg.Companies[key] = c
	}
}

func mergeStrings(a, b []string) []string {
	return unique(append(append([]string{}, a...), b...))
}

func unique(values []string) []string {
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
	sort.Strings(out)
	return out
}
