package analyzer

import (
	"reflect"
	"sort"
	"testing"
)

func TestProductionRuleRegistry(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		stack   Stack
		present []string
		absent  []string
	}{
		{"comment ignored", "# FROM ubuntu:latest\nFROM alpine:3.20\n", StackGeneric, nil, []string{"GEN001"}},
		{"lowercase latest", "from ubuntu:latest\n", StackGeneric, []string{"GEN001"}, nil},
		{"multiline static build", "FROM golang AS build\nRUN echo prep \\\n && go build -o /app\nFROM scratch\n", StackGo, []string{"GO002"}, nil},
		{"third final stage", "FROM golang AS build\nRUN CGO_ENABLED=0 go build\nFROM alpine AS prep\nRUN true\nFROM golang\n", StackGo, []string{"GO003"}, nil},
		{"normal CGO runtime", "FROM golang AS build\nRUN go build -o /app\nFROM debian:bookworm-slim\n", StackGo, nil, []string{"GO002"}},
		{"cargo is not Go build", "FROM alpine AS build\nRUN cargo build\nFROM scratch\n", StackGo, nil, []string{"GO002"}},
		{"punctuated Go build", "FROM golang AS build\nRUN go build; echo done\nFROM scratch\n", StackGo, []string{"GO002"}, nil},
		{"prefixed CGO variable", "FROM golang AS build\nRUN NOT_CGO_ENABLED=0 go build\nFROM scratch\n", StackGo, []string{"GO002"}, nil},
		{"exact CGO assignment", "FROM golang AS build\nRUN CGO_ENABLED=0 go build\nFROM scratch\n", StackGo, nil, []string{"GO002"}},
		{"spaced CGO assignment", "FROM golang AS build\nRUN CGO_ENABLED = 0 go build\nFROM scratch\n", StackGo, nil, []string{"GO002"}},
		{"unrelated Golang substring", "FROM alpine AS build\nFROM acme/notgolang-runtime\n", StackGo, nil, []string{"GO003"}},
		{"PHP flags independent", "FROM php:8.4\nRUN composer install --no-dev\n", StackPHP, []string{"PHP002"}, []string{"PHP001"}},
		{"Go single stage", "FROM alpine:3.20\n", StackGo, []string{"GO001"}, nil},
		{"Java full runtime", "FROM openjdk:17\n", StackJava, []string{"JAVA001"}, nil},
		{"Java slim runtime", "FROM openjdk:17-slim\n", StackJava, nil, []string{"JAVA001"}},
		{"Rust single stage", "FROM rust:1.88\n", StackRust, []string{"RUST001"}, nil},
		{"dotnet untagged", "FROM mcr.microsoft.com/dotnet/runtime\n", StackDotNet, []string{"DOTNET001"}, nil},
		{"dotnet latest handled generically", "FROM mcr.microsoft.com/dotnet/runtime:latest\n", StackDotNet, []string{"GEN001"}, []string{"DOTNET001"}},
		{"PHP both flags missing", "FROM php:8.4\nRUN composer install\n", StackPHP, []string{"PHP001", "PHP002"}, nil},
		{"Ruby deployment mode", "FROM ruby:3.4\nRUN bundle install\n", StackRuby, []string{"RUBY001"}, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Analyze(parseTestDocument(t, test.input), test.stack)
			ids := make(map[string]bool, len(result.Findings))
			for _, finding := range result.Findings {
				ids[finding.ID] = true
				if finding.Range.StartLine == 0 || finding.Range.EndLine < finding.Range.StartLine {
					t.Errorf("finding %s has invalid range: %#v", finding.ID, finding.Range)
				}
				if finding.Stage == nil {
					t.Errorf("finding %s has no stage", finding.ID)
				}
			}
			for _, id := range test.present {
				if !ids[id] {
					t.Errorf("finding %s absent; got %#v", id, result.Findings)
				}
			}
			for _, id := range test.absent {
				if ids[id] {
					t.Errorf("finding %s present; got %#v", id, result.Findings)
				}
			}
		})
	}
}

func TestRuleRegistryMetadata(t *testing.T) {
	wantIDs := []string{"DOTNET001", "GEN001", "GO001", "GO002", "GO003", "JAVA001", "PHP001", "PHP002", "RUBY001", "RUST001"}
	wantSeverity := map[string]Severity{
		"GEN001":    SeverityWarn,
		"GO001":     SeverityWarn,
		"GO002":     SeverityError,
		"GO003":     SeverityWarn,
		"JAVA001":   SeverityInfo,
		"RUST001":   SeverityWarn,
		"DOTNET001": SeverityWarn,
		"PHP001":    SeverityWarn,
		"PHP002":    SeverityWarn,
		"RUBY001":   SeverityInfo,
	}

	rules := Rules()
	gotIDs := make([]string, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if seen[rule.ID()] {
			t.Errorf("duplicate rule ID %q", rule.ID())
		}
		seen[rule.ID()] = true
		gotIDs = append(gotIDs, rule.ID())
		if got := rule.Severity(); got != wantSeverity[rule.ID()] {
			t.Errorf("rule %s severity=%q want=%q", rule.ID(), got, wantSeverity[rule.ID()])
		}
	}
	sort.Strings(gotIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("rule IDs=%v want=%v", gotIDs, wantIDs)
	}
}

func TestRuleRegistryMetadataIsCopied(t *testing.T) {
	rules := Rules()
	if len(rules) == 0 {
		t.Fatal("expected production rules")
	}
	original := rules[0]
	originalID := original.ID()
	rules[0] = nil
	if Rules()[0].ID() != originalID {
		t.Fatal("Rules returned mutable registry storage")
	}

	stacks := original.Stacks()
	if len(stacks) == 0 {
		t.Fatal("expected rule stacks")
	}
	originalStack := stacks[0]
	stacks[0] = StackRuby
	if original.Stacks()[0] != originalStack {
		t.Fatal("Stacks returned mutable rule storage")
	}
}
