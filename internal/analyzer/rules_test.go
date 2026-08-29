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
		{"CGO disabled via ENV", "FROM golang:1.24 AS build\nENV CGO_ENABLED=0\nRUN go build -o /app\nFROM scratch\n", StackGo, nil, []string{"GO002"}},
		{"CGO disabled via ARG", "FROM golang:1.24 AS build\nARG CGO_ENABLED=0\nRUN go build -o /app\nFROM scratch\n", StackGo, nil, []string{"GO002"}},
		{"CGO still flagged without disable", "FROM golang:1.24 AS build\nENV GOFLAGS=-mod=vendor\nRUN go build -o /app\nFROM scratch\n", StackGo, []string{"GO002"}, nil},
		{"untagged base flagged", "FROM ubuntu\nRUN true\n", StackGeneric, []string{"GEN001"}, nil},
		{"digest pinned base not flagged", "FROM alpine@sha256:0123456789abcdef\n", StackGeneric, nil, []string{"GEN001"}},
		{"stage reference not flagged as latest", "FROM golang:1.24 AS build\nRUN CGO_ENABLED=0 go build\nFROM build\n", StackGo, nil, []string{"GEN001"}},
		{"scratch not flagged as latest", "FROM golang:1.24 AS build\nRUN CGO_ENABLED=0 go build\nFROM scratch\n", StackGo, nil, []string{"GEN001"}},
		{"unrelated Golang substring", "FROM alpine AS build\nFROM acme/notgolang-runtime\n", StackGo, nil, []string{"GO003"}},
		{"PHP flags independent", "FROM php:8.4\nRUN composer install --no-dev\n", StackPHP, []string{"PHP002"}, []string{"PHP001"}},
		{"PHP spaced command", "FROM php:8.4\nRUN composer   install\n", StackPHP, []string{"PHP001", "PHP002"}, nil},
		{"PHP prefixed command ignored", "FROM php:8.4\nRUN notcomposer install\n", StackPHP, nil, []string{"PHP001", "PHP002"}},
		{"Go single stage", "FROM alpine:3.20\n", StackGo, []string{"GO001"}, nil},
		{"Java full runtime", "FROM openjdk:17\n", StackJava, []string{"JAVA001"}, nil},
		{"Java slim runtime", "FROM openjdk:17-slim\n", StackJava, nil, []string{"JAVA001"}},
		{"Java other version still flagged", "FROM openjdk:21\n", StackJava, []string{"JAVA001"}, nil},
		{"Java temurin full flagged", "FROM eclipse-temurin:17\n", StackJava, []string{"JAVA001"}, nil},
		{"Java temurin jre not flagged", "FROM eclipse-temurin:17-jre\n", StackJava, nil, []string{"JAVA001"}},
		{"apt missing no-install-recommends", "FROM debian:bookworm\nRUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*\n", StackGeneric, []string{"GEN002"}, []string{"GEN003"}},
		{"apt missing cache cleanup", "FROM debian:bookworm\nRUN apt-get install -y --no-install-recommends curl\n", StackGeneric, []string{"GEN003"}, []string{"GEN002"}},
		{"apt clean and lean", "FROM debian:bookworm\nRUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*\n", StackGeneric, nil, []string{"GEN002", "GEN003"}},
		{"add remote url flagged", "FROM alpine:3.20\nADD https://example.com/app.tar /opt/app.tar\n", StackGeneric, []string{"GEN004"}, nil},
		{"add local archive not flagged", "FROM alpine:3.20\nCOPY app /app\nADD local.tar /app\n", StackGeneric, nil, []string{"GEN004"}},
		{"final user root flagged", "FROM alpine:3.20\nUSER root\n", StackGeneric, []string{"GEN005"}, nil},
		{"final user nonroot not flagged", "FROM alpine:3.20\nUSER app\n", StackGeneric, nil, []string{"GEN005"}},
		{"missing USER flagged as root", "FROM alpine:3.20\nRUN true\n", StackGeneric, []string{"GEN005"}, nil},
		{"nonroot base image not flagged", "FROM gcr.io/distroless/static:nonroot\nCOPY app /app\n", StackGeneric, nil, []string{"GEN005"}},
		{"root only in build stage not flagged", "FROM golang:1.24 AS build\nUSER root\nRUN CGO_ENABLED=0 go build\nFROM scratch\nCOPY --from=build /app /app\nUSER 65532\n", StackGo, nil, []string{"GEN005"}},
		{"apt-get flags before install", "FROM debian:bookworm\nRUN apt-get -y install curl && rm -rf /var/lib/apt/lists/*\n", StackGeneric, []string{"GEN002"}, []string{"GEN003"}},
		{"apt-get in heredoc body", "FROM debian:bookworm\nRUN <<EOF\napt-get install -y curl\nrm -rf /var/lib/apt/lists/*\nEOF\n", StackGeneric, []string{"GEN002"}, []string{"GEN003"}},
		{"apk add without no-cache", "FROM alpine:3.20\nRUN apk add curl\nUSER app\n", StackGeneric, []string{"GEN006"}, nil},
		{"apk add with no-cache", "FROM alpine:3.20\nRUN apk add --no-cache curl\nUSER app\n", StackGeneric, nil, []string{"GEN006"}},
		{"dnf install without clean", "FROM fedora:41\nRUN dnf install -y curl\nUSER app\n", StackGeneric, []string{"GEN007"}, nil},
		{"dnf install with clean", "FROM fedora:41\nRUN dnf install -y curl && dnf clean all\nUSER app\n", StackGeneric, nil, []string{"GEN007"}},
		{"pip without no-cache-dir", "FROM python:3.12-slim\nRUN pip install flask\nUSER app\n", StackPython, []string{"PY001"}, nil},
		{"pip with no-cache-dir", "FROM python:3.12-slim\nRUN pip install --no-cache-dir flask\nUSER app\n", StackPython, nil, []string{"PY001"}},
		{"npm install not ci", "FROM node:22-alpine\nRUN npm install\nUSER node\n", StackNode, []string{"NODE001"}, nil},
		{"npm ci not flagged", "FROM node:22-alpine\nRUN npm ci\nUSER node\n", StackNode, nil, []string{"NODE001"}},
		{"gcc final image", "FROM gcc:14\nRUN make\nUSER app\n", StackCCPP, []string{"CCPP001"}, nil},
		{"gcc builder with runtime final", "FROM gcc:14 AS build\nRUN make\nFROM alpine:3.20\nCOPY --from=build /app /app\nUSER app\n", StackCCPP, nil, []string{"CCPP001"}},
		{"disable comment suppresses latest", "# dockopt:disable GEN001\nFROM ubuntu:latest\nUSER app\n", StackGeneric, nil, []string{"GEN001"}},
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

