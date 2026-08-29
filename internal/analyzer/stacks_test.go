package analyzer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

func parseTestDocument(t *testing.T, input string) *dockerfile.Document {
	t.Helper()
	doc, err := dockerfile.Parse("Dockerfile", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDetectStackUsesParsedSyntax(t *testing.T) {
	tests := []struct {
		input string
		want  Stack
	}{
		{"# Rust is not used\nFROM alpine\n", StackGeneric},
		{"FROM golang:1.24\nRUN go build ./...\n", StackGo},
		{"FROM acme/notgolang-runtime\n", StackGeneric},
		{"FROM alpine\nRUN go   build ./...\n", StackGo},
		{"FROM python:3.12-slim\n", StackPython},
	}
	for _, test := range tests {
		if got := DetectStack(parseTestDocument(t, test.input)); got != test.want {
			t.Errorf("got=%q want=%q", got, test.want)
		}
	}
}

func TestDetectStackPrefersFirstBaseImageEvidenceOverLaterRuns(t *testing.T) {
	tests := []struct {
		input string
		want  Stack
	}{
		{"FROM python:3.12\nRUN go build ./...\n", StackPython},
		{"FROM node:22\nRUN cargo build --release\n", StackNode},
	}
	for _, test := range tests {
		if got := DetectStack(parseTestDocument(t, test.input)); got != test.want {
			t.Errorf("got=%q want=%q", got, test.want)
		}
	}
}

func TestDetectStackUsesBaseImagesInStageOrder(t *testing.T) {
	doc := parseTestDocument(t, "FROM python:3.12 AS build\nFROM golang:1.24\n")
	if got := DetectStack(doc); got != StackGo {
		t.Errorf("got=%q want=%q (final stage)", got, StackGo)
	}
}

func TestDetectStageStackPerStage(t *testing.T) {
	doc := parseTestDocument(t, "FROM python:3.12 AS build\nRUN pip install --no-cache-dir flask\nFROM golang:1.24\n")
	if got := DetectStageStack(doc.Stages[0]); got != StackPython {
		t.Errorf("build stage=%q want python", got)
	}
	if got := DetectStageStack(doc.Stages[1]); got != StackGo {
		t.Errorf("final stage=%q want go", got)
	}
}

func TestDetectStackUsesRunsInSourceOrderWhenNoBaseImageMatches(t *testing.T) {
	doc := parseTestDocument(t, "FROM alpine\nRUN rustc main.rs\nRUN go build ./...\n")
	if got := DetectStack(doc); got != StackRust {
		t.Errorf("got=%q want=%q", got, StackRust)
	}
}

func TestStackValidationAndSupport(t *testing.T) {
	if _, err := ParseStack("golnag"); err == nil {
		t.Fatal("expected invalid stack")
	}
	want := map[Stack]bool{
		StackGeneric: false,
		StackGo:      true,
		StackJava:    true,
		StackPython:  true,
		StackNode:    true,
		StackRust:    true,
		StackDotNet:  true,
		StackPHP:     true,
		StackRuby:    true,
		StackCCPP:    true,
	}
	for stack, supported := range want {
		if got := IsSupported(stack); got != supported {
			t.Errorf("IsSupported(%q)=%v, want %v", stack, got, supported)
		}
	}
}

func TestStackRuleIDsComeFromRegisteredRules(t *testing.T) {
	want := map[Stack][]string{
		StackGeneric: {"GEN001", "GEN002", "GEN003", "GEN004", "GEN005", "GEN006", "GEN007", "GEN008", "GEN009", "GEN010"},
		StackGo:      {"GO001", "GO002", "GO003", "GO004"},
		StackJava:    {"JAVA001"},
		StackPython:  {"PY001", "PY002"},
		StackNode:    {"NODE001", "NODE002", "NODE003", "NODE004"},
		StackRust:    {"RUST001", "RUST002", "RUST003", "RUST004"},
		StackDotNet:  {"DOTNET001", "DOTNET002"},
		StackPHP:     {"PHP001", "PHP002"},
		StackRuby:    {"RUBY001"},
		StackCCPP:    {"CCPP001"},
	}
	for stack, ids := range want {
		if got := stackRuleIDs(stack); !reflect.DeepEqual(got, ids) {
			t.Errorf("stackRuleIDs(%q)=%v, want %v", stack, got, ids)
		}
	}
}
