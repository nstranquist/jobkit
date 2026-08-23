// Package resumeartifact selects and verifies versioned resume artifacts from
// a producer-owned sendable index.
package resumeartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/nstranquist/jobkit/internal/privatefs"
	"github.com/nstranquist/jobkit/internal/track"
)

const IndexSchemaVersion = 1

var ErrNotConfigured = errors.New("resume artifact catalog is not configured")

type Index struct {
	SchemaVersion    int     `json:"schema_version"`
	GeneratedAt      string  `json:"generated_at"`
	RegistryDigest   string  `json:"registry_digest"`
	DefaultVariantID string  `json:"default_variant_id"`
	Entries          []Entry `json:"entries"`
}

type Entry struct {
	Lane        string          `json:"lane"`
	VariantID   string          `json:"variant_id"`
	Version     string          `json:"version"`
	Manifest    File            `json:"manifest"`
	Artifacts   map[string]File `json:"artifacts"`
	RoleTargets []string        `json:"role_targets"`
	MatchTerms  []string        `json:"match_terms"`
	Featured    bool            `json:"featured"`
	Order       int             `json:"order"`
}

type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type Query struct {
	Role          string
	Description   string
	PreferredLane string
	ArtifactKind  string
}

type Selection struct {
	Root           string            `json:"root"`
	IndexPath      string            `json:"index_path"`
	IndexDigest    string            `json:"index_digest"`
	Lane           string            `json:"lane"`
	VariantID      string            `json:"variant_id"`
	Version        string            `json:"version"`
	Score          int               `json:"score"`
	Reasons        []string          `json:"reasons"`
	ManifestPath   string            `json:"manifest_path"`
	ManifestDigest string            `json:"manifest_digest"`
	ArtifactKind   string            `json:"artifact_kind"`
	ArtifactPath   string            `json:"artifact_path"`
	ArtifactDigest string            `json:"artifact_digest"`
	Tags           map[string]string `json:"tags"`
}

// ResolveRoot applies the explicit flag, environment, and Nico dogfood path
// in that order. "none" disables the catalog for one invocation.
func ResolveRoot(explicit string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(explicit), "none") {
		return "", ErrNotConfigured
	}
	candidates := []string{strings.TrimSpace(explicit), strings.TrimSpace(os.Getenv("JOBKIT_RESUME_ROOT"))}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "dev", "nicos-resume"))
	}
	for i, candidate := range candidates {
		if candidate == "" {
			continue
		}
		root, err := filepath.Abs(candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(IndexPath(root)); err == nil {
			return root, nil
		} else if i == 0 && strings.TrimSpace(explicit) != "" {
			return "", fmt.Errorf("resume artifact index not found under %s", root)
		}
	}
	return "", ErrNotConfigured
}

func IndexPath(root string) string {
	return filepath.Join(root, "formats-workspace", "exports", "sendable", "index.json")
}

func Select(root string, query Query) (Selection, error) {
	indexPath := IndexPath(root)
	payload, err := os.ReadFile(indexPath)
	if err != nil {
		return Selection{}, err
	}
	var index Index
	if err := json.Unmarshal(payload, &index); err != nil {
		return Selection{}, fmt.Errorf("parse resume artifact index: %w", err)
	}
	if index.SchemaVersion != IndexSchemaVersion {
		return Selection{}, fmt.Errorf("resume artifact index schema_version=%d; JobKit supports %d", index.SchemaVersion, IndexSchemaVersion)
	}
	if len(index.Entries) == 0 {
		return Selection{}, fmt.Errorf("resume artifact index has no sendable entries")
	}
	kind := strings.ToLower(strings.TrimSpace(query.ArtifactKind))
	if kind == "" {
		kind = "pdf"
	}
	if kind != "pdf" && kind != "docx" && kind != "ats" {
		return Selection{}, fmt.Errorf("resume artifact kind %q must be pdf, docx, or ats", query.ArtifactKind)
	}

	type ranked struct {
		entry   Entry
		score   int
		reasons []string
	}
	rankedEntries := make([]ranked, 0, len(index.Entries))
	for _, entry := range index.Entries {
		score, reasons := scoreEntry(index, entry, query)
		rankedEntries = append(rankedEntries, ranked{entry: entry, score: score, reasons: reasons})
	}
	sort.SliceStable(rankedEntries, func(i, j int) bool {
		if rankedEntries[i].score != rankedEntries[j].score {
			return rankedEntries[i].score > rankedEntries[j].score
		}
		if rankedEntries[i].entry.Order != rankedEntries[j].entry.Order {
			return rankedEntries[i].entry.Order < rankedEntries[j].entry.Order
		}
		return rankedEntries[i].entry.VariantID < rankedEntries[j].entry.VariantID
	})
	selected := rankedEntries[0]
	indexDir := filepath.Dir(indexPath)
	manifestPath, err := resolve(indexDir, selected.entry.Manifest.Path)
	if err != nil {
		return Selection{}, fmt.Errorf("manifest path: %w", err)
	}
	artifact, ok := selected.entry.Artifacts[kind]
	if !ok {
		return Selection{}, fmt.Errorf("resume variant %s has no %s artifact", selected.entry.VariantID, kind)
	}
	artifactPath, err := resolve(indexDir, artifact.Path)
	if err != nil {
		return Selection{}, fmt.Errorf("artifact path: %w", err)
	}
	for label, item := range map[string]struct {
		path   string
		digest string
	}{
		"index":    {path: indexPath, digest: digestBytes(payload)},
		"manifest": {path: manifestPath, digest: selected.entry.Manifest.Digest},
		"artifact": {path: artifactPath, digest: artifact.Digest},
	} {
		actual, err := fileDigest(item.path)
		if err != nil {
			return Selection{}, fmt.Errorf("hash %s: %w", label, err)
		}
		if actual != item.digest {
			return Selection{}, fmt.Errorf("%s digest %s does not match index digest %s", label, actual, item.digest)
		}
	}
	tags, err := track.ResumeManifestTags(manifestPath, kind, artifactPath)
	if err != nil {
		return Selection{}, err
	}
	if tags[track.TagResumeVariantID] != selected.entry.VariantID || tags[track.TagResumeVersion] != selected.entry.Version {
		return Selection{}, fmt.Errorf("resume index entry %s/%s does not match manifest %s/%s", selected.entry.VariantID, selected.entry.Version, tags[track.TagResumeVariantID], tags[track.TagResumeVersion])
	}
	return Selection{
		Root: root, IndexPath: indexPath, IndexDigest: digestBytes(payload),
		Lane: selected.entry.Lane, VariantID: selected.entry.VariantID, Version: selected.entry.Version,
		Score: selected.score, Reasons: selected.reasons,
		ManifestPath: manifestPath, ManifestDigest: selected.entry.Manifest.Digest,
		ArtifactKind: kind, ArtifactPath: artifactPath, ArtifactDigest: artifact.Digest, Tags: tags,
	}, nil
}

