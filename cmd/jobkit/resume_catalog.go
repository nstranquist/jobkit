package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/inbox"
	"github.com/nstranquist/jobkit/internal/jd"
	"github.com/nstranquist/jobkit/internal/privatefs"
	"github.com/nstranquist/jobkit/internal/profile"
	"github.com/nstranquist/jobkit/internal/resume"
	"github.com/nstranquist/jobkit/internal/resumeartifact"
	"github.com/nstranquist/jobkit/internal/track"
)

const applicationResumeReceiptSchemaVersion = 1

type applicationResumeRequest struct {
	CatalogRoot   string
	ArtifactKind  string
	Dir           string
	ApplicationID string
	InboxID       string
	Company       string
	Role          string
	SourceURL     string
	Description   string
	MatchScore    float64
	Assessment    *eligibility.Result
}

type applicationResume struct {
	Name        string
	ReceiptName string
	PlanNote    string
	Tags        map[string]string
}

type applicationResumeReceipt struct {
	SchemaVersion int                        `json:"schema_version"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	ApplicationID string                     `json:"application_id"`
	InboxID       string                     `json:"inbox_id,omitempty"`
	Company       string                     `json:"company"`
	Role          string                     `json:"role"`
	SourceURL     string                     `json:"source_url,omitempty"`
	MatchScore    float64                    `json:"match_score"`
	Selection     applicationResumeSelection `json:"selection"`
	Eligibility   *eligibility.Result        `json:"eligibility,omitempty"`
	Tags          map[string]string          `json:"tags"`
}

type applicationResumeSelection struct {
	Lane           string   `json:"lane"`
	VariantID      string   `json:"variant_id"`
	Version        string   `json:"version"`
	Score          int      `json:"score"`
	Reasons        []string `json:"reasons"`
	IndexDigest    string   `json:"index_digest"`
	ManifestDigest string   `json:"manifest_digest"`
	ArtifactKind   string   `json:"artifact_kind"`
	ArtifactName   string   `json:"artifact_name"`
	ArtifactDigest string   `json:"artifact_digest"`
}

// requestedCatalogResumeKind keeps explicit HTML and Markdown requests on the
// built-in renderer. The verified catalog is the default and supports its
// sendable PDF, DOCX, and ATS artifacts.
func requestedCatalogResumeKind(c *cli) (string, bool) {
	if kind := strings.ToLower(strings.TrimSpace(c.str("resume-artifact"))); kind != "" {
		return kind, true
	}
	switch strings.ToLower(strings.TrimSpace(c.str("format"))) {
	case "":
		return "pdf", true
	case "pdf":
		return "pdf", true
	case "docx":
		return "docx", true
	case "txt", "text", "ats":
		return "ats", true
	default:
		return "", false
	}
}

func applicationSourceURL(src, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if jd.IsURL(src) {
		return src
	}
	return ""
}

func buildPackageResume(c *cli, p *profile.Profile, parsed *jd.JD, options resume.Options, request applicationResumeRequest) (string, string, *applicationResume, error) {
	if kind, useCatalog := requestedCatalogResumeKind(c); useCatalog {
		request.ArtifactKind = kind
		selected, err := prepareApplicationResume(request)
		if err != nil {
			return "", "", nil, err
		}
		if selected != nil {
			return "", selected.Name, selected, nil
		}
		if strings.TrimSpace(c.str("resume-artifact")) != "" {
			return "", "", nil, fmt.Errorf("--resume-artifact requires a configured resume catalog; set --resume-root or JOBKIT_RESUME_ROOT")
		}
	}

	format := firstNonEmpty(c.str("format"), "html")
	doc := resume.Build(p, parsed, options)
	if err := checkGeneratedClaims("resume", resume.RenderText(doc)); err != nil {
		return "", "", nil, err
	}
	switch format {
	case "md", "markdown":
		return resume.RenderMarkdown(doc), "resume.md", nil, nil
	case "txt", "text", "ats":
		return resume.RenderText(doc), "resume.txt", nil, nil
	case "html":
		return resume.RenderHTML(doc), "resume.html", nil, nil
	case "pdf":
		name := "resume.pdf"
		if err := resume.RenderPDF(resume.RenderHTML(doc), filepath.Join(request.Dir, name)); err != nil {
			return "", "", nil, err
		}
		return "", name, nil, nil
	default:
		return "", "", nil, fmt.Errorf("unknown format %q (md|txt|html|pdf); DOCX requires a verified resume catalog", format)
	}
}

func prepareApplicationResume(request applicationResumeRequest) (*applicationResume, error) {
	if strings.EqualFold(strings.TrimSpace(request.CatalogRoot), "none") {
		return nil, nil
	}
	root, err := resumeartifact.ResolveRoot(request.CatalogRoot)
	if errors.Is(err, resumeartifact.ErrNotConfigured) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	preferredLane, err := preferredResumeLane(request.InboxID)
	if err != nil {
		return nil, err
	}
	selection, err := resumeartifact.Select(root, resumeartifact.Query{
		Role: request.Role, Description: request.Description,
		PreferredLane: preferredLane, ArtifactKind: request.ArtifactKind,
	})
	if err != nil {
		return nil, err
	}
	if err := removeStaleApplicationResumes(request.Dir); err != nil {
		return nil, err
	}
	destination, err := selection.CopyTo(request.Dir)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(destination)
	tags := cloneTags(selection.Tags)
	tags[track.TagTailoringReceiptID] = "nicos-resume:" + request.ApplicationID
	tags[track.TagLane] = selection.Lane
	if request.InboxID != "" {
		tags[track.TagInboxID] = request.InboxID
	}
	if request.Assessment != nil && request.Assessment.Override != "" {
		tags[track.TagEligibilityOverride] = request.Assessment.Override
	}
	receipt := applicationResumeReceipt{
		SchemaVersion: applicationResumeReceiptSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		ApplicationID: request.ApplicationID,
		InboxID:       request.InboxID,
		Company:       request.Company,
		Role:          request.Role,
		SourceURL:     request.SourceURL,
		MatchScore:    request.MatchScore,
		Eligibility:   request.Assessment,
		Tags:          tags,
		Selection: applicationResumeSelection{
			Lane: selection.Lane, VariantID: selection.VariantID, Version: selection.Version,
			Score: selection.Score, Reasons: append([]string(nil), selection.Reasons...),
			IndexDigest: selection.IndexDigest, ManifestDigest: selection.ManifestDigest,
			ArtifactKind: selection.ArtifactKind, ArtifactName: name, ArtifactDigest: selection.ArtifactDigest,
		},
	}
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return nil, err
	}
	receiptName := "resume-receipt.json"
	if err := privatefs.WriteFile(filepath.Join(request.Dir, receiptName), append(payload, '\n')); err != nil {
		return nil, err
	}
	return &applicationResume{
		Name: name, ReceiptName: receiptName, Tags: tags,
		PlanNote: fmt.Sprintf("Verified resume: `%s` (%s, %s, %s). Selection reasons and producer digests are in `%s`.",
			name, selection.Lane, selection.VariantID, selection.Version, receiptName),
	}, nil
}

func preferredResumeLane(inboxID string) (string, error) {
	if strings.TrimSpace(inboxID) == "" {
		return "", nil
	}
	ledger, err := openInboxLedger()
	if err != nil {
		return "", err
	}
	items, err := ledger.Replay()
	if err != nil {
		return "", err
	}
	item, err := inbox.Find(items, inboxID)
	if err != nil {
		return "", err
	}
	switch inbox.LaneFor(item) {
	case inbox.LaneFullstack:
		return "fullstack", nil
	case inbox.LaneAdoption:
		return "adoption", nil
	case inbox.LaneStretch:
		return "enterprise", nil
	default:
		return "platform", nil
	}
}

func removeStaleApplicationResumes(dir string) error {
	for _, name := range []string{"resume.md", "resume.txt", "resume.html", "resume.pdf", "resume.docx", "resume-receipt.json"} {
		err := os.Remove(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func cloneTags(tags map[string]string) map[string]string {
	cloned := make(map[string]string, len(tags)+4)
	for key, value := range tags {
		cloned[key] = value
	}
	return cloned
}

func packageFileNames(files map[string]string, extra ...string) []string {
	names := sortedKeys(files)
	seen := make(map[string]bool, len(names)+len(extra))
	for _, name := range names {
		seen[name] = true
	}
	for _, name := range extra {
		if name != "" && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	sort.Strings(names)
	return names
}

func receiptName(selected *applicationResume) string {
	if selected == nil {
		return ""
	}
	return selected.ReceiptName
}
