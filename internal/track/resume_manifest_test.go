package track

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeManifestTagsAcceptsVerifiedSendableArtifact(t *testing.T) {
	artifact := writeResumeArtifact(t, "verified PDF bytes")
	digest, err := fileSHA256Digest(artifact)
	if err != nil {
		t.Fatal(err)
	}
	sourceDigest := "sha256:" + strings.Repeat("c", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	path := writeResumeManifest(t, `{
  "schema_version": 1,
  "variant_id": "general-v1.7.3",
  "version": "v1.7.3",
  "lifecycle": "current",
  "sendability": "sendable",
  "channel": "sendable",
  "claim_set_version": "v1.0.0",
  "source_digest": "`+sourceDigest+`",
  "claim_set_digest": "`+claimDigest+`",
  "artifacts": {"pdf": "`+digest+`", "docx": "`+digest+`", "ats": "`+digest+`"},
  "gates": {
    "source": "pass",
    "claims": "pass",
    "parity": "pass",
    "ats": "pass",
    "lifecycle_metadata": "pass",
    "visual_nvr": "pass"
  },
  "future_additive_field": true
}`)
	tags, err := ResumeManifestTags(path, "pdf", artifact)
	if err != nil {
		t.Fatal(err)
	}
	if tags[TagResumeArtifactDigest] != digest || tags[TagResumeSourceDigest] != sourceDigest || tags[TagResumeVariantID] != "general-v1.7.3" || tags[TagClaimSetVersion] != "v1.0.0" {
		t.Fatalf("tags = %#v", tags)
	}
}

func TestResumeManifestTagsRejectsCandidateAndFailedGate(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	sourceDigest := "sha256:" + strings.Repeat("c", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	gates := `"gates":{"source":"pass","claims":"pass","parity":"pass","ats":"pass","lifecycle_metadata":"pass","visual_nvr":"pass"}`
	candidate := writeResumeManifest(t, `{"schema_version":1,"variant_id":"candidate","version":"v1.8.0-rc.1","lifecycle":"candidate","sendability":"prohibited","channel":"candidates","claim_set_version":"v1","source_digest":"`+sourceDigest+`","claim_set_digest":"`+claimDigest+`","artifacts":{"pdf":"`+digest+`","docx":"`+digest+`","ats":"`+digest+`"},`+gates+`}`)
	if _, err := ResumeManifestTags(candidate, "pdf", "unused"); err == nil {
		t.Fatal("expected candidate manifest to fail closed")
	}
	failed := writeResumeManifest(t, `{"schema_version":1,"variant_id":"general","version":"v1","lifecycle":"current","sendability":"sendable","channel":"sendable","claim_set_version":"v1","source_digest":"`+sourceDigest+`","claim_set_digest":"`+claimDigest+`","artifacts":{"pdf":"`+digest+`","docx":"`+digest+`","ats":"`+digest+`"},"gates":{"source":"pass","claims":"pass","parity":"pass","ats":"pass","lifecycle_metadata":"pass","visual_nvr":"fail"}}`)
	if _, err := ResumeManifestTags(failed, "pdf", "unused"); err == nil {
		t.Fatal("expected failed gate to fail closed")
	}
}

func TestResumeManifestTagsRejectsMissingRequiredGate(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	sourceDigest := "sha256:" + strings.Repeat("c", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	path := writeResumeManifest(t, `{
  "schema_version": 1,
  "variant_id": "general-v1.7.3",
  "version": "v1.7.3",
  "lifecycle": "current",
  "sendability": "sendable",
  "channel": "sendable",
  "claim_set_version": "v1.0.0",
  "source_digest": "`+sourceDigest+`",
  "claim_set_digest": "`+claimDigest+`",
  "artifacts": {"pdf": "`+digest+`", "docx": "`+digest+`", "ats": "`+digest+`"},
  "gates": {
    "source": "pass",
    "claims": "pass",
    "ats": "pass",
    "lifecycle_metadata": "pass",
    "visual_nvr": "pass"
  }
}`)
	if _, err := ResumeManifestTags(path, "pdf", "unused"); err == nil || !strings.Contains(err.Error(), "missing required gate parity") {
		t.Fatalf("error = %v, want missing parity gate", err)
	}
}

func TestResumeManifestTagsRejectsMissingSourceDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	path := writeResumeManifest(t, `{
  "schema_version": 1,
  "variant_id": "general-v1.7.3",
  "version": "v1.7.3",
  "lifecycle": "current",
  "sendability": "sendable",
  "channel": "sendable",
  "claim_set_version": "v1.0.0",
  "claim_set_digest": "`+claimDigest+`",
  "artifacts": {"pdf": "`+digest+`", "docx": "`+digest+`", "ats": "`+digest+`"},
  "gates": {
    "source": "pass",
    "claims": "pass",
    "parity": "pass",
    "ats": "pass",
    "lifecycle_metadata": "pass",
    "visual_nvr": "pass"
  }
}`)
	if _, err := ResumeManifestTags(path, "pdf", "unused"); err == nil || !strings.Contains(err.Error(), "invalid source_digest") {
		t.Fatalf("error = %v, want missing source digest", err)
	}
}

func TestResumeManifestTagsRejectsArtifactDigestMismatch(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	sourceDigest := "sha256:" + strings.Repeat("c", 64)
	claimDigest := "sha256:" + strings.Repeat("b", 64)
	path := writeResumeManifest(t, `{
  "schema_version": 1,
  "variant_id": "general-v1.7.3",
  "version": "v1.7.3",
  "lifecycle": "current",
  "sendability": "sendable",
  "channel": "sendable",
  "claim_set_version": "v1.0.0",
  "source_digest": "`+sourceDigest+`",
  "claim_set_digest": "`+claimDigest+`",
  "artifacts": {"pdf": "`+digest+`", "docx": "`+digest+`", "ats": "`+digest+`"},
  "gates": {
    "source": "pass",
    "claims": "pass",
    "parity": "pass",
    "ats": "pass",
    "lifecycle_metadata": "pass",
    "visual_nvr": "pass"
  }
}`)
	artifact := writeResumeArtifact(t, "different bytes")
	if _, err := ResumeManifestTags(path, "pdf", artifact); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("error = %v, want artifact digest mismatch", err)
	}
}

func writeResumeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeResumeArtifact(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