func (selection Selection) CopyTo(dir string) (string, error) {
	extension := map[string]string{"pdf": ".pdf", "docx": ".docx", "ats": ".txt"}[selection.ArtifactKind]
	if extension == "" {
		return "", fmt.Errorf("unsupported artifact kind %q", selection.ArtifactKind)
	}
	payload, err := os.ReadFile(selection.ArtifactPath)
	if err != nil {
		return "", err
	}
	if digestBytes(payload) != selection.ArtifactDigest {
		return "", fmt.Errorf("resume artifact changed after selection")
	}
	destination := filepath.Join(dir, "resume"+extension)
	if err := privatefs.WriteFile(destination, payload); err != nil {
		return "", err
	}
	return destination, nil
}

func scoreEntry(index Index, entry Entry, query Query) (int, []string) {
	role := normalize(query.Role)
	description := normalize(query.Description)
	text := strings.TrimSpace(role + " " + description)
	preferredLane := normalizeLane(query.PreferredLane)
	score := 0
	var reasons []string
	if preferredLane != "" && normalizeLane(entry.Lane) == preferredLane {
		// The inbox lane is derived from the job title and verified role family.
		// It must outweigh generic company copy in the description, such as a
		// compute-infrastructure posting that mentions "full-stack applications."
		score += 65
		reasons = append(reasons, "preferred lane "+entry.Lane)
	}
	for _, term := range entry.MatchTerms {
		normalized := normalize(term)
		switch {
		case normalized == "":
			continue
		case containsPhrase(role, normalized):
			score += 50
			reasons = append(reasons, "matched title term "+term)
		case containsPhrase(description, normalized):
			score += 10
			reasons = append(reasons, "matched description term "+term)
		}
	}
	for _, target := range entry.RoleTargets {
		normalized := normalize(target)
		if normalized != "" && containsPhrase(text, normalized) {
			score += 35
			reasons = append(reasons, "matched role "+target)
			continue
		}
		matches := 0
		for _, token := range meaningfulTokens(normalized) {
			if containsPhrase(text, token) {
				matches++
			}
		}
		if matches > 0 {
			score += matches * 4
		}
	}
	if entry.VariantID == index.DefaultVariantID {
		score += 5
		reasons = append(reasons, "default variant")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "registry order fallback")
	}
	return score, reasons
}

func normalize(value string) string {
	out := make([]rune, 0, len(value))
	space := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '#' {
			out = append(out, r)
			space = false
		} else if !space {
			out = append(out, ' ')
			space = true
		}
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

func normalizeLane(value string) string {
	value = normalize(value)
	switch {
	case strings.Contains(value, "fullstack") || strings.Contains(value, "full stack"):
		return "fullstack"
	case strings.Contains(value, "adoption") || strings.Contains(value, "forward deployed") || strings.Contains(value, "fde"):
		return "adoption"
	case strings.Contains(value, "enterprise") || strings.Contains(value, "consult"):
		return "enterprise"
	case strings.Contains(value, "stretch"):
		return ""
	case strings.Contains(value, "platform") || strings.Contains(value, "backend") || strings.Contains(value, "infra"):
		return "platform"
	default:
		return value
	}
}

func meaningfulTokens(value string) []string {
	stop := map[string]bool{"senior": true, "staff": true, "engineer": true, "engineering": true, "software": true, "and": true, "the": true}
	var out []string
	for _, token := range strings.Fields(value) {
		if len(token) >= 3 && !stop[token] {
			out = append(out, token)
		}
	}
	return out
}

func containsPhrase(text, phrase string) bool {
	return strings.Contains(" "+text+" ", " "+phrase+" ")
}

func resolve(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be a non-empty relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes the sendable directory", relative)
	}
	candidate := filepath.Join(root, clean)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	relativeToRoot, err := filepath.Rel(realRoot, realCandidate)
	if err != nil {
		return "", err
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeToRoot) {
		return "", fmt.Errorf("path %s resolves outside the sendable directory", relative)
	}
	return realCandidate, nil
}

func fileDigest(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digestBytes(payload), nil
}

func digestBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
