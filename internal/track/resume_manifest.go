package track

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var sha256Digest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var requiredResumeManifestGates = [...]string{
	"source",
	"claims",
	"parity",
	"ats",
	"lifecycle_metadata",
	"visual_nvr",
}

// ResumePackageManifest is the stable subset of nicos-resume's package
// manifest that JobKit needs for application provenance. Unknown fields are
// deliberately tolerated so additive producer changes remain compatible.
type ResumePackageManifest struct {
	SchemaVersion   int               `json:"schema_version"`
	VariantID       string            `json:"variant_id"`
	Version         string            `json:"version"`
	Lifecycle       string            `json:"lifecycle"`
	Sendability     string            `json:"sendability"`
	Channel         string            `json:"channel"`
	ClaimSetVersion string            `json:"claim_set_version"`
	SourceDigest    string            `json:"source_digest"`
	ClaimSetDigest  string            `json:"claim_set_digest"`
	Artifacts       map[string]string `json:"artifacts"`
	Gates           map[string]string `json:"gates"`
}

// ResumeManifestTags validates a nicos-resume package receipt and returns the
// canonical tags to persist in JobKit's append-only application ledger.
// Candidate/history artifacts fail closed even if their files exist locally.
func ResumeManifestTags(path, artifactKind, artifactPath string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest ResumePackageManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse resume manifest %s: %w", path, err)
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("resume manifest schema_version=%d; JobKit supports 1", manifest.SchemaVersion)
	}
	if manifest.Sendability != "sendable" || manifest.Channel != "sendable" {
		return nil, fmt.Errorf("resume manifest %s is %s/%s, not sendable/sendable", path, manifest.Sendability, manifest.Channel)
	}
	if manifest.Lifecycle != "current" && manifest.Lifecycle != "historical" {
		return nil, fmt.Errorf("resume manifest lifecycle %q is not current or explicitly sendable historical", manifest.Lifecycle)
	}
	if manifest.VariantID == "" || manifest.Version == "" || manifest.ClaimSetVersion == "" {
		return nil, fmt.Errorf("resume manifest is missing variant_id, version, or claim_set_version")
	}
	for _, gate := range requiredResumeManifestGates {
		if status, ok := manifest.Gates[gate]; !ok {
			return nil, fmt.Errorf("resume manifest is missing required gate %s", gate)
		} else if status != "pass" {
			return nil, fmt.Errorf("resume manifest gate %s=%s; all gates must pass", gate, status)
		}
	}
	for gate, status := range manifest.Gates {
		if status != "pass" {
			return nil, fmt.Errorf("resume manifest gate %s=%s; all gates must pass", gate, status)
		}
	}
	kind := strings.ToLower(strings.TrimSpace(artifactKind))
	if kind == "" {
		kind = "pdf"
	}
	if kind != "pdf" && kind != "docx" && kind != "ats" {
		return nil, fmt.Errorf("resume artifact kind %q must be pdf, docx, or ats", artifactKind)
	}
	for _, requiredKind := range []string{"pdf", "docx", "ats"} {
		if !ValidSHA256Digest(manifest.Artifacts[requiredKind]) {
			return nil, fmt.Errorf("resume manifest artifact %s has invalid SHA-256 digest %q", requiredKind, manifest.Artifacts[requiredKind])
		}
	}
	if !ValidSHA256Digest(manifest.ClaimSetDigest) {
		return nil, fmt.Errorf("resume manifest has invalid claim_set_digest %q", manifest.ClaimSetDigest)
	}
	if !ValidSHA256Digest(manifest.SourceDigest) {
		return nil, fmt.Errorf("resume manifest has invalid source_digest %q", manifest.SourceDigest)
	}
	digest := manifest.Artifacts[kind]
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" {
		return nil, fmt.Errorf("resume artifact file is required to bind the manifest receipt to the submitted bytes")
	}
	actualDigest, err := fileSHA256Digest(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("hash resume artifact %s: %w", artifactPath, err)
	}
	if actualDigest != digest {
		return nil, fmt.Errorf("resume artifact %s digest %s does not match manifest %s digest %s", artifactPath, actualDigest, kind, digest)
	}
	return map[string]string{
		TagResumeVersion:        manifest.Version,
		TagResumeVariantID:      manifest.VariantID,
		TagResumeArtifactKind:   kind,
		TagResumeArtifactDigest: digest,
		TagResumeSourceDigest:   manifest.SourceDigest,
		TagClaimSetVersion:      manifest.ClaimSetVersion,
		TagClaimSetDigest:       manifest.ClaimSetDigest,
	}, nil
}

// ValidSHA256Digest reports whether value is the canonical digest form shared
// by nicos-resume and JobKit receipts.
func ValidSHA256Digest(value string) bool { return sha256Digest.MatchString(value) }

func fileSHA256Digest(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
