package inbox

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nstranquist/jobkit/internal/eligibility"
)

const (
	LanePlatform  = "platform-devex-ai-infra"
	LaneFullstack = "fullstack-product"
	LaneAdoption  = "technical-adoption-fde"
	LaneStretch   = "stretch"
)

type SlatePolicy struct {
	Platform    int  `json:"platform"`
	Fullstack   int  `json:"fullstack"`
	Adoption    int  `json:"adoption"`
	Stretch     int  `json:"stretch"`
	EmployerCap int  `json:"employer_cap"`
	AllowReview bool `json:"allow_review"`
}

type SlateSelection struct {
	ID          string              `json:"id"`
	Lane        string              `json:"lane"`
	Company     string              `json:"company"`
	Title       string              `json:"title"`
	URL         string              `json:"url,omitempty"`
	MatchScore  float64             `json:"match_score"`
	Opportunity int                 `json:"opportunity"`
	Eligibility *eligibility.Result `json:"eligibility"`
}

type Slate struct {
	Policy     SlatePolicy      `json:"policy"`
	Selections []SlateSelection `json:"selections"`
	Filled     map[string]int   `json:"filled"`
	Warnings   []string         `json:"warnings,omitempty"`
	Skipped    map[string]int   `json:"skipped,omitempty"`
}

func DefaultSlatePolicy() SlatePolicy {
	return SlatePolicy{Platform: 5, Fullstack: 3, Adoption: 1, Stretch: 1, EmployerCap: 2}
}

// BuildSlate selects a deterministic weekly application mix from the active
// inbox. It never selects ineligible or unassessed jobs and never silently
// substitutes one lane for another when the requested mix cannot be filled.
func BuildSlate(items []*Item, policy SlatePolicy) Slate {
	if policy.EmployerCap <= 0 {
		policy.EmployerCap = 2
	}
	result := Slate{
		Policy: policy, Filled: map[string]int{}, Skipped: map[string]int{},
		Selections: []SlateSelection{},
	}
	var candidates []*Item
	for _, item := range items {
		if TerminalStatuses[item.Status] {
			result.Skipped["terminal"]++
			continue
		}
		if item.Job.Eligibility == nil {
			result.Skipped["unassessed"]++
			continue
		}
		if item.Job.Eligibility.Status == eligibility.Ineligible {
			result.Skipped["ineligible"]++
			continue
		}
		if item.Job.Eligibility.Status == eligibility.Review && !policy.AllowReview {
			result.Skipped["review_required"]++
			continue
		}
		if !item.Job.Eligibility.HardConstraintsVerified {
			result.Skipped["hard_constraints_unverified"]++
			continue
		}
		candidates = append(candidates, item)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.Job.Eligibility.Status != right.Job.Eligibility.Status {
			return eligibility.Rank(left.Job.Eligibility.Status) < eligibility.Rank(right.Job.Eligibility.Status)
		}
		if left.Job.Opportunity.Score != right.Job.Opportunity.Score {
			return left.Job.Opportunity.Score > right.Job.Opportunity.Score
		}
		if left.MatchScore != right.MatchScore {
			return left.MatchScore > right.MatchScore
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		return left.ID < right.ID
	})

	selected := map[string]bool{}
	companies := map[string]int{}
	quotas := []struct {
		lane  string
		count int
		match func(*Item) bool
	}{
		{LaneStretch, policy.Stretch, isStretchRole},
		{LaneAdoption, policy.Adoption, isAdoptionRole},
		{LaneFullstack, policy.Fullstack, isFullstackRole},
		{LanePlatform, policy.Platform, isPlatformRole},
	}
	for _, quota := range quotas {
		for _, item := range candidates {
			if result.Filled[quota.lane] >= quota.count {
				break
			}
			if selected[item.ID] || !quota.match(item) {
				continue
			}
			companyKey := strings.ToLower(strings.TrimSpace(firstNonEmpty(item.Job.Company, item.Job.Board, item.Job.Provider, "unknown")))
			if companies[companyKey] >= policy.EmployerCap {
				continue
			}
			selected[item.ID] = true
			companies[companyKey]++
			result.Filled[quota.lane]++
			result.Selections = append(result.Selections, SlateSelection{
				ID: item.ID, Lane: quota.lane, Company: item.Job.Company, Title: item.Job.Title,
				URL: firstNonEmpty(item.Job.ApplyURL, item.Job.URL), MatchScore: item.MatchScore,
				Opportunity: item.Job.Opportunity.Score, Eligibility: item.Job.Eligibility,
			})
		}
		if result.Filled[quota.lane] < quota.count {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: selected %d of %d requested", quota.lane, result.Filled[quota.lane], quota.count))
		}
	}
	if result.Skipped["unassessed"] > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d active job(s) are unassessed; run `jobkit inbox recheck`", result.Skipped["unassessed"]))
	}
	if result.Skipped["hard_constraints_unverified"] > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d active job(s) have old or incomplete hard-constraint evidence; run `jobkit inbox recheck`", result.Skipped["hard_constraints_unverified"]))
	}
	if result.Skipped["review_required"] > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d active job(s) require a human eligibility decision", result.Skipped["review_required"]))
	}
	return result
}

// LaneFor returns the deterministic resume lane for an assessed inbox item.
func LaneFor(item *Item) string {
	switch {
	case item == nil || item.Job.Eligibility == nil:
		return LanePlatform
	case isAdoptionRole(item):
		return LaneAdoption
	case isStretchRole(item):
		return LaneStretch
	case isFullstackRole(item):
		return LaneFullstack
	default:
		return LanePlatform
	}
}

var (
	stretchRolePattern   = regexp.MustCompile(`(?i)\b(staff|principal|lead|architect)\b`)
	fullstackRolePattern = regexp.MustCompile(`(?i)\b(full[- ]?stack|front[- ]?end|product engineer|product software)\b`)
	platformRolePattern  = regexp.MustCompile(`(?i)\b(platform|developer experience|developer productivity|developer tools?|devex|infrastructure|infra|backend|cloud|ai|ml|systems?|sre|site reliability)\b`)
)

func isStretchRole(item *Item) bool {
	if stretchRolePattern.MatchString(item.Job.Title) {
		return true
	}
	for _, reason := range item.Job.Eligibility.Reasons {
		if reason.Code == "years_stretch" {
			return true
		}
	}
	return false
}

func isAdoptionRole(item *Item) bool {
	return item.Job.Eligibility.RoleFamily == "technical-adoption"
}

func isFullstackRole(item *Item) bool {
	if item.Job.Eligibility.RoleFamily == "technical-adoption" || isStretchRole(item) {
		return false
	}
	return fullstackRolePattern.MatchString(item.Job.Title + " " + item.Job.Department)
}

func isPlatformRole(item *Item) bool {
	if item.Job.Eligibility.RoleFamily != "software-engineering" || isStretchRole(item) || isFullstackRole(item) {
		return false
	}
	text := item.Job.Title + " " + item.Job.Department
	return platformRolePattern.MatchString(text)
}
