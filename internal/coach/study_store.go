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
