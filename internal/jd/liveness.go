package jd

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ClosedError is returned when a posting is not a live opening.
// Match and apply-plan must fail closed on this error and must not emit a score
// or write an apply package.
type ClosedError struct {
	Status int
	Reason string
}

func (e *ClosedError) Error() string {
	if e == nil {
		return "posting closed"
	}
	if e.Status != 0 {
		return fmt.Sprintf("posting closed (HTTP %d): %s", e.Status, e.Reason)
	}
	return "posting closed: " + e.Reason
}

// IsClosed reports whether err is a fail-closed dead posting.
func IsClosed(err error) bool {
	var closed *ClosedError
	return errors.As(err, &closed)
}

// HTTPClient is the client Fetch uses. Tests replace it with httptest clients.
var HTTPClient = &http.Client{Timeout: 15 * time.Second}

// ClassifyBody reports whether posting text is a closed/expired opening.
// Patterns are conservative so a live JD that mentions "close-knit" still passes.
func ClassifyBody(text string) (closed bool, reason string) {
	if strings.TrimSpace(text) == "" {
		return false, ""
	}
	if match := closedBodyRe.FindString(text); match != "" {
		return true, compactReason(match)
	}
	return false, ""
}

// RequireLive fails closed when the posting body is expired or closed.
// File-pasted JDs use the same gate as fetched URLs.
func RequireLive(text string) error {
	if closed, reason := ClassifyBody(text); closed {
		return &ClosedError{Reason: reason}
	}
	return nil
}

var closedBodyRe = regexp.MustCompile(`(?i)(?:` +
	`no longer accepting applications|` +
	`this (?:position|role|job|posting|requisition|opportunity) (?:has been filled|is no longer available|has expired|is closed|has been closed|is no longer active)|` +
	`(?:job|position|posting) (?:is )?(?:closed|expired)|` +
	`this listing has expired|` +
	`page not found` +
	`)`)

func compactReason(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
