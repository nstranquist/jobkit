// Package profile defines the master profile — the single source of truth
// about the candidate that every other jobkit surface (match, resume, letter)
// reads from. Stored as YAML at ~/.jobkit/profile.yaml.
package profile

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/privatefs"
	"gopkg.in/yaml.v3"
)

//go:embed template.yaml
var templateYAML string

// Link is a labeled URL (GitHub, portfolio, LinkedIn...).
type Link struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
}

// Skill is a declared capability with optional depth metadata.
type Skill struct {
	Name    string   `yaml:"name" json:"name"`
	Level   string   `yaml:"level,omitempty" json:"level,omitempty"` // familiar|proficient|expert
	Years   float64  `yaml:"years,omitempty" json:"years,omitempty"`
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
}

// Bullet is one achievement line; Tags pin it to skills beyond what the text
// itself mentions.
type Bullet struct {
	Text string   `yaml:"text" json:"text"`
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Experience is one role at one company.
type Experience struct {
	Company  string   `yaml:"company" json:"company"`
	Role     string   `yaml:"role" json:"role"`
	Location string   `yaml:"location,omitempty" json:"location,omitempty"`
	Start    string   `yaml:"start" json:"start"`                 // YYYY-MM
	End      string   `yaml:"end,omitempty" json:"end,omitempty"` // YYYY-MM or "present"
	Bullets  []Bullet `yaml:"bullets" json:"bullets"`
}

// Project is a side/open-source project worth showing.
type Project struct {
	Name        string   `yaml:"name" json:"name"`
	URL         string   `yaml:"url,omitempty" json:"url,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Bullets     []Bullet `yaml:"bullets,omitempty" json:"bullets,omitempty"`
}

// Education is one degree/program.
type Education struct {
	School string `yaml:"school" json:"school"`
	Degree string `yaml:"degree,omitempty" json:"degree,omitempty"`
	Field  string `yaml:"field,omitempty" json:"field,omitempty"`
	Year   string `yaml:"year,omitempty" json:"year,omitempty"`
}

// Profile is the master record.
type Profile struct {
	Name           string       `yaml:"name" json:"name"`
	Headline       string       `yaml:"headline,omitempty" json:"headline,omitempty"`
	Email          string       `yaml:"email,omitempty" json:"email,omitempty"`
	Phone          string       `yaml:"phone,omitempty" json:"phone,omitempty"`
	Location       string       `yaml:"location,omitempty" json:"location,omitempty"`
	Links          []Link       `yaml:"links,omitempty" json:"links,omitempty"`
	Summary        string       `yaml:"summary,omitempty" json:"summary,omitempty"`
	Skills         []Skill      `yaml:"skills,omitempty" json:"skills,omitempty"`
	Experience     []Experience `yaml:"experience,omitempty" json:"experience,omitempty"`
	Projects       []Project    `yaml:"projects,omitempty" json:"projects,omitempty"`
	Education      []Education  `yaml:"education,omitempty" json:"education,omitempty"`
	Certifications []string     `yaml:"certifications,omitempty" json:"certifications,omitempty"`
}

// Load reads and validates a profile from path.
func Load(path string) (*Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if errs := p.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("invalid profile %s: %s", path, strings.Join(errs, "; "))
	}
	return &p, nil
}

// Validate returns human-readable problems; empty means valid.
func (p *Profile) Validate() []string {
	var errs []string
	if strings.TrimSpace(p.Name) == "" {
		errs = append(errs, "name is required")
	}
	for i, e := range p.Experience {
		if e.Company == "" || e.Role == "" {
			errs = append(errs, fmt.Sprintf("experience[%d]: company and role are required", i))
		}
		if e.Start == "" {
			errs = append(errs, fmt.Sprintf("experience[%d] (%s): start is required", i, e.Company))
		}
	}
	for i, s := range p.Skills {
		if s.Name == "" {
			errs = append(errs, fmt.Sprintf("skills[%d]: name is required", i))
		}
	}
	return errs
}

// WriteTemplate writes the starter profile to path. Refuses to overwrite.
func WriteTemplate(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if info != nil && info.IsDir() {
			return fmt.Errorf("%s already exists and is a directory", path)
		}
		return fmt.Errorf("%s already exists", path)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("check %s: %w", path, err)
	}
	return privatefs.WriteFile(path, []byte(templateYAML))
}

// SkillTerms returns every searchable term the profile claims, lowercased:
// declared skill names + aliases + bullet tags. Used by match scoring.
func (p *Profile) SkillTerms() map[string]bool {
	terms := map[string]bool{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			terms[s] = true
		}
	}
	for _, s := range p.Skills {
		add(s.Name)
		for _, a := range s.Aliases {
			add(a)
		}
	}
	for _, e := range p.Experience {
		for _, b := range e.Bullets {
			for _, t := range b.Tags {
				add(t)
			}
		}
	}
	for _, pr := range p.Projects {
		for _, b := range pr.Bullets {
			for _, t := range b.Tags {
				add(t)
			}
		}
	}
	return terms
}

// SkillByTerm finds a declared skill matching term (name or alias,
// case-insensitive). Returns nil when only implied by tags/text.
func (p *Profile) SkillByTerm(term string) *Skill {
	t := strings.ToLower(term)
	for i := range p.Skills {
		if strings.ToLower(p.Skills[i].Name) == t {
			return &p.Skills[i]
		}
		for _, a := range p.Skills[i].Aliases {
			if strings.ToLower(a) == t {
				return &p.Skills[i]
			}
		}
	}
	return nil
}

// AllText concatenates every free-text field, for evidence scanning.
func (p *Profile) AllText() string {
	var b strings.Builder
	b.WriteString(p.Summary)
	b.WriteString("\n")
	for _, e := range p.Experience {
		for _, bl := range e.Bullets {
			b.WriteString(bl.Text)
			b.WriteString("\n")
		}
	}
	for _, pr := range p.Projects {
		b.WriteString(pr.Description)
		b.WriteString("\n")
		for _, bl := range pr.Bullets {
			b.WriteString(bl.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// TopSkills returns up to n declared skills ordered by level then years.
func (p *Profile) TopSkills(n int) []Skill {
	rank := map[string]int{"expert": 3, "proficient": 2, "familiar": 1}
	out := make([]Skill, len(p.Skills))
	copy(out, p.Skills)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank[strings.ToLower(out[i].Level)], rank[strings.ToLower(out[j].Level)]
		if ri != rj {
			return ri > rj
		}
		return out[i].Years > out[j].Years
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
