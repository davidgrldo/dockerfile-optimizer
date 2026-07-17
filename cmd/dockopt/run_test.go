package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

func TestRunCleanGoEnvCGO(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("clean-go-env"))
	if code != 0 {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Stack.Selected != analyzer.StackGo || len(output.Findings) != 0 {
		t.Fatalf("CGO disabled via ENV must be clean: output=%#v", output)
	}
}

func TestRunUntaggedBaseWarns(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--fail-on", "warn", fixturePath("warn-untagged"))
	if code != 1 || !strings.Contains(stdout, "GEN001") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunHeredocAndHereStringParse(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("heredoc-herestring"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
}

func TestRunComplexMultistageIsClean(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("complex-multistage"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Stack.Detected != analyzer.StackGo || len(output.Findings) != 0 {
		t.Fatalf("complex multi-stage build must parse clean: output=%#v", output)
	}
}

func TestRunMultipleFilesEmitJSONLines(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("clean"), fixturePath("warn-latest"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 NDJSON lines, got %d: %q", len(lines), stdout)
	}
	for _, line := range lines {
		var out report.Output
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			t.Fatalf("line %q not a valid Output: %v", line, err)
		}
	}
}

func TestRunMultipleFilesTakeMaxExitCode(t *testing.T) {
	// clean (0) + error-go-static (error finding -> 1) => 1
	if _, _, code := runWithBuffers(fixturePath("clean"), fixturePath("error-go-static")); code != 1 {
		t.Fatalf("threshold aggregate code=%d, want 1", code)
	}
	// clean (0) + malformed (parse error -> 2) => 2, even though clean is fine
	if _, _, code := runWithBuffers("--json", fixturePath("clean"), fixturePath("malformed")); code != 2 {
		t.Fatalf("parse-error aggregate code=%d, want 2", code)
	}
}

func TestRunMultipleFilesHumanHeaders(t *testing.T) {
	stdout, stderr, code := runWithBuffers(fixturePath("generic"), fixturePath("warn-latest"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
	for _, path := range []string{fixturePath("generic"), fixturePath("warn-latest")} {
		if !strings.Contains(stdout, "==> "+path+" <==") {
			t.Fatalf("missing header for %s in %q", path, stdout)
		}
	}
}

func TestRunFlagAfterPathIsUsageError(t *testing.T) {
	_, stderr, code := runWithBuffers(fixturePath("clean"), fixturePath("generic"), "--json")
	if code != 2 || !strings.Contains(stderr, "usage_error") {
		t.Fatalf("code=%d, stderr=%q", code, stderr)
	}
}

func TestRunAptAntiPatternsWarn(t *testing.T) {
	// default threshold is error: four warn-grade findings do not fail the build.
	stdout, stderr, code := runWithBuffers("--json", fixturePath("warn-apt"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	ids := make(map[string]bool, len(output.Findings))
	for _, finding := range output.Findings {
		ids[finding.ID] = true
	}
	for _, want := range []string{"GEN002", "GEN003", "GEN004", "GEN005"} {
		if !ids[want] {
			t.Errorf("finding %s absent; got %#v", want, output.Findings)
		}
	}

	// under --fail-on warn the same findings fail the build.
	if _, _, code := runWithBuffers("--fail-on", "warn", fixturePath("warn-apt")); code != 1 {
		t.Fatalf("warn threshold code=%d, want 1", code)
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

func TestRunTrailingContinuationJSONParseError(t *testing.T) {
	stdout, stderr, code := runWithBuffers("--json", fixturePath("unterminated-continuation"))
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if output.Error.Kind != "parse_error" || !strings.Contains(output.Error.Message, ":2:") {
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

func TestRunJSONIntentAliasesRouteFlagErrorsToJSON(t *testing.T) {
	for _, alias := range []string{"-json", "--json=1", "--json=TRUE"} {
		t.Run(alias, func(t *testing.T) {
			stdout, stderr, code := runWithBuffers(alias, "--stack")
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
		})
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
		{name: "single dash", args: []string{"-json"}, want: true},
		{name: "numeric true", args: []string{"--json=1"}, want: true},
		{name: "uppercase true", args: []string{"--json=TRUE"}, want: true},
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

func TestRunCloseErrorUsesJSONInputErrorBeforeReporting(t *testing.T) {
	reader := &closeErrorReader{Reader: strings.NewReader("FROM alpine:3.20\n"), err: errors.New("close failed")}
	var stdout, stderr bytes.Buffer
	code := runWithOpener([]string{"--json", "Dockerfile"}, &stdout, &stderr, func(string) (io.ReadCloser, error) {
		return reader, nil
	})
	if code != 2 || stderr.String() != "" || !reader.closed {
		t.Fatalf("code=%d, stdout=%q, stderr=%q, closed=%v", code, stdout.String(), stderr.String(), reader.closed)
	}
	var output report.ErrorOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if output.Error.Kind != "input_error" || !strings.Contains(output.Error.Message, "close failed") {
		t.Fatalf("output=%#v", output)
	}
}

func TestRunGenericAnalysisNeverClaimsStackSpecificChecks(t *testing.T) {
	stdout, stderr, code := runWithBuffers(fixturePath("generic"))
	if code != 0 || stderr != "" || !strings.Contains(stdout, "generic checks only") || strings.Contains(stdout, "Stack-specific checks enabled") || strings.Contains(stdout, "No issues found") {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runWithBuffers("--json", fixturePath("generic"))
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	var output report.Output
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatal(err)
	}
	if output.Stack.Selected != analyzer.StackGeneric || output.Stack.Supported {
		t.Fatalf("stack=%#v", output.Stack)
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

type closeErrorReader struct {
	io.Reader
	err    error
	closed bool
}

func (r *closeErrorReader) Close() error {
	r.closed = true
	return r.err
}
