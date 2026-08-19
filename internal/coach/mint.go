package coach

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const DraftSchemaVersion = 1

type StudyDraft struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	ModuleID      string    `json:"module_id"`
	Practice      Practice  `json:"practice"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
}

type StudyOverlay struct {
	SchemaVersion int               `json:"schema_version"`
	Practices     []OverlayPractice `json:"practices"`
}

type OverlayPractice struct {
	ModuleID string   `json:"module_id"`
	Practice Practice `json:"practice"`
}

type MintResult struct {
	ModuleID string       `json:"module_id"`
	Drafts   []StudyDraft `json:"drafts"`
	Written  int          `json:"written"`
	Applied  int          `json:"applied"`
}

var mintStop = map[string]bool{
	"this": true, "that": true, "with": true, "from": true, "into": true, "have": true,
	"been": true, "they": true, "them": true, "than": true, "then": true, "when": true,
	"what": true, "which": true, "true": true, "also": true, "only": true, "does": true,
	"using": true, "used": true, "each": true, "both": true, "more": true, "must": true,
	"should": true, "about": true, "after": true, "before": true, "under": true, "over": true,
	"such": true, "same": true, "make": true, "made": true, "just": true, "like": true,
	"your": true, "their": true, "there": true, "these": true, "those": true, "being": true,
	"because": true, "while": true, "where": true, "statement": true, "public": true,
	"and": true, "the": true, "not": true, "are": true, "was": true, "were": true,
	"can": true, "may": true, "its": true, "for": true, "of": true, "in": true,
	"on": true, "to": true, "is": true, "it": true, "as": true, "or": true, "an": true,
	"a": true, "onto": true,
}

var cannedDistractors = []string{
	"It requires a hosted SaaS account to function.",
	"It launches the operator personal Chrome install.",
	"An LLM judge is the authoritative scorer.",
	"Private factory telemetry is the documented public API.",
}

func MintModule(cur *Curriculum, moduleID string, now time.Time) ([]StudyDraft, error) {
	if cur == nil {
		return nil, fmt.Errorf("study curriculum is required")
	}
	module, err := cur.Module(moduleID)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	existing := append([]Practice(nil), module.Practices...)
	var drafts []StudyDraft
	lines := append(append([]string{}, module.Architecture...), module.Decisions...)
	for i, line := range lines {
		practice, ok := quizFromLine(module, line, lines, i)
		if !ok || promptTaken(existing, drafts, practice.Prompt, practice.ID) {
			continue
		}
		if err := validateMintedPractice(cur, module.ID, practice); err != nil {
			continue
		}
		drafts = append(drafts, StudyDraft{
			SchemaVersion: DraftSchemaVersion,
			ID:            practice.ID,
			ModuleID:      module.ID,
			Practice:      practice,
			Source:        "teaching-line",
			CreatedAt:     now,
		})
		existing = append(existing, practice)
	}
	if demo, ok := demoFromRun(module); ok && !promptTaken(existing, drafts, demo.Prompt, demo.ID) {
		if err := validateMintedPractice(cur, module.ID, demo); err == nil {
			drafts = append(drafts, StudyDraft{
				SchemaVersion: DraftSchemaVersion,
				ID:            demo.ID,
				ModuleID:      module.ID,
				Practice:      demo,
				Source:        "run_demo",
				CreatedAt:     now,
			})
		}
	}
	return drafts, nil
}

func quizFromLine(module Module, line string, all []string, index int) (Practice, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 24 {
		return Practice{}, false
	}
	others := make([]string, 0, len(all)-1)
	for i, candidate := range all {
		if i != index && strings.TrimSpace(candidate) != "" {
			others = append(others, strings.TrimSpace(candidate))
		}
	}
	concepts := uniqueLineConcepts(line, others)
	if len(concepts) < 2 {
		return Practice{}, false
	}
	choices := []string{line}
	for _, other := range others {
		if len(choices) == 4 {
			break
		}
		if other == line || containsAllConcepts(other, concepts) {
			continue
		}
		choices = append(choices, other)
	}
	for _, canned := range cannedDistractors {
		if len(choices) == 4 {
			break
		}
		if containsAllConcepts(canned, concepts) {
			continue
		}
		choices = append(choices, canned)
	}
	if len(choices) < 2 {
		return Practice{}, false
	}
	sum := sha256.Sum256([]byte(module.ID + "\n" + line))
	return Practice{
		ID:               "mint-" + module.ID + "-" + hex.EncodeToString(sum[:])[:12],
		Kind:             PracticeQuiz,
		Prompt:           fmt.Sprintf("Which statement is true of %s?", module.Name),
		Choices:          choices,
		ExpectedConcepts: concepts,
		PassThreshold:    70,
	}, true
}

func demoFromRun(module Module) (Practice, bool) {
	demo := strings.TrimSpace(module.RunDemo)
	if demo == "" || strings.HasPrefix(strings.ToLower(demo), "no public") {
		return Practice{}, false
	}
	for _, existing := range module.Practices {
		if existing.Kind == PracticeDemo {
			return Practice{}, false
		}
	}
	concepts := uniqueLineConcepts(demo, nil)
	if len(concepts) < 2 {
		return Practice{}, false
	}
	if len(concepts) > 3 {
		concepts = concepts[:3]
	}
	sum := sha256.Sum256([]byte(module.ID + "\nrun_demo\n" + demo))
	return Practice{
		ID:               "mint-" + module.ID + "-demo-" + hex.EncodeToString(sum[:])[:8],
		Kind:             PracticeDemo,
		Prompt:           fmt.Sprintf("Name the run or demo path for %s.", module.Name),
		ExpectedConcepts: concepts,
		PassThreshold:    70,
	}, true
}

func uniqueLineConcepts(line string, others []string) []string {
	otherText := strings.ToLower(strings.Join(others, " "))
	var out []string
	seen := map[string]bool{}
	for _, word := range wordList(strings.ToLower(line)) {
		if len(word) < 4 || mintStop[word] || seen[word] {
			continue
		}
		if otherText != "" && strings.Contains(otherText, word) {
			continue
		}
		seen[word] = true
		out = append(out, word)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func containsAllConcepts(text string, concepts []string) bool {
	lower := strings.ToLower(text)
	for _, concept := range concepts {
		if !strings.Contains(lower, concept) {
			return false
		}
	}
	return true
}

func promptTaken(existing []Practice, drafts []StudyDraft, prompt, id string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	for _, practice := range existing {
		if practice.ID == id || strings.ToLower(practice.Prompt) == p {
			return true
		}
	}
	for _, draft := range drafts {
		if draft.ID == id || strings.ToLower(draft.Practice.Prompt) == p {
			return true
		}
	}
	return false
}

func validateMintedPractice(cur *Curriculum, moduleID string, practice Practice) error {
	clone := *cur
	clone.Modules = append([]Module(nil), cur.Modules...)
	for i := range clone.Modules {
		if clone.Modules[i].ID != moduleID {
			continue
		}
		mod := clone.Modules[i]
		mod.Practices = append(append([]Practice(nil), mod.Practices...), practice)
		clone.Modules[i] = mod
		return ValidateCurriculum(&clone)
	}
	return fmt.Errorf("unknown study module %q", moduleID)
}

func MergeOverlay(cur *Curriculum, overlay StudyOverlay) (*Curriculum, error) {
	if cur == nil {
		return nil, fmt.Errorf("study curriculum is required")
	}
	clone := *cur
	clone.Modules = append([]Module(nil), cur.Modules...)
	byID := map[string]int{}
	for i, module := range clone.Modules {
		byID[module.ID] = i
	}
	for _, item := range overlay.Practices {
		idx, ok := byID[item.ModuleID]
		if !ok {
			return nil, fmt.Errorf("overlay references unknown module %q", item.ModuleID)
		}
		mod := clone.Modules[idx]
		taken := false
		for _, existing := range mod.Practices {
			if existing.ID == item.Practice.ID {
				taken = true
				break
			}
		}
		if taken {
			continue
		}
		mod.Practices = append(append([]Practice(nil), mod.Practices...), item.Practice)
		clone.Modules[idx] = mod
	}
	if err := ValidateCurriculum(&clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func LoadEffectiveCurriculum(store *Store) (*Curriculum, error) {
	cur, err := LoadCurriculum()
	if err != nil {
		return nil, err
	}
	if store == nil {
		return cur, nil
	}
	overlay, err := store.StudyOverlay()
	if err != nil {
		return nil, err
	}
	if len(overlay.Practices) == 0 {
		return cur, nil
	}
	return MergeOverlay(cur, overlay)
}

func (s *Store) StudyDraftsPath() string {
	return filepath.Join(s.Root, "study-drafts.jsonl")
}

func (s *Store) StudyOverlayPath() string {
	return filepath.Join(s.Root, "study-overlay.json")
}

func MintToStore(store *Store, moduleID string, apply, dryRun bool, now time.Time) (*MintResult, error) {
	if store == nil {
		return nil, fmt.Errorf("study store is required")
	}
	cur, err := LoadEffectiveCurriculum(store)
	if err != nil {
		return nil, err
	}
	drafts, err := MintModule(cur, moduleID, now)
	if err != nil {
		return nil, err
	}
	result := &MintResult{ModuleID: moduleID, Drafts: drafts}
	if dryRun {
		return result, nil
	}
	for _, draft := range drafts {
		if err := store.AppendStudyDraft(draft); err != nil {
			return nil, err
		}
		result.Written++
		if apply {
			if _, err := store.PromoteDraft(draft.ID); err != nil {
				return nil, err
			}
			result.Applied++
		}
	}
	return result, nil
}
