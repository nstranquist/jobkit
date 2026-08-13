package main

import (
	"strings"
	"testing"
)

func TestReportAnnotatesAndFails(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"output","Package":"example.test/pkg","Test":"TestBroken","Output":"--- FAIL: TestBroken (0.00s)\\n"}`,
		`{"Action":"output","Package":"example.test/pkg","Test":"TestBroken","Output":"    broken_test.go:12: wanted safe state\\n"}`,
		`{"Action":"fail","Package":"example.test/pkg","Test":"TestBroken"}`,
		`{"Action":"fail","Package":"example.test/pkg"}`,
	}, "\n")
	var output strings.Builder
	failed, err := report(strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if !failed {
		t.Fatal("report accepted a failing test stream")
	}
	got := output.String()
	if !strings.Contains(got, "::error title=example.test/pkg/TestBroken::") || !strings.Contains(got, "wanted safe state") {
		t.Fatalf("annotation = %q", got)
	}
	if strings.Count(got, "::error ") != 1 {
		t.Fatalf("annotations = %q, want one test-level annotation", got)
	}
}

func TestReportAcceptsPassingStream(t *testing.T) {
	input := `{"Action":"pass","Package":"example.test/pkg"}`
	var output strings.Builder
	failed, err := report(strings.NewReader(input), &output)
	if err != nil || failed {
		t.Fatalf("failed=%v err=%v", failed, err)
	}
}
