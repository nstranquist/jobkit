// Command license-audit verifies the licenses of every compiled external Go
// module. New dependencies and changed upstream license texts fail closed.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type approvedLicense struct {
	Version    string
	Expression string
	SHA256     string
}

var approved = map[string]approvedLicense{
	"golang.org/x/net": {
		Version: "v0.58.0", Expression: "BSD-3-Clause",
		SHA256: "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
	},
	"golang.org/x/sys": {
		Version: "v0.47.0", Expression: "BSD-3-Clause",
		SHA256: "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
	},
	"go.yaml.in/yaml/v3": {
		Version: "v3.0.5", Expression: "MIT AND Apache-2.0",
		SHA256: "d18f6323b71b0b768bb5e9616e36da390fbd39369a81807cca352de4e4e6aa0b",
	},
}

type dependency struct {
	Path    string
	Version string
	Dir     string
}

func main() {
	deps, err := compiledDependencies()
	if err != nil {
		fmt.Fprintln(os.Stderr, "license audit:", err)
		os.Exit(1)
	}
	if err := verify(deps); err != nil {
		fmt.Fprintln(os.Stderr, "license audit:", err)
		os.Exit(1)
	}
	for _, dep := range deps {
		license := approved[dep.Path]
		fmt.Printf("ok  %s %s  %s\n", dep.Path, dep.Version, license.Expression)
	}
}

func compiledDependencies() ([]dependency, error) {
	format := "{{if and .Module (not .Module.Main)}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}"
	cmd := exec.Command("go", "list", "-deps", "-f", format, "./...")
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	byPath := map[string]dependency{}
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		byPath[fields[0]] = dependency{Path: fields[0], Version: fields[1], Dir: fields[2]}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	deps := make([]dependency, 0, len(byPath))
	for _, dep := range byPath {
		deps = append(deps, dep)
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
	return deps, nil
}

func verify(deps []dependency) error {
	seen := map[string]bool{}
	for _, dep := range deps {
		expected, ok := approved[dep.Path]
		if !ok {
			return fmt.Errorf("unreviewed compiled dependency %s %s", dep.Path, dep.Version)
		}
		if dep.Version != expected.Version {
			return fmt.Errorf("%s version %s, reviewed %s", dep.Path, dep.Version, expected.Version)
		}
		raw, err := os.ReadFile(filepath.Join(dep.Dir, "LICENSE"))
		if err != nil {
			return fmt.Errorf("read %s license: %w", dep.Path, err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != expected.SHA256 {
			return fmt.Errorf("%s license text changed; review required", dep.Path)
		}
		seen[dep.Path] = true
	}
	for path := range approved {
		if !seen[path] {
			return fmt.Errorf("reviewed dependency %s is no longer compiled; update the audit", path)
		}
	}
	return nil
}