func TestGoFindingRangesAndStages(t *testing.T) {
	tests := []struct {
		name  string
		input string
		id    string
		start int
		end   int
		stage int
	}{
		{
			name:  "GO002 logical RUN range",
			input: "FROM golang AS build\nRUN echo prep \\\n && go build -o /app\nFROM scratch\n",
			id:    "GO002",
			start: 2,
			end:   3,
			stage: 0,
		},
		{
			name:  "GO003 actual final FROM",
			input: "FROM golang AS build\nRUN CGO_ENABLED=0 go build\nFROM alpine AS prep\nRUN true\nFROM golang\n",
			id:    "GO003",
			start: 5,
			end:   5,
			stage: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Analyze(parseTestDocument(t, test.input), StackGo)
			for _, finding := range result.Findings {
				if finding.ID != test.id {
					continue
				}
				if finding.Range.StartLine != test.start || finding.Range.EndLine != test.end || finding.Stage == nil || *finding.Stage != test.stage {
					t.Fatalf("finding=%#v, want lines %d-%d stage %d", finding, test.start, test.end, test.stage)
				}
				return
			}
			t.Fatalf("finding %s absent; got %#v", test.id, result.Findings)
		})
	}
}

func TestAnalyzeIgnoresRuleIDsFromArgument(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM ubuntu:latest\nUSER app\n"), StackGeneric, "GEN001")
	for _, finding := range result.Findings {
		if finding.ID == "GEN001" {
			t.Fatalf("GEN001 should be ignored: %#v", result.Findings)
		}
	}
}

func TestAnalyzeGenericRunsOnlyGenericRulesAndIsUnsupported(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM alpine:latest\nUSER app\n"), StackGeneric)
	if result.Supported {
		t.Fatal("generic analysis must not claim stack-specific support")
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "GEN001" {
		t.Fatalf("findings=%#v, want only GEN001", result.Findings)
	}
}

func TestRuleRegistryMetadata(t *testing.T) {
	wantIDs := []string{"CCPP001", "DOTNET001", "GEN001", "GEN002", "GEN003", "GEN004", "GEN005", "GEN006", "GEN007", "GO001", "GO002", "GO003", "JAVA001", "NODE001", "PHP001", "PHP002", "PY001", "RUBY001", "RUST001"}
	wantSeverity := map[string]Severity{
		"GEN001":    SeverityWarn,
		"GEN002":    SeverityWarn,
		"GEN003":    SeverityWarn,
		"GEN004":    SeverityWarn,
		"GEN005":    SeverityWarn,
		"GEN006":    SeverityWarn,
		"GEN007":    SeverityWarn,
		"GO001":     SeverityWarn,
		"GO002":     SeverityError,
		"GO003":     SeverityWarn,
		"JAVA001":   SeverityInfo,
		"RUST001":   SeverityWarn,
		"DOTNET001": SeverityWarn,
		"PHP001":    SeverityWarn,
		"PHP002":    SeverityWarn,
		"RUBY001":   SeverityInfo,
		"PY001":     SeverityWarn,
		"NODE001":   SeverityWarn,
		"CCPP001":   SeverityWarn,
	}

	gotIDs := make([]string, 0, len(registeredRules))
	seen := make(map[string]bool, len(registeredRules))
	for _, r := range registeredRules {
		if seen[r.id] {
			t.Errorf("duplicate rule ID %q", r.id)
		}
		seen[r.id] = true
		gotIDs = append(gotIDs, r.id)
		if r.severity != wantSeverity[r.id] {
			t.Errorf("rule %s severity=%q want=%q", r.id, r.severity, wantSeverity[r.id])
		}
		if len(r.stacks) == 0 || r.check == nil {
			t.Errorf("rule %s missing stacks or check", r.id)
		}
	}
	sort.Strings(gotIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("rule IDs=%v want=%v", gotIDs, wantIDs)
	}
}
