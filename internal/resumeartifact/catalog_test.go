package resumeartifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nstranquist/jobkit/internal/track"
)

func TestSelectUsesRoleEvidenceAndVerifiesReceipt(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Dir(IndexPath(root))
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	platform := writeEntry(t, indexDir, "platform", "platform-v1", "v1", []string{"platform", "developer tooling"})
	enterprise := writeEntry(t, indexDir, "enterprise", "enterprise-v1", "v2", []string{"enterprise", "cloud modernization", "consulting"})
	index := Index{
		SchemaVersion: 1, DefaultVariantID: platform.VariantID,
		Entries: []Entry{platform, enterprise},
	}
	writeIndex(t, root, index)
	selection, err := Select(root, Query{
		Role: "Enterprise AI Engineer", Description: "Lead enterprise cloud modernization and consulting delivery.",
		PreferredLane: "platform-devex-ai-infra", ArtifactKind: "pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.VariantID != enterprise.VariantID || selection.Tags[track.TagResumeVariantID] != enterprise.VariantID {
		t.Fatalf("selection = %#v", selection)
	}
	if selection.Tags[track.TagResumeManifestDigest] == "" || selection.ManifestDigest == "" {
		t.Fatalf("manifest digest is missing: %#v", selection.Tags)
	}
	destination, err := selection.CopyTo(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(destination) != "resume.pdf" {
		t.Fatalf("destination = %s", destination)
	}
}

func TestSelectHonorsFullstackLane(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Dir(IndexPath(root))
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	platform := writeEntry(t, indexDir, "platform", "platform-v1", "v1", []string{"platform"})
	fullstack := writeEntry(t, indexDir, "fullstack", "fullstack-v1", "v1-fullstack", []string{"full stack", "product"})
	writeIndex(t, root, Index{SchemaVersion: 1, DefaultVariantID: platform.VariantID, Entries: []Entry{platform, fullstack}})
	selection, err := Select(root, Query{Role: "Product Engineer", PreferredLane: "fullstack-product", ArtifactKind: "docx"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.VariantID != fullstack.VariantID || selection.ArtifactKind != "docx" {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectDoesNotLetGenericDescriptionOverridePlatformLane(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Dir(IndexPath(root))
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	platform := writeEntry(t, indexDir, "platform", "platform-v1", "v1", []string{"platform", "infrastructure"})
	fullstack := writeEntry(t, indexDir, "fullstack", "fullstack-v1", "v1-fullstack", []string{"full stack", "product", "SaaS"})
	writeIndex(t, root, Index{SchemaVersion: 1, DefaultVariantID: platform.VariantID, Entries: []Entry{platform, fullstack}})

	selection, err := Select(root, Query{
		Role:          "Software Engineer, Compute Infrastructure",
		Description:   "Build the modern cloud platform for developers creating full-stack SaaS product applications.",
		PreferredLane: "platform-devex-ai-infra",
		ArtifactKind:  "pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.VariantID != platform.VariantID {
		t.Fatalf("selection = %#v", selection)
	}
	if !strings.Contains(strings.Join(selection.Reasons, "\n"), "preferred lane platform") {
		t.Fatalf("selection reasons = %v", selection.Reasons)
	}
}

func TestSelectRejectsDigestMismatchAndPathEscape(t *testing.T) {
	root := t.TempDir()
	indexDir := filepath.Dir(IndexPath(root))
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := writeEntry(t, indexDir, "platform", "platform-v1", "v1", []string{"platform"})
	entry.Artifacts["pdf"] = File{Path: "../private.pdf", Digest: entry.Artifacts["pdf"].Digest}
	writeIndex(t, root, Index{SchemaVersion: 1, DefaultVariantID: entry.VariantID, Entries: []Entry{entry}})
	if _, err := Select(root, Query{Role: "Platform Engineer"}); err == nil {
		t.Fatal("path escape must fail")
	}

	entry = writeEntry(t, indexDir, "platform", "platform-v2", "v2", []string{"platform"})
	entry.Artifacts["pdf"] = File{Path: entry.Artifacts["pdf"].Path, Digest: "sha256:" + string(make([]byte, 64))}
	writeIndex(t, root, Index{SchemaVersion: 1, DefaultVariantID: entry.VariantID, Entries: []Entry{entry}})
	if _, err := Select(root, Query{Role: "Platform Engineer"}); err == nil {
		t.Fatal("digest mismatch must fail")
	}
}

func writeEntry(t *testing.T, dir, lane, variant, version string, terms []string) Entry {
	t.Helper()
	artifacts := map[string]File{}
	for _, kind := range []string{"pdf", "docx", "ats"} {
		name := variant + "." + map[string]string{"pdf": "pdf", "docx": "docx", "ats": "txt"}[kind]
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(variant+"-"+kind), 0o600); err != nil {
			t.Fatal(err)
		}
		digest, err := fileDigest(path)
		if err != nil {
			t.Fatal(err)
		}
		artifacts[kind] = File{Path: name, Digest: digest}
	}
	manifest := track.ResumePackageManifest{
		SchemaVersion: 1, VariantID: variant, Version: version, Lifecycle: "current", Sendability: "sendable", Channel: "sendable",
		ClaimSetVersion: "v1", SourceDigest: testDigest("source"), ClaimSetDigest: testDigest("claims"),
		Artifacts: map[string]string{"pdf": artifacts["pdf"].Digest, "docx": artifacts["docx"].Digest, "ats": artifacts["ats"].Digest},
		Gates:     map[string]string{"source": "pass", "claims": "pass", "parity": "pass", "ats": "pass", "lifecycle_metadata": "pass", "visual_nvr": "pass"},
	}
	manifestName := "manifest-" + variant + ".json"
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return Entry{
		Lane: lane, VariantID: variant, Version: version,
		Manifest: File{Path: manifestName, Digest: digestBytes(payload)}, Artifacts: artifacts,
		MatchTerms: terms, RoleTargets: []string{lane + " engineer"}, Order: 10,
	}
}

func writeIndex(t *testing.T, root string, index Index) {
	t.Helper()
	payload, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(IndexPath(root), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testDigest(value string) string {
	return digestBytes([]byte(value))
}
