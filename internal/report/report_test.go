package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
)

func TestPrintJSON_Empty(t *testing.T) {
	results := []analyzer.Suggestion{}
	stack := "go"

	var buf bytes.Buffer
	orig := out // save original output
	out = &buf  // redirect
	defer func() {
		out = orig // restore after test
	}()

	PrintJSON(results, stack) // call AFTER redirection

	got := buf.String()
	if !strings.Contains(got, `"suggestions": []`) {
		t.Errorf("Expected empty suggestions array in JSON; got: %s", got)
	}
	if !strings.Contains(got, `"stack": "go"`) {
		t.Errorf("JSON missing stack field; got: %s", got)
	}
}

func TestPrintHuman_WithSuggestions(t *testing.T) {
	results := []analyzer.Suggestion{
		{Description: "Test issue", Severity: "warn"},
	}

	var buf bytes.Buffer
	orig := out
	out = &buf
	PrintHuman(results)
	out = orig

	got := buf.String()
	if !strings.Contains(got, "Optimization Suggestions") {
		t.Error("Human output missing header; got:", got)
	}
	if !strings.Contains(got, "[warn] Test issue") {
		t.Error("Human output missing suggestion; got:", got)
	}
}

func TestPrintHuman_NoIssues(t *testing.T) {
	results := []analyzer.Suggestion{}
	var buf bytes.Buffer
	orig := out
	out = &buf
	PrintHuman(results)
	out = orig

	got := buf.String()
	if !strings.Contains(got, "No issues found") {
		t.Error("Expected 'No issues found' message; got:", got)
	}
}
