package analyzer

import "testing"

func TestAnalyzeInitializesFindings(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM alpine\n"), StackGeneric)
	if result.Findings == nil {
		t.Fatal("expected non-nil findings")
	}
}
