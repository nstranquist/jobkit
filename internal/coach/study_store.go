package coach

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nstranquist/jobkit/internal/claims"
	"github.com/nstranquist/jobkit/internal/privatefs"
)

type StudyResult struct {
	SchemaVersion   int                `json:"schema_version"`
	ModuleID        string             `json:"module_id"`
	PracticeID      string             `json:"practice_id"`
	Kind            PracticeKind       `json:"kind,omitempty"`
	Answer          string             `json:"answer,omitempty"`
	Score           int                `json:"score"`
	Passed          bool               `json:"passed"`
	Verdict         string             `json:"verdict"`
	CoveredConcepts []string           `json:"covered_concepts,omitempty"`
	MissingConcepts []string           `json:"missing_concepts,omitempty"`
	ClaimViolations []claims.Violation `json:"claim_violations,omitempty"`
	CompletedAt     time.Time          `json:"completed_at"`
}

func (s *Store) StudyResultsPath() string {
	return filepath.Join(s.Root, "study-results.jsonl")
}

func (s *Store) AppendStudyResult(result StudyResult) error {
	if result.SchemaVersion != StudySchemaVersion || result.ModuleID == "" || result.PracticeID == "" || result.Verdict == "" {
		return fmt.Errorf("invalid study result")
	}
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	f, err := privatefs.OpenAppend(s.StudyResultsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) StudyResults() ([]StudyResult, error) {
	f, err := os.Open(s.StudyResultsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var results []StudyResult
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var result StudyResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			return nil, fmt.Errorf("decode study result line %d: %w", line, err)
		}
		results = append(results, result)
	}
	return results, scanner.Err()
}

func (s *Store) AppendStudyDraft(draft StudyDraft) error {
	if draft.SchemaVersion != DraftSchemaVersion || draft.ID == "" || draft.ModuleID == "" || draft.Practice.ID == "" {
		return fmt.Errorf("invalid study draft")
	}
	existing, err := s.StudyDrafts()
	if err != nil {
		return err
	}
	for _, row := range existing {
		if row.ID == draft.ID {
			return nil
		}
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	f, err := privatefs.OpenAppend(s.StudyDraftsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) StudyDrafts() ([]StudyDraft, error) {
	f, err := os.Open(s.StudyDraftsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var drafts []StudyDraft
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var draft StudyDraft
		if err := json.Unmarshal(scanner.Bytes(), &draft); err != nil {
			return nil, fmt.Errorf("decode study draft line %d: %w", line, err)
		}
		drafts = append(drafts, draft)
	}
	return drafts, scanner.Err()
}

func (s *Store) StudyOverlay() (StudyOverlay, error) {
	raw, err := os.ReadFile(s.StudyOverlayPath())
	if errors.Is(err, os.ErrNotExist) {
		return StudyOverlay{SchemaVersion: StudySchemaVersion}, nil
	}
	if err != nil {
		return StudyOverlay{}, err
	}
	var overlay StudyOverlay
	if err := json.Unmarshal(raw, &overlay); err != nil {
		return StudyOverlay{}, fmt.Errorf("decode study overlay: %w", err)
	}
	if overlay.SchemaVersion == 0 {
		overlay.SchemaVersion = StudySchemaVersion
	}
	return overlay, nil
}

func (s *Store) PromoteDraft(id string) (StudyDraft, error) {
	drafts, err := s.StudyDrafts()
	if err != nil {
		return StudyDraft{}, err
	}
	var found StudyDraft
	ok := false
	for _, draft := range drafts {
		if draft.ID == id {
			found = draft
			ok = true
			break
		}
	}
	if !ok {
		return StudyDraft{}, fmt.Errorf("unknown study draft %q", id)
	}
	overlay, err := s.StudyOverlay()
	if err != nil {
		return StudyDraft{}, err
	}
	for _, item := range overlay.Practices {
		if item.Practice.ID == found.Practice.ID {
			return found, nil
		}
	}
	overlay.SchemaVersion = StudySchemaVersion
	overlay.Practices = append(overlay.Practices, OverlayPractice{ModuleID: found.ModuleID, Practice: found.Practice})
	raw, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return StudyDraft{}, err
	}
	if err := privatefs.WriteFile(s.StudyOverlayPath(), append(raw, '\n')); err != nil {
		return StudyDraft{}, err
	}
	return found, nil
}
