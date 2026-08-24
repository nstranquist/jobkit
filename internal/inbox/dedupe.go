package inbox

import (
	"sort"
	"time"
)

// DuplicateRepair is an append-only repair plan for one active duplicate
// group. The winner keeps its ID. The latest posting metadata moves to that
// history, and each loser moves to skipped.
type DuplicateRepair struct {
	Identity      string   `json:"identity"`
	WinnerID      string   `json:"winner_id"`
	LatestID      string   `json:"latest_id"`
	LoserIDs      []string `json:"loser_ids"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
}

// PlanDuplicateRepairs creates a deterministic repair plan without changing
// the ledger. More than one active planned or shortlisted record blocks an
// automatic repair because both records can own human workflow state.
func PlanDuplicateRepairs(items []*Item) []DuplicateRepair {
	byID := make(map[string]*Item, len(items))
	for _, item := range items {
		if item != nil {
			byID[item.ID] = item
		}
	}
	groups := ActiveDuplicateGroups(items)
	plans := make([]DuplicateRepair, 0, len(groups))
	for _, group := range groups {
		if len(group.Items) == 0 {
			continue
		}
		representative := byID[group.Items[0].ID]
		if representative == nil {
			continue
		}
		plan := DuplicateRepair{Identity: group.Identity, WinnerID: ResolveID(items, representative.Job)}
		matching := make([]*Item, 0, len(group.Items)+1)
		workflowOwners := 0
		for _, item := range items {
			if item == nil || !SamePosting(representative.Job, item.Job) {
				continue
			}
			matching = append(matching, item)
		}
		for _, duplicate := range group.Items {
			if duplicate.Status == "planned" || duplicate.Status == "shortlisted" {
				workflowOwners++
			}
			if duplicate.ID != plan.WinnerID {
				plan.LoserIDs = append(plan.LoserIDs, duplicate.ID)
			}
		}
		if workflowOwners > 1 {
			plan.BlockedReason = "more than one active record owns application workflow state"
		}
		sort.Strings(plan.LoserIDs)
		if latest := latestPostingItem(matching); latest != nil {
			plan.LatestID = latest.ID
		}
		plans = append(plans, plan)
	}
	return plans
}

func latestPostingItem(items []*Item) *Item {
	var latest *Item
	for _, item := range items {
		if item == nil {
			continue
		}
		if latest == nil || postingObservedAt(item).After(postingObservedAt(latest)) ||
			(postingObservedAt(item).Equal(postingObservedAt(latest)) && item.ID < latest.ID) {
			latest = item
		}
	}
	return latest
}

func postingObservedAt(item *Item) time.Time {
	if !item.LastSeenAt.IsZero() {
		return item.LastSeenAt
	}
	if !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	return item.UpdatedAt
}
