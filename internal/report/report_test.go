package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

func TestWriteJSONV1EmptyArray(t *testing.T) {
	result := analyzer.Result{
		Source:        "Dockerfile",
		DetectedStack: analyzer.StackPython,
		SelectedStack: analyzer.StackPython,
		Findings:      []analyzer.Finding{},
	}
	var buffer bytes.Buffer
	if err := WriteJSON(&buffer, result); err != nil {
		t.Fatal(err)
	}

	var output Output
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != "1" || output.Findings == nil {
		t.Fatalf("output=%#v", output)
	}
}

func TestNewOutputMapsResultAndCountsSeverities(t *testing.T) {
	stage := 1
	result := analyzer.Result{
		Source:        "Dockerfile.prod",
		DetectedStack: analyzer.StackGo,
		SelectedStack: analyzer.StackRust,
		Supported:     true,
		Findings: []analyzer.Finding{
			{ID: "one", Severity: analyzer.SeverityInfo, Message: "info", Range: dockerfile.Range{StartLine: 2, EndLine: 2}},
			{ID: "two", Severity: analyzer.SeverityWarn, Message: "warn", Range: dockerfile.Range{StartLine: 4, EndLine: 6}, Stage: &stage},
			{ID: "three", Severity: analyzer.SeverityError, Message: "error", Range: dockerfile.Range{StartLine: 8, EndLine: 8}},
		},
	}

	output := NewOutput(result)
	if output.Source != result.Source || output.Stack.Detected != result.DetectedStack || output.Stack.Selected != result.SelectedStack || !output.Stack.Supported {
		t.Fatalf("output=%#v", output)
	}
	if output.Summary != (Summary{Info: 1, Warn: 1, Error: 1}) {
		t.Fatalf("summary=%#v", output.Summary)
	}
	want := FindingOutput{ID: "two", Severity: analyzer.SeverityWarn, Message: "warn", Line: 4, EndLine: 6, Stage: &stage}
	if got := output.Findings[1]; got.ID != want.ID || got.Severity != want.Severity || got.Message != want.Message || got.Line != want.Line || got.EndLine != want.EndLine || got.Stage == nil || *got.Stage != *want.Stage {
		t.Fatalf("finding=%#v", got)
	}
}

func TestWriteJSONUsesExplicitLowercaseFieldNames(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteJSON(&buffer, analyzer.Result{}); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(buffer.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"schema_version", "source", "stack", "findings", "summary"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("missing JSON field %q in %s", name, buffer.String())
		}
	}
}

func TestWriteHumanUnsupportedReportsGenericChecksOnly(t *testing.T) {
	result := analyzer.Result{DetectedStack: analyzer.StackPython, SelectedStack: analyzer.StackPython, Supported: false}
	var buffer bytes.Buffer
	if err := WriteHuman(&buffer, result); err != nil {
		t.Fatal(err)
	}
	if got := buffer.String(); !strings.Contains(got, "generic checks only") || strings.Contains(got, "No issues found") {
		t.Fatalf("output=%q", got)
	}
}

func TestWriteHumanFormatsFindingLines(t *testing.T) {
	result := analyzer.Result{
		DetectedStack: analyzer.StackGo,
		SelectedStack: analyzer.StackGo,
		Supported:     true,
		Findings: []analyzer.Finding{
			{ID: "single", Severity: analyzer.SeverityWarn, Message: "single line", Range: dockerfile.Range{StartLine: 3, EndLine: 3}},
			{ID: "range", Severity: analyzer.SeverityError, Message: "line range", Range: dockerfile.Range{StartLine: 5, EndLine: 7}},
		},
	}
	var buffer bytes.Buffer
	if err := WriteHuman(&buffer, result); err != nil {
		t.Fatal(err)
	}
	got := buffer.String()
	for _, want := range []string{"Detected stack: go", "[warn] single (line 3): single line", "[error] range (lines 5-7): line range"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestWriteHumanSupportedEmptyReportsNoIssues(t *testing.T) {
	result := analyzer.Result{DetectedStack: analyzer.StackGo, SelectedStack: analyzer.StackGo, Supported: true}
	var buffer bytes.Buffer
	if err := WriteHuman(&buffer, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "No issues found") {
		t.Fatalf("output=%q", buffer.String())
	}
}

func TestWritersPropagateErrors(t *testing.T) {
	want := errors.New("broken pipe")
	writer := errorWriter{err: want}
	for name, write := range map[string]func() error{
		"json":  func() error { return WriteJSON(writer, analyzer.Result{}) },
		"human": func() error { return WriteHuman(writer, analyzer.Result{}) },
		"error": func() error { return WriteErrorJSON(writer, "output_error", "broken pipe") },
	} {
		t.Run(name, func(t *testing.T) {
			if err := write(); !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
		})
	}
}

func TestWriteErrorJSONV1(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteErrorJSON(&buffer, "parse_error", "Dockerfile:4: invalid FROM"); err != nil {
		t.Fatal(err)
	}
	var output ErrorOutput
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != "1" || output.Error.Kind != "parse_error" || output.Error.Message != "Dockerfile:4: invalid FROM" {
		t.Fatalf("output=%#v", output)
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
