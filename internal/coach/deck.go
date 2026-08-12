package coach

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/jd"
)

type Mode string

const (
	ModeProject      Mode = "project"
	ModeBehavioral   Mode = "behavioral"
	ModeSystemDesign Mode = "system-design"
	ModeClaimDefense Mode = "claim-defense"
	ModeMixed        Mode = "mixed"
)

func ParseMode(raw string) (Mode, error) {
	if strings.TrimSpace(raw) == "" {
		return ModeMixed, nil
	}
	mode := Mode(strings.ToLower(strings.TrimSpace(raw)))
	switch mode {
	case ModeProject, ModeBehavioral, ModeSystemDesign, ModeClaimDefense, ModeMixed:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown coach mode %q", raw)
	}
}

type DeckOptions struct {
	Mode       Mode
	Minutes    int
	ProjectIDs []string
	Now        time.Time
}

type Deck struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	SourceDigest  string     `json:"source_digest"`
	JDDigest      string     `json:"jd_digest"`
	Role          string     `json:"role"`
	Company       string     `json:"company,omitempty"`
	Mode          Mode       `json:"mode"`
	Minutes       int        `json:"minutes"`
	RoleKeywords  []string   `json:"role_keywords,omitempty"`
	ProjectIDs    []string   `json:"project_ids,omitempty"`
	Questions     []Question `json:"questions"`
}

type Question struct {
	ID               string   `json:"id"`
	Mode             Mode     `json:"mode"`
	Prompt           string   `json:"prompt"`
	TimeSeconds      int      `json:"time_seconds"`
	ProjectID        string   `json:"project_id,omitempty"`
	StoryID          string   `json:"story_id,omitempty"`
	Skill            string   `json:"skill,omitempty"`
	ExpectedConcepts []string `json:"expected_concepts,omitempty"`
	EvidenceIDs      []string `json:"evidence_ids,omitempty"`
	ClaimIDs         []string `json:"claim_ids,omitempty"`
}

func BuildDeck(bundle *SourceBundle, posting *jd.JD, postingText string, opts DeckOptions) (*Deck, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	if posting == nil {
		return nil, fmt.Errorf("job description is required")
	}
	if opts.Minutes == 0 {
		opts.Minutes = 20
	}
	if opts.Minutes < 5 || opts.Minutes > 120 {
		return nil, fmt.Errorf("coach minutes must be between 5 and 120")
	}
	if opts.Mode == "" {
		opts.Mode = ModeMixed
	}
	if _, err := ParseMode(string(opts.Mode)); err != nil {
		return nil, err
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	projects, err := selectProjects(bundle.Projects, posting, opts.ProjectIDs)
	if err != nil {
		return nil, err
	}
	roleKeywords := roleKeywords(posting)
	candidates := buildQuestionCandidates(bundle, projects, posting, opts.Mode)
	questionCount := opts.Minutes / 4
	if questionCount < 3 {
		questionCount = 3
	}
	if questionCount > 10 {
		questionCount = 10
	}
	if len(candidates) < questionCount {
		questionCount = len(candidates)
	}
	if questionCount == 0 {
		return nil, fmt.Errorf("coach source produced no questions for mode %q", opts.Mode)
	}
	questions := append([]Question(nil), candidates[:questionCount]...)
	perQuestion := opts.Minutes * 60 / len(questions)
	for i := range questions {
		questions[i].ID = fmt.Sprintf("q%02d", i+1)
		questions[i].TimeSeconds = perQuestion
	}
	role := strings.TrimSpace(posting.Title)
	if role == "" {
		role = "target role"
	}
	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ID)
	}
	deck := &Deck{
		SchemaVersion: SchemaVersion,
		CreatedAt:     opts.Now.UTC(),
		SourceDigest:  bundle.Digest(),
		JDDigest:      digestText(postingText),
		Role:          role,
		Company:       strings.TrimSpace(posting.Company),
		Mode:          opts.Mode,
		Minutes:       opts.Minutes,
		RoleKeywords:  roleKeywords,
		ProjectIDs:    projectIDs,
		Questions:     questions,
	}
	deck.ID = deckID(deck)
	return deck, nil
}

func selectProjects(projects []ProjectCard, posting *jd.JD, requested []string) ([]ProjectCard, error) {
	byID := map[string]ProjectCard{}
	for _, project := range projects {
		byID[project.ID] = project
	}
	if len(requested) > 0 {
		out := make([]ProjectCard, 0, len(requested))
		for _, id := range requested {
			project, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown coach project %q", id)
			}
			out = append(out, project)
		}
		return out, nil
	}
	type ranked struct {
		project ProjectCard
		score   int
	}
	var rows []ranked
	for _, project := range projects {
		haystack := strings.ToLower(project.Name + " " + project.Summary + " " + strings.Join(project.Skills, " "))
		score := 0
		for _, skill := range posting.Skills {
			if strings.Contains(haystack, strings.ToLower(skill.Name)) {
				score += 3
				if skill.Required {
					score += 2
				}
			}
		}
		rows = append(rows, ranked{project: project, score: score})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].score != rows[j].score {
			return rows[i].score > rows[j].score
		}
		return rows[i].project.ID < rows[j].project.ID
	})
	if len(rows) > 3 {
		rows = rows[:3]
	}
	out := make([]ProjectCard, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.project)
	}
	return out, nil
}

