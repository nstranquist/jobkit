// Command ci-test-report converts a go test -json stream into normal test
// output plus GitHub Actions annotations. It exits with a failure when any Go
// package fails, so a pipeline does not hide the go test exit status.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxAnnotationBytes = 8000

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func main() {
	failed, err := report(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ci-test-report: %v\n", err)
		os.Exit(2)
	}
	if failed {
		os.Exit(1)
	}
}

func report(input io.Reader, output io.Writer) (bool, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	buffers := map[string]string{}
	annotatedPackages := map[string]bool{}
	failed := false
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			fmt.Fprintln(output, scanner.Text())
			continue
		}
		if event.Output != "" {
			fmt.Fprint(output, event.Output)
			key := eventKey(event)
			buffers[key] = tail(buffers[key]+event.Output, maxAnnotationBytes)
			if event.Test != "" {
				buffers[event.Package] = tail(buffers[event.Package]+event.Output, maxAnnotationBytes)
			}
		}
		if event.Action != "fail" {
			continue
		}
		failed = true
		if event.Test == "" && annotatedPackages[event.Package] {
			continue
		}
		message := strings.TrimSpace(buffers[eventKey(event)])
		if message == "" {
			message = "go test reported a failure"
		}
		title := event.Package
		if event.Test != "" {
			title += "/" + event.Test
			annotatedPackages[event.Package] = true
		}
		fmt.Fprintf(output, "::error title=%s::%s\n", escapeProperty(title), escapeData(message))
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return failed, nil
}

func eventKey(event testEvent) string {
	if event.Test == "" {
		return event.Package
	}
	return event.Package + "\x00" + event.Test
}

func tail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func escapeProperty(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	value = strings.ReplaceAll(value, "\n", "%0A")
	value = strings.ReplaceAll(value, ":", "%3A")
	return strings.ReplaceAll(value, ",", "%2C")
}

func escapeData(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")
	return strings.ReplaceAll(value, "\n", "%0A")
}
