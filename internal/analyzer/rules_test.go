package analyzer

import (
	"strings"
	"testing"
)

// A minimal set of rules for testing
var testRules = []Rule{
	{
		Description: "Avoid latest tag",
		Stack:       "generic",
		Severity:    "warn",
		Check: func(lines []string) string {
			for _, l := range lines {
				if strings.Contains(l, ":latest") {
					return "Avoid using :latest"
				}
			}
			return ""
		},
	},
	{
		Description: "Multi-stage build for Go",
		Stack:       "go",
		Severity:    "info",
		Check: func(lines []string) string {
			count := 0
			for _, l := range lines {
				if strings.HasPrefix(strings.ToUpper(l), "FROM") {
					count++
				}
			}
			if count < 2 {
				return "Use multi-stage builds"
			}
			return ""
		},
	},
}

func TestRunChecksDetailed(t *testing.T) {
	// Temporarily override the global rules
	oldRules := rules
	rules = testRules
	defer func() { rules = oldRules }()

	// Case 1: Go Dockerfile with latest tag and single stage
	lines1 := []string{
		"FROM golang:latest",
		"RUN go build -o app",
	}
	got := RunChecksDetailed(lines1, "go")
	if len(got) != 2 {
		t.Errorf("Expected 2 suggestions, got %d", len(got))
	}

	// Check descriptions and severities
	found := map[string]string{} // desc -> severity
	for _, s := range got {
		found[s.Description] = s.Severity
	}
	if found["Avoid using :latest"] != "warn" {
		t.Error("Expected severity warn for latest-tag rule")
	}
	if found["Use multi-stage builds"] != "info" {
		t.Error("Expected severity info for multi-stage rule")
	}

	// Case 2: Java Dockerfile (generic rule only)
	lines2 := []string{
		"FROM openjdk:latest",
		"COPY . /app",
	}
	got2 := RunChecksDetailed(lines2, "java")
	if len(got2) != 1 {
		t.Errorf("Expected 1 suggestion for generic rule, got %d", len(got2))
	}
	if got2[0].Description != "Avoid using :latest" {
		t.Errorf("Unexpected suggestion: %s", got2[0].Description)
	}
}

func TestRunChecks(t *testing.T) {
	// Temporarily override global rules
	oldRules := rules
	defer func() { rules = oldRules }()

	// Create test rules
	rules = []Rule{
		{
			Description: "Disallow latest tag",
			Stack:       "generic",
			Check: func(lines []string) string {
				for _, l := range lines {
					if strings.Contains(l, ":latest") {
						return "Avoid using :latest"
					}
				}
				return ""
			},
		},
		{
			Description: "Use multi-stage build for Go",
			Stack:       "go",
			Check: func(lines []string) string {
				count := 0
				for _, l := range lines {
					if strings.HasPrefix(strings.ToUpper(l), "FROM") {
						count++
					}
				}
				if count < 2 {
					return "Use multi-stage builds"
				}
				return ""
			},
		},
	}

	// Run against a Go Dockerfile with one FROM
	lines := []string{
		"FROM golang:latest",
		"RUN go build -o app",
	}

	// Expect both rules to trigger
	findings := RunChecks(lines, "go")

	if len(findings) != 2 {
		t.Errorf("Expected 2 findings, got %d", len(findings))
	}
	if findings[0] != "Avoid using :latest" && findings[1] != "Avoid using :latest" {
		t.Errorf("Missing expected finding for :latest tag")
	}
	if findings[0] != "Use multi-stage builds" && findings[1] != "Use multi-stage builds" {
		t.Errorf("Missing expected finding for multi-stage builds")
	}
}
