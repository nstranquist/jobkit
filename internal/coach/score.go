package coach

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/claims"
)

type Answer struct {
	QuestionID string `json:"question_id"`
	Text       string `json:"text"`
}

type ScoreBreakdown struct {
	Structure         int `json:"structure"`
	DecisionsTradeoff int `json:"decisions_tradeoffs"`
	RoleRelevance     int `json:"role_relevance"`
	EvidenceSafety    int `json:"evidence_safety"`
	Concision         int `json:"concision"`
	Total             int `json:"total"`
}

type QuestionResult struct {
	QuestionID      string             `json:"question_id"`
	Score           ScoreBreakdown     `json:"score"`
	CoveredConcepts []string           `json:"covered_concepts,omitempty"`
	MissingConcepts []string           `json:"missing_concepts,omitempty"`
	ClaimViolations []claims.Violation `json:"claim_violations,omitempty"`
}

type ProviderFeedback struct {
	Provider  string   `json:"provider"`
	Model     string   `json:"model,omitempty"`
	Advisory  bool     `json:"advisory"`
	Summary   string   `json:"summary"`
	FollowUps []string `json:"follow_ups,omitempty"`
	TokensIn  int      `json:"tokens_in,omitempty"`
	TokensOut int      `json:"tokens_out,omitempty"`
	CostUSD   float64  `json:"cost_usd,omitempty"`
}

type Session struct {
	SchemaVersion   int               `json:"schema_version"`
	ID              string            `json:"id"`
	DeckID          string            `json:"deck_id"`
	StartedAt       time.Time         `json:"started_at"`
	CompletedAt     time.Time         `json:"completed_at"`
	Answers         []Answer          `json:"answers"`
	Results         []QuestionResult  `json:"results"`
	Score           int               `json:"score"`
	ClaimViolations int               `json:"claim_violations"`
	NextReviewAt    time.Time         `json:"next_review_at"`
	Feedback        *ProviderFeedback `json:"feedback,omitempty"`
	ProviderError   string            `json:"provider_error,omitempty"`
	Useful          *bool             `json:"useful,omitempty"`
}

// ValidateSessionInput rejects stale decks and malformed answer sets before a
// session is scored or stored. Empty answer text is valid because an honest
// missed answer is useful practice data.
func ValidateSessionInput(deck *Deck, bundle *SourceBundle, answers []Answer) error {
	if deck == nil {
		return fmt.Errorf("coach deck is required")
	}
	if bundle == nil {
		return fmt.Errorf("coach source is required")
	}
	if err := bundle.Validate(); err != nil {
		return err
	}
	currentDigest := bundle.Digest()
	if deck.SourceDigest != currentDigest {
		return fmt.Errorf("coach deck %q uses stale source %s; current source is %s", deck.ID, deck.SourceDigest, currentDigest)
	}
	questionIDs := make(map[string]struct{}, len(deck.Questions))
	for _, question := range deck.Questions {
		if question.ID == "" {
			return fmt.Errorf("coach deck %q has a question without an id", deck.ID)
		}
		if _, exists := questionIDs[question.ID]; exists {
			return fmt.Errorf("coach deck %q repeats question %q", deck.ID, question.ID)
		}
		questionIDs[question.ID] = struct{}{}
	}
	answered := make(map[string]struct{}, len(answers))
	for _, answer := range answers {
		if _, exists := questionIDs[answer.QuestionID]; !exists {
			return fmt.Errorf("coach answer references unknown question %q", answer.QuestionID)
		}
		if _, exists := answered[answer.QuestionID]; exists {
			return fmt.Errorf("coach answers repeat question %q", answer.QuestionID)
		}
		answered[answer.QuestionID] = struct{}{}
	}
	for _, question := range deck.Questions {
		if _, exists := answered[question.ID]; !exists {
			return fmt.Errorf("coach answers are missing question %q", question.ID)
		}
	}
	return nil
}

func Evaluate(deck *Deck, bundle *SourceBundle, answers []Answer, started, completed time.Time) *Session {
	if started.IsZero() {
		started = completed
	}
	if completed.IsZero() {
		completed = time.Now().UTC()
	}
	byID := map[string]string{}
	for _, answer := range answers {
		byID[answer.QuestionID] = answer.Text
	}
	var results []QuestionResult
	total := 0
	violationCount := 0
	for _, question := range deck.Questions {
		result := scoreAnswer(question, deck.RoleKeywords, byID[question.ID], allowedClaimsForQuestion(bundle, question.ClaimIDs))
		results = append(results, result)
		total += result.Score.Total
		violationCount += len(result.ClaimViolations)
	}
	if len(results) > 0 {
		total = int(math.Round(float64(total) / float64(len(results))))
	}
	next := completed.Add(reviewInterval(total, violationCount))
	session := &Session{
		SchemaVersion: SchemaVersion,
		DeckID:        deck.ID, StartedAt: started.UTC(), CompletedAt: completed.UTC(),
		Answers: answers, Results: results, Score: total,
		ClaimViolations: violationCount, NextReviewAt: next.UTC(),
	}
	session.ID = sessionID(session)
	return session
}

