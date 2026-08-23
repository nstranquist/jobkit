package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/eligibility"
	"github.com/nstranquist/jobkit/internal/resumeartifact"
	"github.com/nstranquist/jobkit/internal/track"
)

func TestPrepareApplicationResumeCopiesVerifiedArtifactAndWritesReceipt(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Dir(resumeartifact.IndexPath(root))
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]resumeartifact.File{}
	for _, kind := range []string{"pdf", "docx", "ats"} {
		extension := map[string]string{"pdf": ".pdf", "docx": ".docx", "ats": ".txt"}[kind]
		name := "enterprise" + extension
		payload := []byte("verified-enterprise-" + kind)
		if err := os.WriteFile(filepath.Join(indexDir, name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		artifacts[kind] = resumeartifact.File{Path: name, Digest: resumeCatalogTestDigest(payload)}
	}
	manifest := track.ResumePackageManifest{
		SchemaVersion: 1, VariantID: "enterprise-v1", Version: "v1.2.3-enterprise",
		Lifecycle: "current", Sendability: "sendable", Channel: "sendable", ClaimSetVersion: "v7",
		SourceDigest: resumeCatalogTestDigest([]byte("source")), ClaimSetDigest: resumeCatalogTestDigest([]byte("claims")),
		Artifacts: map[string]string{
			"pdf": artifacts["pdf"].Digest, "docx": artifacts["docx"].Digest, "ats": artifacts["ats"].Digest,
		},
		Gates: map[string]string{
			"source": "pass", "claims": "pass", "parity": "pass", "ats": "pass",
			"lifecycle_metadata": "pass", "visual_nvr": "pass",
		},
	}
	manifestPayload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestName := "manifest-enterprise.json"
	if err := os.WriteFile(filepath.Join(indexDir, manifestName), manifestPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	index := resumeartifact.Index{
		SchemaVersion: 1, DefaultVariantID: "enterprise-v1",
		Entries: []resumeartifact.Entry{{
			Lane: "enterprise", VariantID: "enterprise-v1", Version: "v1.2.3-enterprise",
			Manifest:  resumeartifact.File{Path: manifestName, Digest: resumeCatalogTestDigest(manifestPayload)},
			Artifacts: artifacts, RoleTargets: []string{"enterprise platform engineer"}, MatchTerms: []string{"enterprise"},
		}},
	}
	indexPayload, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resumeartifact.IndexPath(root), indexPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "resume.html"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	selected, err := prepareApplicationResume(applicationResumeRequest{
		CatalogRoot: root, ArtifactKind: "pdf", Dir: dir, ApplicationID: "acme--enterprise",
		Company: "Acme", Role: "Enterprise Platform Engineer", SourceURL: "https://example.test/job",
		Description: "Build an enterprise platform.", MatchScore: 88,
		Assessment: &eligibility.Result{Status: eligibility.Eligible, HardConstraintsVerified: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.Name != "resume.pdf" || selected.ReceiptName != "resume-receipt.json" {
		t.Fatalf("selection = %#v", selected)
	}
	if _, err := os.Stat(filepath.Join(dir, "resume.html")); !os.IsNotExist(err) {
		t.Fatalf("stale resume.html was not removed: %v", err)
	}
	artifactPayload, err := os.ReadFile(filepath.Join(dir, "resume.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(artifactPayload) != "verified-enterprise-pdf" {
		t.Fatalf("artifact = %q", artifactPayload)
	}
	receiptPayload, err := os.ReadFile(filepath.Join(dir, selected.ReceiptName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(receiptPayload), root) {
		t.Fatal("receipt leaked the producer root")
	}
	var receipt applicationResumeReceipt
	if err := json.Unmarshal(receiptPayload, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.Selection.VariantID != "enterprise-v1" || receipt.Selection.ArtifactDigest != artifacts["pdf"].Digest {
		t.Fatalf("receipt = %#v", receipt)
	}
	if receipt.Tags[track.TagResumeManifestDigest] == "" || receipt.Tags[track.TagLane] != "enterprise" || receipt.Tags[track.TagTailoringReceiptID] != "nicos-resume:acme--enterprise" {
		t.Fatalf("tags = %#v", receipt.Tags)
	}
}

func TestRequestedCatalogResumeKindAndFileInventory(t *testing.T) {
	for _, test := range []struct {
		args []string
		kind string
		use  bool
	}{
		{args: []string{"apply-plan", "job.txt"}, kind: "pdf", use: true},
		{args: []string{"apply-plan", "job.txt", "--format", "ats"}, kind: "ats", use: true},
		{args: []string{"apply-plan", "job.txt", "--format", "docx"}, kind: "docx", use: true},
		{args: []string{"apply-plan", "job.txt", "--format", "html"}, use: false},
		{args: []string{"apply-plan", "job.txt", "--format", "html", "--resume-artifact", "pdf"}, kind: "pdf", use: true},
	} {
		kind, use := requestedCatalogResumeKind(parseArgs(test.args))
		if kind != test.kind || use != test.use {
			t.Fatalf("args %v: got (%q, %v), want (%q, %v)", test.args, kind, use, test.kind, test.use)
		}
	}
	names := packageFileNames(map[string]string{"plan.md": "x", "resume.pdf": "unused"}, "resume.pdf", "resume-receipt.json")
	if strings.Join(names, ",") != "plan.md,resume-receipt.json,resume.pdf" {
		t.Fatalf("files = %v", names)
	}
}

func resumeCatalogTestDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
