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

type Context struct {
	Stack Stack
}

type Rule interface {
	ID() string
	Severity() Severity
	Stacks() []Stack
	Evaluate(*dockerfile.Document, Context) []Finding
}

func Analyze(doc *dockerfile.Document, selected Stack) Result {
	result := Result{
		Source:        doc.Name,
		DetectedStack: DetectStack(doc),
		SelectedStack: selected,
		Supported:     IsSupported(selected),
		Findings:      []Finding{},
	}
	context := Context{Stack: selected}
	for _, rule := range Rules() {
		if appliesTo(rule, selected) {
			result.Findings = append(result.Findings, rule.Evaluate(doc, context)...)
		}
	}
	return result
}

func appliesTo(rule Rule, selected Stack) bool {
	for _, stack := range rule.Stacks() {
		if stack == StackGeneric || stack == selected {
			return true
		}
	}
	return false
}
