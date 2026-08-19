// Study mode teaches hiring-pin OSS courses plus extra honest OSS modules and
// scores practice attempts deterministically on the existing Coach surface.
package coach

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/claims"
)

// ErrStudyComplete is returned when every pin practice already has a pass.
var ErrStudyComplete = errors.New("all pin practices are complete")

// LookupError is an unknown module or practice id.
type LookupError struct {
	Kind string
	ID   string
}

func (e *LookupError) Error() string {
	if e == nil {
		return "unknown study item"
	}
	if e.Kind == "practice" {
		return fmt.Sprintf("unknown study practice %s", e.ID)
	}
	return fmt.Sprintf("unknown study module %q", e.ID)
}

func IsLookup(err error) bool {
	var lookup *LookupError
	return errors.As(err, &lookup)
}

//go:embed curriculum.json
var shippedCurriculumJSON []byte

// RequiredPinIDs is the current job-search pin order from the 2026-08-12
// JobKit stack decision. Hidden Bar and other P2 extracts are not required.
var RequiredPinIDs = []string{
	"docs-puller",
	"nicos-catalog",
	"openbook",
	"agent-ops",
	"keepawake",
	"jobkit",
}

const (
	StudySchemaVersion = 1
	defaultPassMark    = 70
)

// Allowed claim authorities for quantified teaching content.
const (
	AuthorityClaimsLock      = "claims-lock"
	AuthorityPublicREADME    = "public-readme"
	AuthorityPublicCaseStudy = "public-case-study"
)

type Curriculum struct {
	SchemaVersion int          `json:"schema_version"`
	ID            string       `json:"id"`
	Order         []string     `json:"order"`
	Claims        []StudyClaim `json:"claims"`
	Modules       []Module     `json:"modules"`
}

type StudyClaim struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Authority string `json:"authority"`
	Locator   string `json:"locator"`
}

type Module struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Track         string     `json:"track,omitempty"`
	Purpose       string     `json:"purpose"`
	Architecture  []string   `json:"architecture"`
	Decisions     []string   `json:"decisions"`
	RunDemo       string     `json:"run_demo"`
	TalkingPoints []string   `json:"talking_points"`
	ClaimIDs      []string   `json:"claim_ids"`
	Practices     []Practice `json:"practices"`
}

type PracticeKind string

const (
	PracticeExplain PracticeKind = "explain"
	PracticeDemo    PracticeKind = "demo"
	PracticeDefend  PracticeKind = "defend"
	PracticeQuiz    PracticeKind = "quiz"
	PracticeRecall  PracticeKind = "recall"
)

type Practice struct {
	ID               string       `json:"id"`
	Kind             PracticeKind `json:"kind"`
	Prompt           string       `json:"prompt"`
	Choices          []string     `json:"choices,omitempty"`
	Hint             string       `json:"hint,omitempty"`
	ExpectedConcepts []string     `json:"expected_concepts"`
	ClaimIDs         []string     `json:"claim_ids,omitempty"`
	PassThreshold    int          `json:"pass_threshold"`
}

type PracticeScore struct {
	ModuleID        string             `json:"module_id"`
	PracticeID      string             `json:"practice_id"`
	Kind            PracticeKind       `json:"kind"`
	Score           int                `json:"score"`
	Passed          bool               `json:"passed"`
	Threshold       int                `json:"threshold"`
	CoveredConcepts []string           `json:"covered_concepts,omitempty"`
	MissingConcepts []string           `json:"missing_concepts,omitempty"`
	ClaimViolations []claims.Violation `json:"claim_violations,omitempty"`
	Verdict         string             `json:"verdict"`
}

type StudyItem struct {
	ModuleID   string       `json:"module_id"`
	ModuleName string       `json:"module_name"`
	PracticeID string       `json:"practice_id"`
	Kind       PracticeKind `json:"kind"`
	Prompt     string       `json:"prompt"`
	Ordinal    int          `json:"ordinal"`
	Remaining  int          `json:"remaining"`
}

type ModuleSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
	Practices int    `json:"practices"`
	Passed    int    `json:"passed"`
	Complete  bool   `json:"complete"`
}

