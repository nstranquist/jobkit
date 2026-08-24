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

// DuplicateItem is a description-free identity record for duplicate review.
type DuplicateItem struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Company    string `json:"company,omitempty"`
	Title      string `json:"title"`
	Provider   string `json:"provider,omitempty"`
	Board      string `json:"board,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
	URL        string `json:"url,omitempty"`
}

// DuplicateGroup contains active inbox records that identify one posting.
// It omits job descriptions and event histories.
type DuplicateGroup struct {
	Identity string          `json:"identity"`
	Items    []DuplicateItem `json:"items"`
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
	ActiveDuplicateGroups  int            `json:"active_duplicate_groups"`
	ActiveDuplicateItems   int            `json:"active_duplicate_items"`
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
	duplicates := ActiveDuplicateGroups(items)
	result.ActiveDuplicateGroups = len(duplicates)
	for _, group := range duplicates {
		result.ActiveDuplicateItems += len(group.Items)
	}
	return result
}

// ActiveDuplicateGroups returns each set of non-terminal records that identify
// one posting. The result is stable and contains no job descriptions.
func ActiveDuplicateGroups(items []*Item) []DuplicateGroup {
	active := make([]*Item, 0, len(items))
	for _, item := range items {
		if item != nil && !TerminalStatuses[item.Status] {
			active = append(active, item)
		}
	}
	parents := make([]int, len(active))
	identities := make([]postingIdentity, len(active))
	componentATS := make([]string, len(active))
	for i := range parents {
		parents[i] = i
		identities[i] = postingIdentityFor(active[i].Job)
		componentATS[i] = identities[i].ATS
	}
	find := func(i int) int {
		for parents[i] != i {
			parents[i] = parents[parents[i]]
			i = parents[i]
		}
		return i
	}
	for i := range active {
		for j := i + 1; j < len(active); j++ {
			if !samePostingIdentity(identities[i], identities[j]) {
				continue
			}
			left, right := find(i), find(j)
			if left != right {
				if componentATS[left] != "" && componentATS[right] != "" && componentATS[left] != componentATS[right] {
					continue
				}
				parents[right] = left
				if componentATS[left] == "" {
					componentATS[left] = componentATS[right]
				}
			}
		}
	}
	byRoot := map[int][]*Item{}
	for i := range active {
		root := find(i)
		byRoot[root] = append(byRoot[root], active[i])
	}
	groups := make([]DuplicateGroup, 0)
	for _, groupItems := range byRoot {
		if len(groupItems) < 2 {
			continue
		}
		sort.SliceStable(groupItems, func(i, j int) bool { return groupItems[i].ID < groupItems[j].ID })
		group := DuplicateGroup{Identity: IdentityKey(groupItems[0].Job), Items: make([]DuplicateItem, 0, len(groupItems))}
		for _, item := range groupItems {
			group.Items = append(group.Items, DuplicateItem{
				ID: item.ID, Status: item.Status, Company: item.Job.Company, Title: item.Job.Title,
				Provider: item.Job.Provider, Board: item.Job.Board, ExternalID: item.Job.ExternalID,
				URL: firstNonEmpty(item.Job.URL, item.Job.ApplyURL, item.Job.SourceURL),
			})
		}
		groups = append(groups, group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].Items) != len(groups[j].Items) {
			return len(groups[i].Items) > len(groups[j].Items)
		}
		return groups[i].Identity < groups[j].Identity
	})
	return groups
}
