package analyzer

import (
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
	if got := DetectStack(doc); got != StackPython {
		t.Errorf("got=%q want=%q", got, StackPython)
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
	if IsSupported(StackPython) || !IsSupported(StackGo) {
		t.Fatal("support registry mismatch")
	}
}