func allowedClaimsForQuestion(bundle *SourceBundle, claimIDs []string) []string {
	wanted := make(map[string]bool, len(claimIDs))
	for _, id := range claimIDs {
		wanted[id] = true
	}
	out := make([]string, 0, len(claimIDs))
	for _, claim := range bundle.Claims {
		if wanted[claim.ID] {
			out = append(out, claim.Text)
		}
	}
	return out
}

func scoreAnswer(question Question, roleKeywords []string, answer string, allowedClaims []string) QuestionResult {
	lower := strings.ToLower(answer)
	structureTerms := map[Mode][]string{
		ModeBehavioral:   {"situation", "task", "action", "result"},
		ModeSystemDesign: {"requirement", "data", "failure", "observability", "rollout"},
		ModeProject:      {"problem", "owned", "built", "architecture", "result"},
		ModeClaimDefense: {"measured", "source", "contribution", "boundary"},
	}
	structure := scaledCoverage(lower, structureTerms[question.Mode], 25)
	if structure == 0 && len(strings.Fields(answer)) >= 40 {
		structure = 5
	}
	covered, missing := partitionCoverage(lower, question.ExpectedConcepts)
	conceptScore := scaledFraction(len(covered), len(question.ExpectedConcepts), 18)
	tradeoffTerms := []string{"tradeoff", "because", "instead", "alternative", "constraint", "risk", "failure"}
	tradeoffScore := scaledCoverage(lower, tradeoffTerms, 7)
	decisions := minInt(25, conceptScore+tradeoffScore)
	role := scaledCoverage(lower, roleKeywords, 20)
	if len(roleKeywords) == 0 && len(strings.Fields(answer)) > 0 {
		role = 10
	}
	violations := claims.Check(answer, allowedClaims)
	evidence := 20
	if len(violations) > 0 {
		evidence = 0
	}
	concision := concisionScore(len(strings.Fields(answer)), question.TimeSeconds)
	total := structure + decisions + role + evidence + concision
	if len(violations) > 0 && total > 59 {
		total = 59
	}
	return QuestionResult{
		QuestionID: question.ID,
		Score: ScoreBreakdown{
			Structure: structure, DecisionsTradeoff: decisions, RoleRelevance: role,
			EvidenceSafety: evidence, Concision: concision, Total: total,
		},
		CoveredConcepts: covered, MissingConcepts: missing, ClaimViolations: violations,
	}
}

func partitionCoverage(text string, terms []string) ([]string, []string) {
	var covered, missing []string
	for _, term := range uniqueLower(append([]string(nil), terms...)) {
		if strings.Contains(text, term) {
			covered = append(covered, term)
		} else {
			missing = append(missing, term)
		}
	}
	return covered, missing
}

func scaledCoverage(text string, terms []string, max int) int {
	covered, _ := partitionCoverage(text, terms)
	return scaledFraction(len(covered), len(uniqueLower(append([]string(nil), terms...))), max)
}

func scaledFraction(have, total, max int) int {
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(max) * float64(have) / float64(total)))
}

func concisionScore(words, seconds int) int {
	if words == 0 || seconds <= 0 {
		return 0
	}
	target := float64(seconds) / 60.0 * 130.0
	ratio := float64(words) / target
	switch {
	case ratio >= 0.40 && ratio <= 1.25:
		return 10
	case ratio >= 0.25 && ratio <= 1.60:
		return 6
	default:
		return 2
	}
}

func reviewInterval(score, violations int) time.Duration {
	switch {
	case violations > 0 || score < 60:
		return 24 * time.Hour
	case score < 75:
		return 3 * 24 * time.Hour
	case score < 90:
		return 7 * 24 * time.Hour
	default:
		return 14 * 24 * time.Hour
	}
}

func sessionID(session *Session) string {
	raw, _ := json.Marshal(struct {
		DeckID      string
		CompletedAt time.Time
		Answers     []Answer
	}{session.DeckID, session.CompletedAt, session.Answers})
	sum := sha256.Sum256(raw)
	return "session-" + hex.EncodeToString(sum[:8])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