type ModuleView struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Track         string     `json:"track,omitempty"`
	Purpose       string     `json:"purpose"`
	Architecture  []string   `json:"architecture"`
	Decisions     []string   `json:"decisions"`
	RunDemo       string     `json:"run_demo"`
	TalkingPoints []string   `json:"talking_points"`
	ClaimIDs      []string   `json:"claim_ids,omitempty"`
	Practices     []Practice `json:"practices"`
}

type HistoryItem struct {
	ModuleID    string    `json:"module_id"`
	PracticeID  string    `json:"practice_id"`
	Score       int       `json:"score"`
	Passed      bool      `json:"passed"`
	Verdict     string    `json:"verdict"`
	CompletedAt time.Time `json:"completed_at"`
}

type StudyReport struct {
	Modules []ModuleSummary `json:"modules"`
	Bank    []ModuleView    `json:"bank,omitempty"`
	History []HistoryItem   `json:"history,omitempty"`
	Module  *ModuleView     `json:"module,omitempty"`
	Attempt *PracticeScore  `json:"attempt,omitempty"`
	Next    *StudyItem      `json:"next,omitempty"`
}

type LaunchOptions struct {
	ModuleID   string
	PracticeID string
	DraftID    string
	Answer     string
	Now        time.Time
}

type ClaimTraceRow struct {
	ModuleID  string `json:"module_id"`
	Token     string `json:"token"`
	ClaimID   string `json:"claim_id"`
	Authority string `json:"authority"`
	Locator   string `json:"locator"`
	Context   string `json:"context,omitempty"`
}

func LoadCurriculum() (*Curriculum, error) {
	return ParseCurriculum(shippedCurriculumJSON)
}

func ParseCurriculum(raw []byte) (*Curriculum, error) {
	var cur Curriculum
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cur); err != nil {
		return nil, fmt.Errorf("decode study curriculum: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode study curriculum: trailing JSON data")
	}
	if err := ValidateCurriculum(&cur); err != nil {
		return nil, err
	}
	return &cur, nil
}

func ValidateCurriculum(cur *Curriculum) error {
	if cur == nil {
		return fmt.Errorf("study curriculum is required")
	}
	if cur.SchemaVersion != StudySchemaVersion {
		return fmt.Errorf("study curriculum schema_version=%d; supported version is %d", cur.SchemaVersion, StudySchemaVersion)
	}
	if strings.TrimSpace(cur.ID) == "" {
		return fmt.Errorf("study curriculum id is required")
	}
	if len(cur.Modules) == 0 {
		return fmt.Errorf("study curriculum needs at least one module")
	}
	claimsByID := map[string]StudyClaim{}
	for _, claim := range cur.Claims {
		id := strings.TrimSpace(claim.ID)
		if id == "" || strings.TrimSpace(claim.Text) == "" {
			return fmt.Errorf("study claim needs id and text")
		}
		switch claim.Authority {
		case AuthorityClaimsLock, AuthorityPublicREADME, AuthorityPublicCaseStudy:
		default:
			return fmt.Errorf("study claim %q has unknown authority %q", id, claim.Authority)
		}
		if err := validatePublicURL(claim.Locator); err != nil {
			return fmt.Errorf("study claim %q: %w", id, err)
		}
		if _, exists := claimsByID[id]; exists {
			return fmt.Errorf("study claim %q is repeated", id)
		}
		claimsByID[id] = claim
	}
	byID := map[string]Module{}
	for _, module := range cur.Modules {
		id := strings.TrimSpace(module.ID)
		if id == "" || strings.TrimSpace(module.Name) == "" || strings.TrimSpace(module.Purpose) == "" {
			return fmt.Errorf("study module needs id, name, and purpose")
		}
		if _, exists := byID[id]; exists {
			return fmt.Errorf("study module %q is repeated", id)
		}
		if len(module.Architecture) == 0 && len(module.Decisions) == 0 {
			return fmt.Errorf("study module %q needs architecture or decisions", id)
		}
		if strings.TrimSpace(module.RunDemo) == "" {
			return fmt.Errorf("study module %q needs a run/demo path", id)
		}
		if len(module.Practices) == 0 {
			return fmt.Errorf("study module %q needs at least one practice", id)
		}
		for _, claimID := range module.ClaimIDs {
			if _, ok := claimsByID[claimID]; !ok {
				return fmt.Errorf("study module %q references unknown claim %q", id, claimID)
			}
		}
		seenPractice := map[string]bool{}
		for _, practice := range module.Practices {
			if strings.TrimSpace(practice.ID) == "" || strings.TrimSpace(practice.Prompt) == "" {
				return fmt.Errorf("study module %q has a practice without id or prompt", id)
			}
			if seenPractice[practice.ID] {
				return fmt.Errorf("study module %q repeats practice %q", id, practice.ID)
			}
			seenPractice[practice.ID] = true
			switch practice.Kind {
			case PracticeExplain, PracticeDemo, PracticeDefend, PracticeQuiz, PracticeRecall:
			default:
				return fmt.Errorf("study practice %s/%s has unknown kind %q", id, practice.ID, practice.Kind)
			}
			if practice.Kind == PracticeQuiz && len(practice.Choices) < 2 {
				return fmt.Errorf("study practice %s/%s quiz needs at least two choices", id, practice.ID)
			}
			if len(uniqueLower(append([]string(nil), practice.ExpectedConcepts...))) == 0 {
				return fmt.Errorf("study practice %s/%s needs expected concepts", id, practice.ID)
			}
			if practice.PassThreshold < 1 || practice.PassThreshold > 100 {
				return fmt.Errorf("study practice %s/%s pass_threshold must be 1-100", id, practice.ID)
			}
			for _, claimID := range practice.ClaimIDs {
				if _, ok := claimsByID[claimID]; !ok {
					return fmt.Errorf("study practice %s/%s references unknown claim %q", id, practice.ID, claimID)
				}
			}
		}
		if err := rejectUntraceableQuantities(module, claimsByID); err != nil {
			return err
		}
		byID[id] = module
	}
	if len(cur.Order) == 0 {
		return fmt.Errorf("study curriculum order is required")
	}
	seenOrder := map[string]bool{}
	for _, id := range cur.Order {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("study curriculum order references unknown module %q", id)
		}
		if seenOrder[id] {
			return fmt.Errorf("study curriculum order repeats %q", id)
		}
		seenOrder[id] = true
	}
	if cur.ID == "job-search-oss-pins" {
		if len(cur.Order) < len(RequiredPinIDs) {
			return fmt.Errorf("shipped pin curriculum must start with %d hiring pins", len(RequiredPinIDs))
		}
		for i, id := range RequiredPinIDs {
			if cur.Order[i] != id {
				return fmt.Errorf("shipped pin order[%d]=%q, want %q", i, cur.Order[i], id)
			}
		}
	}
	if problems := studySafetyProblems(cur); len(problems) > 0 {
		return fmt.Errorf("study curriculum is not public-safe: %s", strings.Join(problems, ", "))
	}
	return nil
}