func buildQuestionCandidates(bundle *SourceBundle, projects []ProjectCard, posting *jd.JD, mode Mode) []Question {
	projectQuestions := func() []Question {
		var out []Question
		for _, project := range projects {
			evidenceIDs, claimIDs := projectEvidence(project)
			concepts := uniqueLower(append([]string{"problem", "ownership", "architecture", "result"}, project.Skills...))
			out = append(out, Question{
				Mode: ModeProject, ProjectID: project.ID,
				Prompt:           fmt.Sprintf("Explain %s to a senior engineer. State the problem, your ownership, the architecture, and one verified result.", project.Name),
				ExpectedConcepts: concepts, EvidenceIDs: evidenceIDs, ClaimIDs: claimIDs,
			})
			decisions := append([]string{"tradeoff", "alternative", "failure mode"}, project.Decisions...)
			out = append(out, Question{
				Mode: ModeProject, ProjectID: project.ID,
				Prompt:           fmt.Sprintf("Describe the hardest design decision in %s. Compare the chosen approach with one alternative and name the failure mode.", project.Name),
				ExpectedConcepts: uniqueLower(decisions), EvidenceIDs: evidenceIDs, ClaimIDs: claimIDs,
			})
		}
		return out
	}
	behavioralQuestions := func() []Question {
		var out []Question
		for _, story := range bundle.Stories {
			out = append(out, Question{
				Mode: ModeBehavioral, StoryID: story.ID,
				Prompt:           fmt.Sprintf("Use STAR to explain %s. Separate the situation, task, action, and result.", story.Title),
				ExpectedConcepts: uniqueLower(append([]string{"situation", "task", "action", "result"}, story.Skills...)),
				EvidenceIDs:      story.EvidenceIDs,
				ClaimIDs:         story.ClaimIDs,
			})
		}
		return out
	}
	systemQuestions := func() []Question {
		keywords := roleKeywords(posting)
		primary := "a production service"
		projectID := ""
		evidenceIDs := []string(nil)
		claimIDs := []string(nil)
		if len(projects) > 0 {
			primary = projects[0].Name
			projectID = projects[0].ID
			evidenceIDs, claimIDs = projectEvidence(projects[0])
		}
		return []Question{
			{
				Mode: ModeSystemDesign, ProjectID: projectID,
				Prompt:           fmt.Sprintf("Design a production system related to %s for %s. Cover requirements, data flow, failure modes, observability, and rollout.", primary, strings.TrimSpace(posting.Title)),
				ExpectedConcepts: uniqueLower(append([]string{"requirements", "data flow", "failure", "observability", "rollout"}, keywords...)),
				EvidenceIDs:      evidenceIDs,
				ClaimIDs:         claimIDs,
			},
			{
				Mode: ModeSystemDesign, ProjectID: projectID,
				Prompt:           "Explain how you would test and operate this design. Include capacity, degradation, recovery, and one measurable service objective.",
				ExpectedConcepts: []string{"capacity", "degradation", "recovery", "service objective", "test"},
				EvidenceIDs:      evidenceIDs,
				ClaimIDs:         claimIDs,
			},
		}
	}
	claimQuestions := func() []Question {
		var out []Question
		for _, claim := range bundle.Claims {
			out = append(out, Question{
				Mode:             ModeClaimDefense,
				Prompt:           fmt.Sprintf("Defend this claim: %q. State what was measured, the source, your contribution, and the evidence boundary.", claim.Text),
				ExpectedConcepts: []string{"measured", "source", "contribution", "boundary"},
				ClaimIDs:         []string{claim.ID},
			})
		}
		return out
	}

	switch mode {
	case ModeProject:
		return projectQuestions()
	case ModeBehavioral:
		return behavioralQuestions()
	case ModeSystemDesign:
		return systemQuestions()
	case ModeClaimDefense:
		return claimQuestions()
	case ModeMixed:
		groups := [][]Question{projectQuestions(), behavioralQuestions(), systemQuestions(), claimQuestions()}
		var out []Question
		for round := 0; ; round++ {
			added := false
			for _, group := range groups {
				if round < len(group) {
					out = append(out, group[round])
					added = true
				}
			}
			if !added {
				return out
			}
		}
	default:
		return nil
	}
}

func projectEvidence(project ProjectCard) ([]string, []string) {
	var evidenceIDs, claimIDs []string
	for _, evidence := range project.Evidence {
		evidenceIDs = append(evidenceIDs, evidence.ID)
		claimIDs = append(claimIDs, evidence.ClaimIDs...)
	}
	return unique(evidenceIDs), unique(claimIDs)
}

func roleKeywords(posting *jd.JD) []string {
	var out []string
	for _, skill := range posting.Skills {
		out = append(out, strings.ToLower(skill.Name))
		if len(out) == 12 {
			break
		}
	}
	return unique(out)
}

func uniqueLower(items []string) []string {
	for i := range items {
		items[i] = strings.ToLower(strings.TrimSpace(items[i]))
	}
	return unique(items)
}

func unique(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func digestText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func deckID(deck *Deck) string {
	identity := struct {
		SourceDigest string
		JDDigest     string
		Mode         Mode
		Minutes      int
		ProjectIDs   []string
		Questions    []Question
	}{deck.SourceDigest, deck.JDDigest, deck.Mode, deck.Minutes, deck.ProjectIDs, deck.Questions}
	raw, _ := json.Marshal(identity)
	sum := sha256.Sum256(raw)
	return "deck-" + hex.EncodeToString(sum[:8])
}
