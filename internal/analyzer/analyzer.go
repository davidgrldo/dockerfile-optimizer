package analyzer

import "github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Finding struct {
	ID       string
	Severity Severity
	Message  string
	Range    dockerfile.Range
	Stage    *int
}

type Result struct {
	Source        string
	DetectedStack Stack
	SelectedStack Stack
	Supported     bool
	Findings      []Finding
}

// Analyze runs the applicable rules over doc. override selects a stack
// explicitly; an empty override uses the detected stack.
func Analyze(doc *dockerfile.Document, override Stack) Result {
	detected := DetectStack(doc)
	selected := detected
	if override != "" {
		selected = override
	}
	result := Result{
		Source:        doc.Name,
		DetectedStack: detected,
		SelectedStack: selected,
		Supported:     IsSupported(selected),
		Findings:      []Finding{},
	}
	for _, r := range registeredRules {
		if !r.appliesTo(selected) {
			continue
		}
		for _, f := range r.check(doc) {
			f.ID = r.id
			f.Severity = r.severity
			result.Findings = append(result.Findings, f)
		}
	}
	return result
}
