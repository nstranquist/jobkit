package main

import (
	"fmt"
	"strings"

	"github.com/nstranquist/jobkit/internal/envelope"
	"github.com/nstranquist/jobkit/internal/inbox"
)

const maximumImplicitInboxJSONItems = 200

func inboxSummaryLimit(c *cli) (int, error) {
	limit, err := c.int("limit", inbox.DefaultSummaryLimit)
	if err != nil {
		return 0, err
	}
	if limit < 1 || limit > 100 {
		return 0, envelope.New(envelope.CodeInvalidArgs, "--limit must be between 1 and 100 for inbox summary")
	}
	return limit, nil
}

func boundInboxList(c *cli, items []*inbox.Item) ([]*inbox.Item, error) {
	_, hasLimit := c.flags["limit"]
	if !hasLimit {
		if c.bool("json") && len(items) > maximumImplicitInboxJSONItems {
			return nil, envelope.Newf(envelope.CodeInvalidArgs, "inbox JSON contains %d items and exceeds the implicit output limit", len(items)).
				WithHint("run `jobkit inbox summary --json`, add `--limit N`, or pass `--limit 0` for an explicit full export")
		}
		return items, nil
	}
	limit, err := c.int("limit", 0)
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, envelope.New(envelope.CodeInvalidArgs, "--limit must not be negative")
	}
	if limit == 0 || limit >= len(items) {
		return items, nil
	}
	return items[:limit], nil
}

func renderInboxSummary(summary inbox.Summary) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Inbox: %d total, %d active, %d ready, %d unassessed, %d hard-constraint rechecks, %d active duplicate groups\n",
		summary.Total, summary.Active, summary.ReadyTotal, summary.Unassessed, summary.HardConstraintsPending, summary.ActiveDuplicateGroups)
	for _, item := range summary.Ready {
		fmt.Fprintf(&out, "- %s · %s · %s · %.0f/100 · %s\n", item.Company, item.Title, item.Lane, item.MatchScore, item.ID)
	}
	if summary.Truncated {
		fmt.Fprintf(&out, "Showing %d of %d ready roles. Increase --limit to show more.\n", summary.Returned, summary.ReadyTotal)
	}
	return out.String()
}

func renderInboxDuplicates(groups []inbox.DuplicateGroup) string {
	if len(groups) == 0 {
		return "no active duplicate inbox records\n"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Active duplicate groups: %d\n", len(groups))
	for _, group := range groups {
		fmt.Fprintf(&out, "\n%s\n", group.Identity)
		for _, item := range group.Items {
			fmt.Fprintf(&out, "- %s [%s] %s — %s", item.ID, item.Status, item.Company, item.Title)
			if item.Provider != "" || item.Board != "" || item.ExternalID != "" {
				fmt.Fprintf(&out, " (%s:%s:%s)", item.Provider, item.Board, item.ExternalID)
			}
			out.WriteString("\n")
		}
	}
	return out.String()
}

func renderInboxDedupePlan(plans []inbox.DuplicateRepair) string {
	if len(plans) == 0 {
		return "no active duplicate inbox records\n"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Duplicate repair plan: %d group(s)\n", len(plans))
	for _, plan := range plans {
		fmt.Fprintf(&out, "\n%s\n", plan.Identity)
		fmt.Fprintf(&out, "  keep: %s\n", plan.WinnerID)
		fmt.Fprintf(&out, "  latest metadata: %s\n", plan.LatestID)
		fmt.Fprintf(&out, "  skip: %s\n", strings.Join(plan.LoserIDs, ", "))
		if plan.BlockedReason != "" {
			fmt.Fprintf(&out, "  blocked: %s\n", plan.BlockedReason)
		}
	}
	out.WriteString("\nReview the plan. Run `jobkit inbox dedupe --apply` to append the repairs.\n")
	return out.String()
}
