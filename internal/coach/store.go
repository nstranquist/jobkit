package coach

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/privatefs"
)

type Store struct {
	Root string
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("coach store root is required")
	}
	store := &Store{Root: root}
	for _, dir := range []string{root, store.DecksDir()} {
		if err := privatefs.EnsureDir(dir); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) SourcePath() string    { return filepath.Join(s.Root, "source.json") }
func (s *Store) DecksDir() string      { return filepath.Join(s.Root, "decks") }
func (s *Store) SessionsPath() string  { return filepath.Join(s.Root, "sessions.jsonl") }
func (s *Store) ProvidersPath() string { return filepath.Join(s.Root, "providers.json") }

func (s *Store) SaveSource(bundle *SourceBundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return privatefs.WriteFile(s.SourcePath(), append(raw, '\n'))
}

func (s *Store) LoadSource() (*SourceBundle, error) {
	return LoadSource(s.SourcePath())
}

func (s *Store) SaveDeck(deck *Deck) error {
	if deck.SchemaVersion != SchemaVersion || deck.ID == "" || filepath.Base(deck.ID) != deck.ID || len(deck.Questions) == 0 {
		return fmt.Errorf("invalid coach deck")
	}
	raw, err := json.MarshalIndent(deck, "", "  ")
	if err != nil {
		return err
	}
	return privatefs.WriteFile(filepath.Join(s.DecksDir(), deck.ID+".json"), append(raw, '\n'))
}

func (s *Store) LoadDeck(id string) (*Deck, error) {
	if id == "latest" {
		decks, err := s.ListDecks()
		if err != nil {
			return nil, err
		}
		if len(decks) == 0 {
			return nil, os.ErrNotExist
		}
		return &decks[0], nil
	}
	raw, err := os.ReadFile(filepath.Join(s.DecksDir(), filepath.Base(id)+".json"))
	if err != nil {
		return nil, err
	}
	var deck Deck
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&deck); err != nil {
		return nil, fmt.Errorf("decode coach deck: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode coach deck: trailing JSON data")
	}
	if deck.SchemaVersion != SchemaVersion || deck.ID != id || len(deck.Questions) == 0 {
		return nil, fmt.Errorf("invalid coach deck %q", id)
	}
	return &deck, nil
}

func (s *Store) ListDecks() ([]Deck, error) {
	entries, err := os.ReadDir(s.DecksDir())
	if err != nil {
		return nil, err
	}
	var decks []Deck
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		deck, err := s.LoadDeck(id)
		if err != nil {
			return nil, err
		}
		decks = append(decks, *deck)
	}
	sort.Slice(decks, func(i, j int) bool {
		return decks[i].CreatedAt.After(decks[j].CreatedAt)
	})
	return decks, nil
}

func (s *Store) AppendSession(session *Session) error {
	if session.SchemaVersion != SchemaVersion || session.RubricVersion != RubricVersion || session.ID == "" || session.DeckID == "" {
		return fmt.Errorf("invalid coach session")
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	f, err := privatefs.OpenAppend(s.SessionsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *Store) Sessions() ([]Session, error) {
	f, err := os.Open(s.SessionsPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var sessions []Session
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		var session Session
		if err := json.Unmarshal(scanner.Bytes(), &session); err != nil {
			return nil, fmt.Errorf("decode coach session line %d: %w", line, err)
		}
		sessions = append(sessions, session)
	}
	return sessions, scanner.Err()
}

type Stats struct {
	Sessions        int             `json:"sessions"`
	AverageScore    float64         `json:"average_score"`
	ClaimViolations int             `json:"claim_violations"`
	DueReviews      int             `json:"due_reviews"`
	ByProject       map[string]Band `json:"by_project,omitempty"`
	ByMode          map[Mode]Band   `json:"by_mode,omitempty"`
	ByRubric        map[string]int  `json:"by_rubric,omitempty"`
}

type Band struct {
	Answers int     `json:"answers"`
	Average float64 `json:"average"`
}

func (s *Store) Stats(now time.Time, projectFilter string) (*Stats, error) {
	sessions, err := s.Sessions()
	if err != nil {
		return nil, err
	}
	report := &Stats{ByProject: map[string]Band{}, ByMode: map[Mode]Band{}, ByRubric: map[string]int{}}
	total := 0
	projectTotals := map[string]int{}
	modeTotals := map[Mode]int{}
	for _, session := range sessions {
		deck, err := s.LoadDeck(session.DeckID)
		if err != nil {
			return nil, err
		}
		if projectFilter != "" && !contains(deck.ProjectIDs, projectFilter) {
			continue
		}
		report.Sessions++
		rubric := session.RubricVersion
		if rubric == "" {
			rubric = "legacy"
		}
		report.ByRubric[rubric]++
		total += session.Score
		report.ClaimViolations += session.ClaimViolations
		if !session.NextReviewAt.After(now) {
			report.DueReviews++
		}
		for _, result := range session.Results {
			question := questionByID(deck, result.QuestionID)
			modeBand := report.ByMode[question.Mode]
			modeBand.Answers++
			modeTotals[question.Mode] += result.Score.Total
			report.ByMode[question.Mode] = modeBand
			if question.ProjectID != "" {
				projectBand := report.ByProject[question.ProjectID]
				projectBand.Answers++
				projectTotals[question.ProjectID] += result.Score.Total
				report.ByProject[question.ProjectID] = projectBand
			}
		}
	}
	if report.Sessions > 0 {
		report.AverageScore = roundOne(float64(total) / float64(report.Sessions))
	}
	for mode, band := range report.ByMode {
		band.Average = roundOne(float64(modeTotals[mode]) / float64(band.Answers))
		report.ByMode[mode] = band
	}
	for project, band := range report.ByProject {
		band.Average = roundOne(float64(projectTotals[project]) / float64(band.Answers))
		report.ByProject[project] = band
	}
	return report, nil
}

func questionByID(deck *Deck, id string) Question {
	for _, question := range deck.Questions {
		if question.ID == id {
			return question
		}
	}
	return Question{}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func roundOne(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}
