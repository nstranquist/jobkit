// Package searches owns saved board groups and query profiles for repeatable
// job search. The config is YAML at ~/.jobkit/searches.yaml so agents and
// humans can edit it directly.
package searches

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/jobsearch"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Boards   map[string][]string `yaml:"boards" json:"boards"`
	Targets  map[string]Target   `yaml:"targets,omitempty" json:"targets,omitempty"`
	Searches map[string]Profile  `yaml:"searches" json:"searches"`
}

type Target struct {
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Boards      []string `yaml:"boards" json:"boards"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

type Profile struct {
	Query      string   `yaml:"query" json:"query"`
	Boards     []string `yaml:"boards" json:"boards"`
	RemoteOnly bool     `yaml:"remote,omitempty" json:"remote,omitempty"`
	Location   string   `yaml:"location,omitempty" json:"location,omitempty"`
	Limit      int      `yaml:"limit,omitempty" json:"limit,omitempty"`
	Sort       string   `yaml:"sort,omitempty" json:"sort,omitempty"`
	MinComp    int      `yaml:"min_comp,omitempty" json:"min_comp,omitempty"`
	Persona    string   `yaml:"persona,omitempty" json:"persona,omitempty"`
}

func Template() *Config {
	return &Config{
		Boards: map[string][]string{
			"ai-infra": {
				"greenhouse:openai",
				"lever:anthropic",
			},
			"startup-ats": {
				"greenhouse:acme",
				"lever:demo",
				"ashby:Ashby",
			},
		},
		Targets: map[string]Target{
			"ai-infra": {
				Description: "AI infrastructure and agent-platform companies with public ATS boards.",
				Tags:        []string{"ai", "infra", "agents"},
				Boards:      []string{"greenhouse:openai", "ashby:Cursor", "ashby:Modal", "ashby:Vercel", "greenhouse:scaleai"},
			},
			"product-engineering": {
				Description: "Product engineering companies with strong full-stack/platform roles.",
				Tags:        []string{"product", "fullstack", "platform"},
				Boards:      []string{"greenhouse:stripe", "greenhouse:figma", "greenhouse:databricks", "greenhouse:airbnb", "greenhouse:coinbase"},
			},
		},
		Searches: map[string]Profile{
			"backend-ai": {
				Query:      "backend platform go ai",
				Boards:     []string{"#ai-infra"},
				RemoteOnly: true,
				Limit:      25,
				Sort:       "opportunity",
				Persona:    "agent-infra",
			},
		},
	}
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	normalize(&c)
	return &c, nil
}

func LoadOrEmpty(path string) (*Config, error) {
	c, err := Load(path)
	if os.IsNotExist(err) {
		return &Config{Boards: map[string][]string{}, Searches: map[string]Profile{}}, nil
	}
	return c, err
}

func WriteTemplate(path string) error {
	exists, err := pathExists(path)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s already exists", path)
	}
	return Save(path, Template())
}

func Save(path string, c *Config) error {
	normalize(c)
	raw, err := yaml.Marshal(c)
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

func (c *Config) AddSearch(name string, p Profile) {
	normalize(c)
	c.Searches[name] = p
}

func (c *Config) SearchNames() []string {
	var names []string
	for name := range c.Searches {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) BoardNames() []string {
	var names []string
	for name := range c.Boards {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) TargetNames() []string {
	var names []string
	for name := range c.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) ResolveBoards(raw []string) ([]jobsearch.Board, []string, error) {
	normalize(c)
	var specs []string
	var expand func(string, []string) error
	expand = func(spec string, stack []string) error {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			return nil
		}
		if strings.HasPrefix(spec, "@") {
			name := strings.TrimPrefix(spec, "@")
			if contains(stack, name) {
				return fmt.Errorf("board group cycle: %s -> %s", strings.Join(stack, " -> "), name)
			}
			group, ok := c.Boards[name]
			if !ok {
				return fmt.Errorf("unknown board group @%s", name)
			}
			for _, child := range group {
				if err := expand(child, append(stack, name)); err != nil {
					return err
				}
			}
			return nil
		}
		if strings.HasPrefix(spec, "#") {
			name := strings.TrimPrefix(spec, "#")
			stackName := "#" + name
			if contains(stack, stackName) {
				return fmt.Errorf("target pack cycle: %s -> %s", strings.Join(stack, " -> "), stackName)
			}
			target, ok := c.Targets[name]
			if !ok {
				return fmt.Errorf("unknown target pack #%s", name)
			}
			for _, child := range target.Boards {
				if err := expand(child, append(stack, stackName)); err != nil {
					return err
				}
			}
			return nil
		}
		specs = append(specs, spec)
		return nil
	}
	for _, spec := range raw {
		for _, part := range strings.Split(spec, ",") {
			if err := expand(part, nil); err != nil {
				return nil, nil, err
			}
		}
	}
	boards, err := jobsearch.ParseBoards(strings.Join(specs, ","))
	if err != nil {
		return nil, nil, err
	}
	return boards, specs, nil
}

func normalize(c *Config) {
	if c.Boards == nil {
		c.Boards = map[string][]string{}
	}
	if c.Targets == nil {
		c.Targets = map[string]Target{}
	}
	if c.Searches == nil {
		c.Searches = map[string]Profile{}
	}
}

func contains(vals []string, needle string) bool {
	for _, v := range vals {
		if v == needle {
			return true
		}
	}
	return false
}
