package analyzer

import (
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

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
// explicitly; an empty override uses the detected stack. ignore suppresses
// findings whose IDs match (case-insensitive), in addition to per-instruction
// # dockopt:disable comments.
func Analyze(doc *dockerfile.Document, override Stack, ignore ...string) Result {
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
	ignored := ignoreSet(ignore)
	for _, r := range registeredRules {
		if !r.appliesTo(selected) {
			continue
		}
		for _, f := range r.check(doc) {
			f.ID = r.id
			f.Severity = r.severity
			if ignored[strings.ToUpper(r.id)] || instructionDisables(doc, f, r.id) {
				continue
			}
			result.Findings = append(result.Findings, f)
		}
	}
	return result
}

func ignoreSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		for _, part := range strings.Split(id, ",") {
			if normalized := strings.ToUpper(strings.TrimSpace(part)); normalized != "" {
				set[normalized] = true
			}
		}
	}
	return set
}

func instructionDisables(doc *dockerfile.Document, f Finding, ruleID string) bool {
	for _, instruction := range doc.Instructions {
		if instruction.Range != f.Range {
			continue
		}
		for _, disabled := range instruction.Disabled {
			if strings.EqualFold(disabled, ruleID) {
				return true
			}
		}
	}
	return false
}

func KnownRuleID(id string) bool {
	want := strings.ToUpper(strings.TrimSpace(id))
	for _, r := range registeredRules {
		if r.id == want {
			return true
		}
	}
	return false
}
