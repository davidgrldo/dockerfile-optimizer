package analyzer

import (
	"reflect"
	"sort"
	"strings"
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
		{"Go single stage without compile is not GO001", "FROM alpine:3.20\nUSER app\n", StackGo, nil, []string{"GO001"}},
		{"Go single stage with compile", "FROM golang:1.24\nRUN go build -o /app\nUSER app\n", StackGo, []string{"GO001"}, nil},
		{"Java full runtime", "FROM openjdk:17\n", StackJava, []string{"JAVA001"}, nil},
		{"Java slim runtime", "FROM openjdk:17-slim\n", StackJava, nil, []string{"JAVA001"}},
		{"Java other version still flagged", "FROM openjdk:21\n", StackJava, []string{"JAVA001"}, nil},
		{"Java temurin full flagged", "FROM eclipse-temurin:17\n", StackJava, []string{"JAVA001"}, nil},
		{"Java temurin jre not flagged", "FROM eclipse-temurin:17-jre\n", StackJava, nil, []string{"JAVA001"}},
		{"Java builder JDK not flagged", "FROM eclipse-temurin:17 AS build\nRUN ./mvnw package\nFROM eclipse-temurin:17-jre\nUSER app\n", StackJava, nil, []string{"JAVA001"}},
		{"pip copy whole context first", "FROM python:3.12-slim\nCOPY . /app\nRUN pip install --no-cache-dir -r requirements.txt\nUSER app\n", StackPython, []string{"PY002"}, nil},
		{"pip copy requirements first", "FROM python:3.12-slim\nCOPY requirements.txt /app/\nRUN pip install --no-cache-dir -r requirements.txt\nCOPY . /app\nUSER app\n", StackPython, nil, []string{"PY002"}},
		{"npm copy whole context first", "FROM node:22-alpine\nCOPY . /app\nRUN npm ci\nUSER node\n", StackNode, []string{"NODE002"}, nil},
		{"npm copy package.json first", "FROM node:22-alpine\nCOPY package.json package-lock.json /app/\nRUN npm ci\nCOPY . /app\nUSER node\n", StackNode, nil, []string{"NODE002"}},
		{"copy from stage is not build context", "FROM node:22-alpine AS build\nRUN npm ci\nFROM node:22-alpine\nCOPY --from=build . /app\nRUN npm ci\nUSER node\n", StackNode, nil, []string{"NODE002"}},
		{"secret ARG flagged", "FROM alpine:3.20\nARG GITHUB_TOKEN\nUSER app\n", StackGeneric, []string{"GEN008"}, nil},
		{"plain ARG not flagged", "FROM alpine:3.20\nARG VERSION=1\nUSER app\n", StackGeneric, nil, []string{"GEN008"}},
		{"secret substring is not a secret name", "FROM alpine:3.20\nARG NONSECRET=1\nENV SECRETARY=app\nUSER app\n", StackGeneric, nil, []string{"GEN008"}},
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
		{"Rust single stage without compile is not RUST001", "FROM alpine:3.20\nUSER app\n", StackRust, nil, []string{"RUST001"}},
		{"Rust single stage with cargo build", "FROM rust:1.88\nRUN cargo build --release\nUSER app\n", StackRust, []string{"RUST001"}, nil},
		{"go.mod copied after source", "FROM golang:1.24 AS build\nCOPY . /src\nRUN go mod download\nUSER app\n", StackGo, []string{"GO004"}, nil},
		{"go.mod copied before source", "FROM golang:1.24 AS build\nCOPY go.mod go.sum /src/\nRUN go mod download\nCOPY . /src\nUSER app\n", StackGo, nil, []string{"GO004"}},
		{"Cargo.toml copied after source", "FROM rust:1.88 AS build\nCOPY . /src\nRUN cargo fetch\nUSER app\n", StackRust, []string{"RUST002"}, nil},
		{"Cargo.toml copied before source", "FROM rust:1.88 AS build\nCOPY Cargo.toml Cargo.lock /src/\nRUN cargo fetch\nCOPY . /src\nUSER app\n", StackRust, nil, []string{"RUST002"}},
		{"go build without cache mount", "FROM golang:1.24 AS build\nRUN go build -o /app\nUSER app\n", StackGo, []string{"GEN009"}, nil},
		{"go build with cache mount", "FROM golang:1.24 AS build\nRUN --mount=type=cache,target=/go/pkg/mod go build -o /app\nUSER app\n", StackGo, nil, []string{"GEN009"}},
		{"yarn install without frozen lockfile", "FROM node:22-alpine\nRUN yarn install\nUSER node\n", StackNode, []string{"NODE003"}, nil},
		{"yarn install with immutable", "FROM node:22-alpine\nRUN yarn install --immutable\nUSER node\n", StackNode, nil, []string{"NODE003"}},
		{"pnpm install without frozen lockfile", "FROM node:22-alpine\nRUN pnpm install\nUSER node\n", StackNode, []string{"NODE003"}, nil},
		{"node install without NODE_ENV", "FROM node:22-alpine\nRUN npm ci\nUSER node\n", StackNode, []string{"NODE004"}, nil},
		{"node install with NODE_ENV", "FROM node:22-alpine\nENV NODE_ENV=production\nRUN npm ci\nUSER node\n", StackNode, nil, []string{"NODE004"}},
		{"dotnet sdk as final", "FROM mcr.microsoft.com/dotnet/sdk:8.0\nUSER app\n", StackDotNet, []string{"DOTNET002"}, nil},
		{"dotnet aspnet as final", "FROM mcr.microsoft.com/dotnet/aspnet:8.0\nUSER app\n", StackDotNet, nil, []string{"DOTNET002"}},
		{"cargo build without release", "FROM rust:1.88-alpine AS build\nRUN cargo build\nUSER app\n", StackRust, []string{"RUST003"}, nil},
		{"cargo build with release", "FROM rust:1.88-alpine AS build\nRUN cargo build --release\nUSER app\n", StackRust, nil, []string{"RUST003"}},
		{"rust full image as final", "FROM rust:1.88\nRUN cargo build --release\nUSER app\n", StackRust, []string{"RUST004"}, nil},
		{"rust alpine as final", "FROM rust:1.88-alpine\nRUN cargo build --release\nUSER app\n", StackRust, nil, []string{"RUST004"}},
		{"tagged image without digest", "FROM alpine:3.20\nUSER app\n", StackGeneric, []string{"GEN010"}, nil},
		{"digest pinned image", "FROM alpine:3.20@sha256:0123456789abcdef\nUSER app\n", StackGeneric, nil, []string{"GEN010"}},
		{"ARG-substituted FROM skips digest pin", "ARG VER=3.20\nFROM alpine:${VER}\nUSER app\n", StackGeneric, nil, []string{"GEN010"}},
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

func TestAnalyzeRunsRulesForEveryStageStack(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM python:3.12-slim AS build\nRUN pip install flask\nFROM golang:1.24\nUSER app\n"), "")
	ids := map[string]bool{}
	for _, finding := range result.Findings {
		ids[finding.ID] = true
	}
	if result.DetectedStack != StackGo {
		t.Fatalf("detected=%q want go", result.DetectedStack)
	}
	if !ids["PY001"] {
		t.Fatalf("expected PY001 on python builder; got %#v", result.Findings)
	}
	if !ids["GO003"] {
		t.Fatalf("expected GO003 on golang final; got %#v", result.Findings)
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

func TestFindingsIncludeSuggestedFixes(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM debian:bookworm\nRUN apt-get install curl\nUSER app\n"), StackGeneric)
	for _, finding := range result.Findings {
		if finding.ID == "GEN002" {
			if finding.SuggestedFix == "" {
				t.Fatal("GEN002 must include a suggested fix")
			}
			return
		}
	}
	t.Fatalf("GEN002 absent: %#v", result.Findings)
}

func TestAnalyzeGenericRunsOnlyGenericRulesAndIsUnsupported(t *testing.T) {
	result := Analyze(parseTestDocument(t, "FROM alpine:latest\nUSER app\n"), StackGeneric)
	if result.Supported {
		t.Fatal("generic analysis must not claim stack-specific support")
	}
	ids := map[string]bool{}
	for _, finding := range result.Findings {
		if !strings.HasPrefix(finding.ID, "GEN") {
			t.Fatalf("generic analysis emitted stack rule %s: %#v", finding.ID, result.Findings)
		}
		ids[finding.ID] = true
	}
	if !ids["GEN001"] {
		t.Fatalf("findings=%#v, want GEN001", result.Findings)
	}
}

func TestRuleRegistryMetadata(t *testing.T) {
	wantIDs := []string{"CCPP001", "DOTNET001", "DOTNET002", "GEN001", "GEN002", "GEN003", "GEN004", "GEN005", "GEN006", "GEN007", "GEN008", "GEN009", "GEN010", "GO001", "GO002", "GO003", "GO004", "JAVA001", "NODE001", "NODE002", "NODE003", "NODE004", "PHP001", "PHP002", "PY001", "PY002", "RUBY001", "RUST001", "RUST002", "RUST003", "RUST004"}
	wantSeverity := map[string]Severity{
		"GEN001":    SeverityWarn,
		"GEN002":    SeverityWarn,
		"GEN003":    SeverityWarn,
		"GEN004":    SeverityWarn,
		"GEN005":    SeverityWarn,
		"GEN006":    SeverityWarn,
		"GEN007":    SeverityWarn,
		"GEN008":    SeverityWarn,
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
		"PY002":     SeverityWarn,
		"NODE001":   SeverityWarn,
		"NODE002":   SeverityWarn,
		"NODE003":   SeverityWarn,
		"NODE004":   SeverityWarn,
		"CCPP001":   SeverityWarn,
		"GO004":     SeverityWarn,
		"GEN009":    SeverityWarn,
		"GEN010":    SeverityInfo,
		"DOTNET002": SeverityWarn,
		"RUST002":   SeverityWarn,
		"RUST003":   SeverityWarn,
		"RUST004":   SeverityWarn,
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
		if suggestedFix(r.id) == "" {
			t.Errorf("rule %s missing suggested fix", r.id)
		}
	}
	sort.Strings(gotIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("rule IDs=%v want=%v", gotIDs, wantIDs)
	}
}
