package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

func TestRunCleanJSON(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("clean"))
	if code != 0 {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.SchemaVersion != "1" || output.Findings == nil || len(output.Findings) != 0 {
		t.Fatalf("output=%#v", output)
	}
}

func TestRunWarningThreshold(t *testing.T) {
	path := fixturePath("warn-latest")
	stdout, stderr, code := runWithBuffers(path)
	if code != 0 || !strings.Contains(stdout, "GEN001") {
		t.Fatalf("default: code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runWithBuffers("--fail-on", "warn", path)
	if code != 1 || !strings.Contains(stdout, "GEN001") {
		t.Fatalf("warn: code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunStaticGoError(t *testing.T) {
	stdout, stderr, code := runWithBuffers(fixturePath("error-go-static"))
	if code != 1 || !strings.Contains(stdout, "GO002") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunRejectsUnknownStack(t *testing.T) {
	_, stderr, code := runWithBuffers("--stack", "golnag", fixturePath("clean"))
	if code != 2 || !strings.Contains(stderr, "unknown stack") {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
}

func TestRunUnsupportedStackDoesNotClaimClean(t *testing.T) {
	stdout, stderr, code := runWithBuffers(fixturePath("unsupported-python"))
	if code != 0 || !strings.Contains(stdout, "generic checks only") || strings.Contains(stdout, "No issues found") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunMalformedJSONError(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("malformed"))
	if code != 2 {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Error.Kind != "parse_error" {
		t.Fatalf("output=%#v", output)
	}
}

func TestRunMissingFile(t *testing.T) {
	_, stderr, code := runWithBuffers("missing.Dockerfile")
	if code != 2 || !strings.Contains(stderr, "input_error") {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
}

func TestRunRejectsInvalidThreshold(t *testing.T) {
	_, stderr, code := runWithBuffers("--fail-on", "warning", fixturePath("clean"))
	if code != 2 || !strings.Contains(stderr, "fail-on") {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
}

func TestRunRequiresFlagsBeforePath(t *testing.T) {
	stdout, stderr, code := runWithBuffers(fixturePath("clean"), "--json")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "usage_error") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunFlagParseErrorUsesRequestedJSON(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", "--stack")
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Error.Kind != "usage_error" {
		t.Fatalf("output=%#v", output)
	}
}

func TestRunUnknownFlagBeforeJSONUsesJSONError(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--bogus", "--json", fixturePath("clean"))
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Error.Kind != "usage_error" {
		t.Fatalf("output=%#v", output)
	}
}

func TestRequestsJSONStopsAtPathOrFlagTerminator(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "flag", args: []string{"--json"}, want: true},
		{name: "explicit true", args: []string{"--json=true"}, want: true},
		{name: "explicit false", args: []string{"--json=false"}, want: false},
		{name: "after path", args: []string{"Dockerfile", "--json"}, want: false},
		{name: "after terminator", args: []string{"--", "--json"}, want: false},
		{name: "after invalid flag", args: []string{"--bogus", "--json", "Dockerfile"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := requestsJSON(test.args); got != test.want {
				t.Fatalf("requestsJSON(%q)=%v, want %v", test.args, got, test.want)
			}
		})
	}
}

func TestRunDirectoryReadErrorUsesJSONInputError(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", t.TempDir())
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Error.Kind != "input_error" {
		t.Fatalf("output=%#v", output)
	}
}

func TestRunOutputError(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{fixturePath("clean")}, failingWriter{errors.New("broken pipe")}, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "broken pipe") {
		t.Fatalf("code=%d, stderr=%q", code, stderr.String())
	}
}

func TestMeetsThreshold(t *testing.T) {
	findings := []analyzer.Finding{
		{Severity: analyzer.SeverityInfo},
		{Severity: analyzer.SeverityWarn},
		{Severity: analyzer.SeverityError},
	}
	for _, test := range []struct {
		name      string
		findings  []analyzer.Finding
		threshold string
		want      bool
	}{
		{name: "none", findings: findings, threshold: "none", want: false},
		{name: "warn matches warning", findings: findings[:2], threshold: "warn", want: true},
		{name: "warn matches error", findings: findings[2:], threshold: "warn", want: true},
		{name: "error ignores warning", findings: findings[:2], threshold: "error", want: false},
		{name: "error matches error", findings: findings[2:], threshold: "error", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := meetsThreshold(test.findings, test.threshold); got != test.want {
				t.Fatalf("meetsThreshold(%v, %q)=%v, want %v", test.findings, test.threshold, got, test.want)
			}
		})
	}
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "dockerfiles", name+".Dockerfile")
}

func runWithBuffers(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