func rejectUntraceableQuantities(module Module, claimsByID map[string]StudyClaim) error {
	allowed := allowedClaimTexts(module, Practice{}, claimsByID)
	text := moduleTeachingText(module)
	violations := claims.Check(text, allowed)
	if len(violations) == 0 {
		return nil
	}
	parts := make([]string, 0, len(violations))
	for _, v := range violations {
		parts = append(parts, v.Token)
	}
	return fmt.Errorf("study module %q has untraceable quantified claim(s): %s", module.ID, strings.Join(parts, ", "))
}

func moduleTeachingText(module Module) string {
	var b strings.Builder
	b.WriteString(module.Purpose)
	b.WriteByte('\n')
	b.WriteString(module.RunDemo)
	b.WriteByte('\n')
	for _, line := range module.Architecture {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range module.Decisions {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range module.TalkingPoints {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, practice := range module.Practices {
		b.WriteString(practice.Prompt)
		b.WriteByte('\n')
		b.WriteString(strings.Join(practice.ExpectedConcepts, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func studySafetyProblems(cur *Curriculum) []string {
	raw, _ := json.Marshal(cur)
	lower := strings.ToLower(string(raw))
	markers := []string{
		"/users/", `c:\\users\\`, "file://", "~/", ".jobkit/",
		"nicos-tools", "private-admin-evidence", "edurain", "pw-harness",
		"garrid dispatcher", "valuation layer",
	}
	var found []string
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			found = append(found, marker)
		}
	}
	if publicEmailRE.Match(raw) {
		found = append(found, "email-address")
	}
	if publicPhoneRE.Match(raw) {
		found = append(found, "phone-number")
	}
	sort.Strings(found)
	return found
}

func (c *Curriculum) Module(id string) (Module, error) {
	id = strings.TrimSpace(id)
	for _, module := range c.OrderedModules() {
		if module.ID == id {
			return module, nil
		}
	}
	return Module{}, &LookupError{Kind: "module", ID: id}
}

func (c *Curriculum) OrderedModules() []Module {
	byID := map[string]Module{}
	for _, module := range c.Modules {
		byID[module.ID] = module
	}
	out := make([]Module, 0, len(c.Order))
	for _, id := range c.Order {
		if module, ok := byID[id]; ok {
			out = append(out, module)
		}
	}
	return out
}

func (c *Curriculum) Practice(moduleID, practiceID string) (Module, Practice, error) {
	module, err := c.Module(moduleID)
	if err != nil {
		return Module{}, Practice{}, err
	}
	if strings.TrimSpace(practiceID) == "" {
		if len(module.Practices) == 0 {
			return Module{}, Practice{}, fmt.Errorf("study module %q has no practices", moduleID)
		}
		return module, module.Practices[0], nil
	}
	for _, practice := range module.Practices {
		if practice.ID == practiceID {
			return module, practice, nil
		}
	}
	return Module{}, Practice{}, &LookupError{Kind: "practice", ID: moduleID + "/" + practiceID}
}

func (c *Curriculum) ClaimMap() map[string]StudyClaim {
	out := make(map[string]StudyClaim, len(c.Claims))
	for _, claim := range c.Claims {
		out[claim.ID] = claim
	}
	return out
}

func allowedClaimTexts(module Module, practice Practice, claimsByID map[string]StudyClaim) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ids []string) {
		for _, id := range ids {
			claim, ok := claimsByID[id]
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, claim.Text)
		}
	}
	add(module.ClaimIDs)
	add(practice.ClaimIDs)
	return out
}

func ScorePractice(module Module, practice Practice, answer string, claimsByID map[string]StudyClaim) PracticeScore {
	threshold := practice.PassThreshold
	if threshold <= 0 {
		threshold = defaultPassMark
	}
	covered, missing := partitionCoverage(strings.ToLower(answer), practice.ExpectedConcepts)
	total := len(uniqueLower(append([]string(nil), practice.ExpectedConcepts...)))
	score := scaledFraction(len(covered), total, 100)
	violations := claims.Check(answer, allowedClaimTexts(module, practice, claimsByID))
	dump := dumpSensitive(practice.Kind) && isConceptDump(answer, practice.ExpectedConcepts)
	passed := score >= threshold && len(violations) == 0 && !dump
	verdict := "fail"
	switch {
	case len(violations) > 0:
		verdict = "claim_rejected"
		passed = false
	case dump:
		verdict = "dump"
		passed = false
	case passed:
		verdict = "pass"
	}
	return PracticeScore{
		ModuleID:        module.ID,
		PracticeID:      practice.ID,
		Kind:            practice.Kind,
		Score:           score,
		Passed:          passed,
		Threshold:       threshold,
		CoveredConcepts: covered,
		MissingConcepts: missing,
		ClaimViolations: violations,
		Verdict:         verdict,
	}
}

func NextIncomplete(cur *Curriculum, results []StudyResult) *StudyItem {
	passed := passedSet(results)
	ordinal := 0
	remaining := 0
	var first *StudyItem
	for _, module := range cur.OrderedModules() {
		for _, practice := range module.Practices {
			ordinal++
			key := practiceKey(module.ID, practice.ID)
			if passed[key] {
				continue
			}
			remaining++
			if first == nil {
				item := StudyItem{
					ModuleID:   module.ID,
					ModuleName: module.Name,
					PracticeID: practice.ID,
					Kind:       practice.Kind,
					Prompt:     practice.Prompt,
					Ordinal:    ordinal,
				}
				first = &item
			}
		}
	}
	if first != nil {
		first.Remaining = remaining
	}
	return first
}

func SummarizeModules(cur *Curriculum, results []StudyResult) []ModuleSummary {
	passed := passedSet(results)
	out := make([]ModuleSummary, 0, len(cur.Order))
	for i, module := range cur.OrderedModules() {
		summary := ModuleSummary{
			ID:        module.ID,
			Name:      module.Name,
			Order:     i + 1,
			Practices: len(module.Practices),
		}
		for _, practice := range module.Practices {
			if passed[practiceKey(module.ID, practice.ID)] {
				summary.Passed++
			}
		}
		summary.Complete = summary.Practices > 0 && summary.Passed == summary.Practices
		out = append(out, summary)
	}
	return out
}

func ViewModule(module Module) ModuleView {
	return ModuleView{
		ID:            module.ID,
		Name:          module.Name,
		Track:         module.Track,
		Purpose:       module.Purpose,
		Architecture:  append([]string(nil), module.Architecture...),
		Decisions:     append([]string(nil), module.Decisions...),
		RunDemo:       module.RunDemo,
		TalkingPoints: append([]string(nil), module.TalkingPoints...),
		ClaimIDs:      append([]string(nil), module.ClaimIDs...),
		Practices:     append([]Practice(nil), module.Practices...),
	}
}

func Launch(store *Store, opts LaunchOptions) (*StudyReport, error) {
	cur, err := LoadEffectiveCurriculum(store)
	if err != nil {
		return nil, err
	}
	return LaunchCurriculum(store, cur, opts)
}

func LaunchCurriculum(store *Store, cur *Curriculum, opts LaunchOptions) (*StudyReport, error) {
	if cur == nil {
		return nil, fmt.Errorf("study curriculum is required")
	}
	if store == nil {
		return nil, fmt.Errorf("study store is required")
	}
	results, err := store.StudyResults()
	if err != nil {
		return nil, err
	}
	report := &StudyReport{
		Modules: SummarizeModules(cur, results),
		History: compactHistory(results),
		Next:    NextIncomplete(cur, results),
	}
	if strings.TrimSpace(opts.ModuleID) == "" && strings.TrimSpace(opts.Answer) == "" {
		for _, module := range cur.OrderedModules() {
			report.Bank = append(report.Bank, ViewModule(module))
		}
	}
	if opts.ModuleID != "" {
		module, err := cur.Module(opts.ModuleID)
		if err != nil {
			return nil, err
		}
		view := ViewModule(module)
		report.Module = &view
	}
	if strings.TrimSpace(opts.Answer) == "" {
		return report, nil
	}
	if draftID := strings.TrimSpace(opts.DraftID); draftID != "" {
		drafts, err := store.StudyDrafts()
		if err != nil {
			return nil, err
		}
		var found *StudyDraft
		for i := range drafts {
			if drafts[i].ID == draftID {
				found = &drafts[i]
				break
			}
		}
		if found == nil {
			return nil, &LookupError{Kind: "practice", ID: draftID}
		}
		module, err := cur.Module(found.ModuleID)
		if err != nil {
			return nil, err
		}
		opts.ModuleID = found.ModuleID
		opts.PracticeID = found.Practice.ID
		if opts.Now.IsZero() {
			opts.Now = time.Now().UTC()
		}
		score := ScorePractice(module, found.Practice, opts.Answer, cur.ClaimMap())
		score.PracticeID = found.Practice.ID
		record := StudyResult{
			SchemaVersion:   StudySchemaVersion,
			ModuleID:        score.ModuleID,
			PracticeID:      score.PracticeID,
			Kind:            score.Kind,
			Answer:          opts.Answer,
			Score:           score.Score,
			Passed:          score.Passed,
			Verdict:         score.Verdict,
			CoveredConcepts: score.CoveredConcepts,
			MissingConcepts: score.MissingConcepts,
			ClaimViolations: score.ClaimViolations,
			CompletedAt:     opts.Now.UTC(),
		}
		if err := store.AppendStudyResult(record); err != nil {
			return nil, err
		}
		results, err = store.StudyResults()
		if err != nil {
			return nil, err
		}
		view := ViewModule(module)
		view.Practices = append(view.Practices, found.Practice)
		report.Modules = SummarizeModules(cur, results)
		report.History = compactHistory(results)
		report.Module = &view
		report.Attempt = &score
		report.Next = NextIncomplete(cur, results)
		return report, nil
	}
	moduleID := strings.TrimSpace(opts.ModuleID)
	practiceID := strings.TrimSpace(opts.PracticeID)
	if moduleID == "" {
		if report.Next == nil {
			return nil, ErrStudyComplete
		}
		moduleID = report.Next.ModuleID
		practiceID = report.Next.PracticeID
	} else if practiceID == "" {
		if id := firstIncompleteInModule(cur, moduleID, results); id != "" {
			practiceID = id
		}
	}
	module, practice, err := cur.Practice(moduleID, practiceID)
	if err != nil {
		return nil, err
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	score := ScorePractice(module, practice, opts.Answer, cur.ClaimMap())
	record := StudyResult{
		SchemaVersion:   StudySchemaVersion,
		ModuleID:        score.ModuleID,
		PracticeID:      score.PracticeID,
		Kind:            score.Kind,
		Answer:          opts.Answer,
		Score:           score.Score,
		Passed:          score.Passed,
		Verdict:         score.Verdict,
		CoveredConcepts: score.CoveredConcepts,
		MissingConcepts: score.MissingConcepts,
		ClaimViolations: score.ClaimViolations,
		CompletedAt:     opts.Now.UTC(),
	}
	if err := store.AppendStudyResult(record); err != nil {
		return nil, err
	}
	results, err = store.StudyResults()
	if err != nil {
		return nil, err
	}
	view := ViewModule(module)
	report.Modules = SummarizeModules(cur, results)
	report.History = compactHistory(results)
	report.Module = &view
	report.Attempt = &score
	report.Next = NextIncomplete(cur, results)
	return report, nil
}

func ClaimTrace(cur *Curriculum) []ClaimTraceRow {
	if cur == nil {
		return nil
	}
	claimsByID := cur.ClaimMap()
	var rows []ClaimTraceRow
	for _, module := range cur.OrderedModules() {
		allowedIDs := append([]string(nil), module.ClaimIDs...)
		for _, practice := range module.Practices {
			allowedIDs = append(allowedIDs, practice.ClaimIDs...)
		}
		text := moduleTeachingText(module)
		extracted := claims.Extract(text)
		if len(extracted) == 0 {
			rows = append(rows, ClaimTraceRow{ModuleID: module.ID, Token: "(none)", ClaimID: "", Authority: ""})
			continue
		}
		for _, v := range extracted {
			row := ClaimTraceRow{ModuleID: module.ID, Token: v.Token, Context: v.Context}
			for _, id := range unique(allowedIDs) {
				claim, ok := claimsByID[id]
				if !ok {
					continue
				}
				if len(claims.Check(v.Token, []string{claim.Text})) == 0 {
					row.ClaimID = claim.ID
					row.Authority = claim.Authority
					row.Locator = claim.Locator
					break
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func dumpSensitive(kind PracticeKind) bool {
	switch kind {
	case PracticeExplain, PracticeRecall, PracticeDefend:
		return true
	default:
		return false
	}
}

func isConceptDump(answer string, concepts []string) bool {
	answerWords := wordList(strings.ToLower(answer))
	if len(answerWords) == 0 {
		return false
	}
	conceptWords := map[string]bool{}
	for _, concept := range uniqueLower(append([]string(nil), concepts...)) {
		for _, word := range wordList(concept) {
			conceptWords[word] = true
		}
	}
	if len(conceptWords) == 0 {
		return false
	}
	for _, word := range answerWords {
		if !conceptWords[word] {
			return false
		}
	}
	return true
}

func wordList(s string) []string {
	var b strings.Builder
	var out []string
	flush := func() {
		w := strings.Trim(b.String(), ".,;:!?\"'")
		b.Reset()
		if w != "" {
			out = append(out, w)
		}
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '@', r == '%', r == '+', r == '-':
			b.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func compactHistory(results []StudyResult) []HistoryItem {
	out := make([]HistoryItem, 0, len(results))
	for _, result := range results {
		out = append(out, HistoryItem{
			ModuleID:    result.ModuleID,
			PracticeID:  result.PracticeID,
			Score:       result.Score,
			Passed:      result.Passed,
			Verdict:     result.Verdict,
			CompletedAt: result.CompletedAt,
		})
	}
	return out
}

func firstIncompleteInModule(cur *Curriculum, moduleID string, results []StudyResult) string {
	module, err := cur.Module(moduleID)
	if err != nil {
		return ""
	}
	passed := passedSet(results)
	for _, practice := range module.Practices {
		if !passed[practiceKey(module.ID, practice.ID)] {
			return practice.ID
		}
	}
	return ""
}

func passedSet(results []StudyResult) map[string]bool {
	out := map[string]bool{}
	for _, result := range results {
		if result.Passed {
			out[practiceKey(result.ModuleID, result.PracticeID)] = true
		}
	}
	return out
}

func practiceKey(moduleID, practiceID string) string {
	return moduleID + "/" + practiceID
}
