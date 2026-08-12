// Package coach builds evidence-linked interview drills and records local
// practice sessions. Deterministic scoring is authoritative. Optional model
// feedback is advisory.
package coach

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

var (
	publicEmailRE = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	publicPhoneRE = regexp.MustCompile(`(?:\+?1[ .-]?)?\(?[2-9][0-9]{2}\)?[ .-][0-9]{3}[ .-][0-9]{4}`)
)

type SourceBundle struct {
	SchemaVersion int               `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Scope         string            `json:"scope"`
	Candidate     Candidate         `json:"candidate"`
	Projects      []ProjectCard     `json:"projects"`
	Stories       []Story           `json:"stories"`
	Claims        []Claim           `json:"claims"`
	SourceDigests map[string]string `json:"source_digests"`
}

type Candidate struct {
	Name     string   `json:"name"`
	Headline string   `json:"headline,omitempty"`
	Skills   []string `json:"skills,omitempty"`
}

type ProjectCard struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Summary   string     `json:"summary"`
	URL       string     `json:"url,omitempty"`
	Skills    []string   `json:"skills,omitempty"`
	Decisions []string   `json:"decisions,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

type Evidence struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	URL      string   `json:"url,omitempty"`
	ClaimIDs []string `json:"claim_ids,omitempty"`
}

type Story struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Situation   string   `json:"situation"`
	Task        string   `json:"task"`
	Actions     []string `json:"actions"`
	Result      string   `json:"result"`
	Skills      []string `json:"skills,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	ClaimIDs    []string `json:"claim_ids,omitempty"`
}

type Claim struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
}

func LoadSource(path string) (*SourceBundle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(raw)
}

func ParseSource(raw []byte) (*SourceBundle, error) {
	var bundle SourceBundle
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode coach source: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode coach source: trailing JSON data")
	}
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func (b *SourceBundle) Validate() error {
	if b.SchemaVersion != SchemaVersion {
		return fmt.Errorf("coach source schema_version=%d; supported version is %d", b.SchemaVersion, SchemaVersion)
	}
	if b.Scope != "public" {
		return fmt.Errorf("coach source scope must be public, got %q", b.Scope)
	}
	if strings.TrimSpace(b.Candidate.Name) == "" {
		return fmt.Errorf("coach source candidate.name is required")
	}
	if _, err := time.Parse(time.RFC3339, b.GeneratedAt); err != nil {
		return fmt.Errorf("coach source generated_at must use RFC3339: %w", err)
	}
	if len(b.Projects) == 0 {
		return fmt.Errorf("coach source needs at least one public project")
	}
	if len(b.SourceDigests) == 0 {
		return fmt.Errorf("coach source needs at least one source digest")
	}
	for name, digest := range b.SourceDigests {
		decoded, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
		if strings.TrimSpace(name) == "" || !strings.HasPrefix(digest, "sha256:") || err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("coach source digest %q must use sha256: followed by 64 hexadecimal characters", name)
		}
	}
	ids := map[string]string{}
	addID := func(id, kind string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("coach source %s id is required", kind)
		}
		if prior := ids[id]; prior != "" {
			return fmt.Errorf("coach source id %q is used by %s and %s", id, prior, kind)
		}
		ids[id] = kind
		return nil
	}
	claimIDs := map[string]bool{}
	for _, claim := range b.Claims {
		if err := addID(claim.ID, "claim"); err != nil {
			return err
		}
		if strings.TrimSpace(claim.Text) == "" || strings.TrimSpace(claim.Source) == "" {
			return fmt.Errorf("coach claim %q needs text and source", claim.ID)
		}
		claimIDs[claim.ID] = true
	}
	evidenceIDs := map[string]bool{}
	for _, project := range b.Projects {
		if err := addID(project.ID, "project"); err != nil {
			return err
		}
		if strings.TrimSpace(project.Name) == "" || strings.TrimSpace(project.Summary) == "" {
			return fmt.Errorf("coach project %q needs name and summary", project.ID)
		}
		if project.URL != "" {
			if err := validatePublicURL(project.URL); err != nil {
				return fmt.Errorf("coach project %q: %w", project.ID, err)
			}
		}
		for _, evidence := range project.Evidence {
			if err := addID(evidence.ID, "evidence"); err != nil {
				return err
			}
			if strings.TrimSpace(evidence.Label) == "" || strings.TrimSpace(evidence.URL) == "" {
				return fmt.Errorf("coach evidence %q needs label and public URL", evidence.ID)
			}
			evidenceIDs[evidence.ID] = true
			if err := validatePublicURL(evidence.URL); err != nil {
				return fmt.Errorf("coach evidence %q: %w", evidence.ID, err)
			}
			for _, claimID := range evidence.ClaimIDs {
				if !claimIDs[claimID] {
					return fmt.Errorf("coach evidence %q references unknown claim %q", evidence.ID, claimID)
				}
			}
		}
	}
	for _, story := range b.Stories {
		if err := addID(story.ID, "story"); err != nil {
			return err
		}
		if strings.TrimSpace(story.Title) == "" || strings.TrimSpace(story.Situation) == "" || strings.TrimSpace(story.Task) == "" || strings.TrimSpace(story.Result) == "" || len(story.Actions) == 0 {
			return fmt.Errorf("coach story %q needs title, situation, task, actions, and result", story.ID)
		}
		for _, evidenceID := range story.EvidenceIDs {
			if !evidenceIDs[evidenceID] {
				return fmt.Errorf("coach story %q references unknown evidence %q", story.ID, evidenceID)
			}
		}
		for _, claimID := range story.ClaimIDs {
			if !claimIDs[claimID] {
				return fmt.Errorf("coach story %q references unknown claim %q", story.ID, claimID)
			}
		}
	}
	if problems := publicSafetyProblems(b); len(problems) > 0 {
		return fmt.Errorf("coach source is not public-safe: %s", strings.Join(problems, ", "))
	}
	return nil
}

func validatePublicURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("public URL must use https with a public host, got %q", raw)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("public URL cannot use a local host, got %q", raw)
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return fmt.Errorf("public URL cannot use a private address, got %q", raw)
	}
	return nil
}

func publicSafetyProblems(bundle *SourceBundle) []string {
	raw, _ := json.Marshal(bundle)
	lower := strings.ToLower(string(raw))
	markers := []string{
		"/users/", `c:\\users\\`, "file://", "~/", ".jobkit/",
		"private-admin-evidence",
	}
	var found []string
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			found = append(found, marker)
		}
	}
	if publicEmailRE.Match(raw) {
		found = append(found, "email-address")
	}
	if publicPhoneRE.Match(raw) {
		found = append(found, "phone-number")
	}
	sort.Strings(found)
	return found
}

func (b *SourceBundle) Digest() string {
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (b *SourceBundle) AllowedClaims() []string {
	out := make([]string, 0, len(b.Claims))
	for _, claim := range b.Claims {
		out = append(out, claim.Text)
	}
	return out
}
