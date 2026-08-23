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
	fmt.Fprintf(&out, "Inbox: %d total, %d active, %d ready, %d unassessed, %d hard-constraint rechecks\n",
		summary.Total, summary.Active, summary.ReadyTotal, summary.Unassessed, summary.HardConstraintsPending)
	for _, item := range summary.Ready {
		fmt.Fprintf(&out, "- %s · %s · %s · %.0f/100 · %s\n", item.Company, item.Title, item.Lane, item.MatchScore, item.ID)
	}
	if summary.Truncated {
		fmt.Fprintf(&out, "Showing %d of %d ready roles. Increase --limit to show more.\n", summary.Returned, summary.ReadyTotal)
	}
	return out.String()
}
