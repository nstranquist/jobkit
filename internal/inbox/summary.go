package inbox

import (
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/eligibility"
)

const DefaultSummaryLimit = 20

// SummaryItem is the bounded, description-free shape for agent and human
// inbox triage. It never includes raw job text or the event history.
type SummaryItem struct {
	ID          string             `json:"id"`
	Status      string             `json:"status"`
	NextAction  string             `json:"next_action,omitempty"`
	Company     string             `json:"company,omitempty"`
	Title       string             `json:"title"`
	Location    string             `json:"location,omitempty"`
	URL         string             `json:"url,omitempty"`
	Lane        string             `json:"lane"`
	MatchScore  float64            `json:"match_score"`
	Opportunity int                `json:"opportunity"`
	Eligibility eligibility.Status `json:"eligibility"`
}

// Summary reports the complete inbox counts and a bounded list of roles that
// are ready for an application plan.
type Summary struct {
	Total                  int            `json:"total"`
	Active                 int            `json:"active"`
	Terminal               int            `json:"terminal"`
	Closed                 int            `json:"closed"`
	Unassessed             int            `json:"unassessed"`
	HardConstraintsPending int            `json:"hard_constraints_pending"`
	ReadyTotal             int            `json:"ready_total"`
	Returned               int            `json:"returned"`
	Truncated              bool           `json:"truncated"`
	ByStatus               map[string]int `json:"by_status"`
	ByEligibility          map[string]int `json:"by_eligibility"`
	ByNextAction           map[string]int `json:"by_next_action"`
	ByLane                 map[string]int `json:"by_lane"`
	Ready                  []SummaryItem  `json:"ready"`
}

// BuildSummary computes full counts locally and returns at most limit compact
// ready-role records. A non-positive limit uses DefaultSummaryLimit.
func BuildSummary(items []*Item, limit int) Summary {
	if limit <= 0 {
		limit = DefaultSummaryLimit
	}
	result := Summary{
		ByStatus: map[string]int{}, ByEligibility: map[string]int{},
		ByNextAction: map[string]int{}, ByLane: map[string]int{}, Ready: []SummaryItem{},
	}
	ready := make([]SummaryItem, 0)
	for _, item := range items {
		if item == nil {
			continue
		}
		result.Total++
		result.ByStatus[item.Status]++
		result.ByNextAction[firstNonEmpty(item.NextAction, "unspecified")]++
		if TerminalStatuses[item.Status] {
			result.Terminal++
		} else {
			result.Active++
		}
		if strings.EqualFold(item.Job.Liveness, "closed") {
			result.Closed++
		}
		assessment := item.Job.Eligibility
		if assessment == nil {
			result.Unassessed++
			result.ByEligibility[string(eligibility.Unassessed)]++
			continue
		}
		result.ByEligibility[string(assessment.Status)]++
		if assessment.Status != eligibility.Eligible || !assessment.HardConstraintsVerified {
			if assessment.Status == eligibility.Eligible && !assessment.HardConstraintsVerified {
				result.HardConstraintsPending++
			}
			continue
		}
		if TerminalStatuses[item.Status] || strings.EqualFold(item.Job.Liveness, "closed") {
			continue
		}
		lane := LaneFor(item)
		result.ByLane[lane]++
		ready = append(ready, SummaryItem{
			ID: item.ID, Status: item.Status, NextAction: item.NextAction,
			Company: item.Job.Company, Title: item.Job.Title, Location: item.Job.Location,
			URL: firstNonEmpty(item.Job.ApplyURL, item.Job.URL), Lane: lane,
			MatchScore: item.MatchScore, Opportunity: item.Job.Opportunity.Score,
			Eligibility: assessment.Status,
		})
	}
	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].Opportunity != ready[j].Opportunity {
			return ready[i].Opportunity > ready[j].Opportunity
		}
		if ready[i].MatchScore != ready[j].MatchScore {
			return ready[i].MatchScore > ready[j].MatchScore
		}
		return ready[i].ID < ready[j].ID
	})
	result.ReadyTotal = len(ready)
	if len(ready) > limit {
		result.Truncated = true
		ready = ready[:limit]
	}
	result.Ready = ready
	result.Returned = len(ready)
	return result
}
