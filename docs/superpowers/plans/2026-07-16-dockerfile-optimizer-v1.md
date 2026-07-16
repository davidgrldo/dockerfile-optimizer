# Dockerfile Optimizer v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace raw-line scanning with a dependency-free internal Dockerfile parser, typed production rules, a breaking stable v1 CLI/JSON contract, enforceable exits, and a verified delivery pipeline.

**Architecture:** Parse bytes into `Document`, `Stage`, and `Instruction` values carrying source ranges. Detect stack and execute immutable rules against that model, render typed results through human or JSON writers, then map the configured severity threshold to process exit codes.

**Tech Stack:** Go 1.24 standard library, Go `testing` and fuzzing, GitHub Actions.

## Global Constraints

- Add no parser dependency; `internal/dockerfile` uses the Go standard library.
- Remove `github.com/spf13/pflag`; v1 has no third-party runtime dependency.
- Breaking changes to old flags, JSON names, and exit behavior are allowed.
- CLI: `dockopt [--json] [--stack <name>] [--fail-on none|warn|error] <Dockerfile>`.
- Exit codes: `0` below threshold, `1` threshold met, `2` usage/input/parse/output failure.
- JSON uses `schema_version: "1"`; `findings` is always an array.
- Python, Node, and C/C++ remain detectable but explicitly unsupported for stack rules.
- Auto-fix, UI/TUI, recursive scanning, SARIF, new rule packs, and full shell semantics are excluded.
- Each task uses red-green-refactor and ends in one reviewable commit.

## File Map

- Create `internal/dockerfile/{model.go,parser.go,parser_test.go}` for syntax and source modeling.
- Replace `internal/analyzer/{analyzer.go,stacks.go,rules.go}` and tests for typed analysis.
- Replace `internal/report/{report.go,report_test.go}` for v1 rendering.
- Create `cmd/dockopt/{run.go,run_test.go}`; reduce `main.go` to process wiring.
- Delete `internal/parser/parser.go`, remove `pflag`, and delete the resulting empty `go.sum`.
- Create `testdata/dockerfiles/*` for CLI fixtures.
- Replace CI and README; create Dependabot config, ignore file, license, and changelog.

---

### Task 1: Internal model and core parser

**Files:**
- Create: `internal/dockerfile/model.go`
- Create: `internal/dockerfile/parser.go`
- Create: `internal/dockerfile/parser_test.go`

**Interfaces:**
- Consumes: `io.Reader`.
- Produces: `Parse(name string, r io.Reader) (*Document, error)` and the exact model below.

- [ ] **Step 1: Write failing core tests**

Create tests covering comments, mixed-case opcodes, leading whitespace, `FROM --platform ... AS ...`, three stages, and malformed `FROM`:

```go
func TestParseCoreSyntax(t *testing.T) {
	input := "  # FROM rust:latest\n  from --platform=linux/amd64 golang:1.24 AS build\n  run go build ./...\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if len(doc.Instructions) != 2 || doc.Instructions[0].Opcode != "FROM" { t.Fatalf("instructions=%#v", doc.Instructions) }
	stage := doc.Stages[0]
	if stage.BaseImage != "golang:1.24" || stage.Name != "build" || stage.Platform != "linux/amd64" { t.Fatalf("stage=%#v", stage) }
	if stage.From.Range != (Range{StartLine: 2, EndLine: 2}) { t.Fatalf("range=%#v", stage.From.Range) }
}

func TestParseActualFinalStage(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM golang AS build\nRUN go build\nFROM alpine AS prep\nRUN true\nFROM scratch\n"))
	if err != nil { t.Fatal(err) }
	if len(doc.Stages) != 3 || doc.Stages[2].BaseImage != "scratch" || doc.Stages[2].Index != 2 { t.Fatalf("stages=%#v", doc.Stages) }
}

func TestParseMalformedFrom(t *testing.T) {
	_, err := Parse("Dockerfile", strings.NewReader("FROM --platform=linux/amd64\n"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Line != 1 { t.Fatalf("error=%#v", err) }
}
```

Run: `go test ./internal/dockerfile`

Expected: FAIL because the package does not exist.

- [ ] **Step 2: Add the model**

Create `model.go`:

```go
package dockerfile

import "fmt"

type Range struct { StartLine, EndLine int }
type Instruction struct { Opcode, Original, Value string; JSON bool; Range Range }
type Stage struct { Index int; Name, BaseImage, Platform string; From Instruction; Instructions []Instruction }
type Document struct { Name string; EscapeToken rune; Instructions []Instruction; Stages []Stage }
type ParseError struct { Source string; Line int; Message string }
func (e *ParseError) Error() string { return fmt.Sprintf("%s:%d: %s", e.Source, e.Line, e.Message) }
```

- [ ] **Step 3: Implement core parsing**

In `parser.go`, implement these complete responsibilities:

```go
func Parse(name string, r io.Reader) (*Document, error)
func parseInstruction(source, text string, start, end int) (Instruction, error)
func parseFrom(value string) (base, name, platform string, err error)
func parseEscapeDirective(line string) (rune, bool)
func stripContinuation(line string, escape rune) (continued bool, text string)
func (d *Document) addInstruction(instruction Instruction)
```

`Parse` must use `bufio.Scanner` with a 4 MiB maximum token, default escape `\\`, uppercase opcodes, ignored blank/comment lines, logical line ranges, and non-nil empty slices. `parseInstruction` validates JSON-array forms with `encoding/json`. `parseFrom` skips flags, extracts `--platform`, base image, and optional `AS` name. It returns `ParseError` with the logical start line for missing/invalid values.

Use this continuation rule exactly: trim trailing Unicode whitespace, count trailing escape runes, and continue only when the count is odd; remove one escape rune before joining with one space.

- [ ] **Step 4: Run, format, and commit**

```bash
gofmt -w internal/dockerfile
go test ./internal/dockerfile
go test ./...
git add internal/dockerfile
git commit -m "feat: add internal Dockerfile parser"
```

Expected: PASS.

---

### Task 2: Advanced syntax and fuzz safety

**Files:**
- Modify: `internal/dockerfile/parser.go`
- Modify: `internal/dockerfile/parser_test.go`

**Interfaces:**
- Consumes: Task 1 parser.
- Produces: continuation ranges, custom escapes, JSON validation, heredoc isolation, and fuzz safety.

- [ ] **Step 1: Add failing advanced tests**

```go
func TestParseContinuationRange(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM golang AS build\nRUN echo prep \\\n && go build -o /app\n"))
	if err != nil { t.Fatal(err) }
	run := doc.Stages[0].Instructions[0]
	if run.Value != "echo prep && go build -o /app" || run.Range != (Range{StartLine: 2, EndLine: 3}) { t.Fatalf("run=%#v", run) }
}

func TestParseCustomEscapeAndJSON(t *testing.T) {
	input := "# escape=`\nFROM windows/servercore:ltsc2022`\n AS runtime\nCMD [\"cmd\", \"/C\", \"echo ok\"]\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if doc.EscapeToken != '`' || doc.Stages[0].Name != "runtime" || !doc.Stages[0].Instructions[0].JSON { t.Fatalf("doc=%#v", doc) }
}

func TestParseHeredocIsolation(t *testing.T) {
	input := "FROM alpine\nRUN <<EOF\nFROM rust:latest\ngo build ./...\nEOF\nRUN echo done\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil { t.Fatal(err) }
	if len(doc.Instructions) != 3 { t.Fatalf("instructions=%d", len(doc.Instructions)) }
	if got := doc.Stages[0].Instructions[0].Range; got != (Range{StartLine: 2, EndLine: 5}) { t.Fatalf("range=%#v", got) }
}

func TestParseUnterminatedHeredoc(t *testing.T) {
	_, err := Parse("Dockerfile", strings.NewReader("FROM alpine\nRUN <<EOF\necho hi\n"))
	if err == nil || !strings.Contains(err.Error(), "unterminated heredoc EOF") { t.Fatalf("error=%v", err) }
}

func FuzzParse(f *testing.F) {
	f.Add("FROM alpine\nRUN echo ok\n")
	f.Fuzz(func(t *testing.T, input string) {
		doc, err := Parse("fuzz", strings.NewReader(input)); if err != nil { return }
		for _, item := range doc.Instructions { if item.Range.StartLine < 1 || item.Range.EndLine < item.Range.StartLine { t.Fatalf("range=%#v", item.Range) } }
	})
}
```

Run: `go test ./internal/dockerfile`

Expected: heredoc tests FAIL before body consumption exists.

- [ ] **Step 2: Implement heredoc consumption**

Scan physical lines into `[]physicalLine{number,text}` before logical parsing. After parsing a logical instruction, extract each whitespace-delimited token beginning `<<`, strip optional `-` and quotes, then advance through physical lines until the delimiter. Extend `Range.EndLine` through the delimiter but do not append body text to `Instruction.Value`. Missing delimiters return `ParseError{Message: "unterminated heredoc <delimiter>"}`.

Add:

```go
type physicalLine struct { number int; text string }
func heredocDelimiters(value string) []string
```

- [ ] **Step 3: Verify unit and fuzz behavior, then commit**

```bash
gofmt -w internal/dockerfile
go test ./internal/dockerfile
go test ./internal/dockerfile -run=^$ -fuzz=FuzzParse -fuzztime=3s
go test ./...
git add internal/dockerfile
git commit -m "feat: support advanced Dockerfile syntax"
```

Expected: both test commands PASS without panics.

---

### Task 3: Typed analyzer and stack registry

**Files:**
- Replace: `internal/analyzer/analyzer.go`
- Replace: `internal/analyzer/stacks.go`
- Replace: `internal/analyzer/rules.go`
- Replace: `internal/analyzer/rules_test.go`
- Replace: `internal/analyzer/stacks_test.go`
- Modify: `cmd/dockopt/main.go`

**Interfaces:**
- Consumes: `*dockerfile.Document`.
- Produces: `Stack`, `Severity`, `Finding`, `Result`, `Rule`, `DetectStack`, `ParseStack`, `IsSupported`, and `Analyze`.

- [ ] **Step 1: Write failing stack tests**

```go
func TestDetectStackUsesParsedSyntax(t *testing.T) {
	tests := []struct { input string; want Stack }{
		{"# Rust is not used\nFROM alpine\n", StackGeneric},
		{"FROM golang:1.24\nRUN go build ./...\n", StackGo},
		{"FROM python:3.12-slim\n", StackPython},
	}
	for _, test := range tests { if got := DetectStack(parseTestDocument(t, test.input)); got != test.want { t.Errorf("got=%q want=%q", got, test.want) } }
}

func TestStackValidationAndSupport(t *testing.T) {
	if _, err := ParseStack("golnag"); err == nil { t.Fatal("expected invalid stack") }
	if IsSupported(StackPython) || !IsSupported(StackGo) { t.Fatal("support registry mismatch") }
}
```

Run: `go test ./internal/analyzer -run 'TestDetectStack|TestStackValidation'`

Expected: FAIL because typed APIs do not exist.

- [ ] **Step 2: Implement analyzer contracts**

Replace `analyzer.go` with:

```go
type Severity string
const ( SeverityInfo Severity = "info"; SeverityWarn Severity = "warn"; SeverityError Severity = "error" )
type Finding struct { ID string; Severity Severity; Message string; Range dockerfile.Range; Stage *int }
type Result struct { Source string; DetectedStack, SelectedStack Stack; Supported bool; Findings []Finding }
type Context struct { Stack Stack }
type Rule interface { ID() string; Severity() Severity; Stacks() []Stack; Evaluate(*dockerfile.Document, Context) []Finding }
func Analyze(doc *dockerfile.Document, selected Stack) Result
```

`Analyze` iterates `Rules()`, runs generic plus selected-stack rules, returns findings as a non-nil slice, and sets source, detected/selected stacks, and support status.

Retain this adapter type only until Task 5 migrates the reporter call:

```go
type Suggestion struct { Description string; Severity string }
```

- [ ] **Step 3: Implement one authoritative stack registry**

Define constants for `generic`, `go`, `java`, `python`, `node`, `rust`, `dotnet`, `php`, `ruby`, and `c_cpp`. The registry stores detection keywords and support state. `ParseStack` accepts only registry names. `DetectStack` examines parsed base images and `RUN.Value` only. Mark Go/Java/Rust/.NET/PHP/Ruby supported and Python/Node/C++ unsupported.

Replace `rules.go` with an empty but typed registry so the repository compiles before Task 4:

```go
package analyzer
var registeredRules []Rule
func Rules() []Rule { return append([]Rule(nil), registeredRules...) }
```

Replace `rules_test.go` with `TestAnalyzeInitializesFindings`, asserting `Analyze(...).Findings != nil`.

Update `cmd/dockopt/main.go` to open the file and call `dockerfile.Parse`, `DetectStack`, and `Analyze`. Convert findings to `[]analyzer.Suggestion` before calling the old report functions. Keep `pflag`, existing flags, and existing printing behavior in this bridge. Keep `internal/parser` until Task 6.

- [ ] **Step 4: Run and commit**

```bash
gofmt -w internal/analyzer
gofmt -w cmd/dockopt
go test ./...
git add internal/analyzer cmd/dockopt
git commit -m "refactor: add typed analyzer and stack registry"
```

Expected: PASS using an empty `registeredRules` slice until Task 4 replaces it.

---

### Task 4: Correct production rules and test the real registry

**Files:**
- Replace: `internal/analyzer/rules.go`
- Replace: `internal/analyzer/rules_test.go`
- Modify: `internal/analyzer/analyzer.go`

**Interfaces:**
- Consumes: Task 3 contracts.
- Produces stable rule IDs: `GEN001`, `GO001`–`GO003`, `JAVA001`, `RUST001`, `DOTNET001`, `PHP001`–`PHP002`, `RUBY001`.

- [ ] **Step 1: Write failing production-registry regressions**

Use one table that asserts present and absent IDs for:

```go
{"comment ignored", "# FROM ubuntu:latest\nFROM alpine:3.20\n", StackGeneric, nil, []string{"GEN001"}}
{"lowercase latest", "from ubuntu:latest\n", StackGeneric, []string{"GEN001"}, nil}
{"multiline static build", "FROM golang AS build\nRUN echo prep \\\n && go build -o /app\nFROM scratch\n", StackGo, []string{"GO002"}, nil}
{"third final stage", "FROM golang AS build\nRUN CGO_ENABLED=0 go build\nFROM alpine AS prep\nRUN true\nFROM golang\n", StackGo, []string{"GO003"}, nil}
{"normal CGO runtime", "FROM golang AS build\nRUN go build -o /app\nFROM debian:bookworm-slim\n", StackGo, nil, []string{"GO002"}}
{"PHP flags independent", "FROM php:8.4\nRUN composer install --no-dev\n", StackPHP, []string{"PHP002"}, []string{"PHP001"}}
```

Also assert every `Rules()` ID is unique. Run `go test ./internal/analyzer`; expect FAIL.

- [ ] **Step 2: Implement immutable rule metadata**

Create a private `rule` type implementing `Rule`, with copied stack slices and a check function. `Rules()` returns a copy of the registry. Remove Task 3's empty `registeredRules` implementation when adding the authoritative registry.

- [ ] **Step 3: Implement corrected rules**

- `GEN001` warns only parsed `FROM ...:latest` stages.
- `GO001` warns when a Go document has fewer than two stages.
- `GO002` errors only when final image is `scratch` and a builder `RUN` contains `go build` without `CGO_ENABLED=0`.
- `GO003` warns when the actual last stage uses a Golang base image.
- `JAVA001` preserves slim-runtime guidance using parsed base images.
- `RUST001` checks parsed stage count.
- `DOTNET001` warns on an untagged `mcr.microsoft.com/dotnet/*` base; generic latest remains `GEN001`.
- `PHP001` and `PHP002` independently check exact command tokens `--no-dev` and `--optimize-autoloader`.
- `RUBY001` preserves deployment-mode guidance using parsed `RUN` values.

Every finding uses its instruction/stage range and stage index.

- [ ] **Step 4: Run and commit**

```bash
gofmt -w internal/analyzer
go test ./...
git add internal/analyzer
git commit -m "fix: migrate production rules to parsed instructions"
```

Expected: all production regression cases PASS.

---

### Task 5: Versioned reporters

**Files:**
- Replace: `internal/report/report.go`
- Replace: `internal/report/report_test.go`
- Modify: `cmd/dockopt/main.go`
- Modify: `internal/analyzer/analyzer.go`

**Interfaces:**
- Consumes: `analyzer.Result` and `io.Writer`.
- Produces: `WriteJSON`, `WriteHuman`, `WriteErrorJSON`; all return errors.

- [ ] **Step 1: Write failing contract tests**

Test decoded JSON rather than substrings:

```go
func TestWriteJSONV1EmptyArray(t *testing.T) {
	result := analyzer.Result{Source: "Dockerfile", DetectedStack: analyzer.StackPython, SelectedStack: analyzer.StackPython, Findings: []analyzer.Finding{}}
	var buffer bytes.Buffer
	if err := WriteJSON(&buffer, result); err != nil { t.Fatal(err) }
	var output Output
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil { t.Fatal(err) }
	if output.SchemaVersion != "1" || output.Findings == nil { t.Fatalf("output=%#v", output) }
}
```

Also test that unsupported human output contains `generic checks only` and not `No issues found`, and a writer returning `broken pipe` is propagated. Run `go test ./internal/report`; expect FAIL.

- [ ] **Step 2: Implement v1 DTOs**

Define explicit JSON-tagged `Output`, `StackOutput`, `FindingOutput`, `Summary`, `ErrorOutput`, and `ErrorDetail`. Required fields are:

```go
type Output struct { SchemaVersion string `json:"schema_version"`; Source string `json:"source"`; Stack StackOutput `json:"stack"`; Findings []FindingOutput `json:"findings"`; Summary Summary `json:"summary"` }
type FindingOutput struct { ID string `json:"id"`; Severity analyzer.Severity `json:"severity"`; Message string `json:"message"`; Line int `json:"line"`; EndLine int `json:"end_line"`; Stage *int `json:"stage"` }
```

`NewOutput` initializes `Findings` with `[]FindingOutput{}` and counts all severities.

- [ ] **Step 3: Implement writers and commit**

`WriteJSON` and `WriteErrorJSON` use `json.NewEncoder(w).Encode`. `WriteHuman` prints detected stack, support disclaimer, then `[severity] ID (line/range): message`. It reports `No issues found` only when the selected stack is supported. No package-level output writer remains.

Update the bridge in `cmd/dockopt/main.go` to pass `analyzer.Result` directly to `report.WriteJSON(os.Stdout, result)` or `report.WriteHuman(os.Stdout, result)` and fail on returned write errors. Remove `Suggestion` from `analyzer.go`; it has no remaining caller. Keep the old CLI flags and success exit behavior until Task 6.

```bash
gofmt -w internal/report internal/analyzer cmd/dockopt
go test ./...
git add internal/report internal/analyzer cmd/dockopt
git commit -m "feat: define v1 report contract"
```

Expected: PASS, including broken-writer propagation.

---

### Task 6: v1 CLI and exit semantics

**Files:**
- Create: `cmd/dockopt/run.go`
- Create: `cmd/dockopt/run_test.go`
- Replace: `cmd/dockopt/main.go`
- Create: `testdata/dockerfiles/{clean,warn-latest,error-go-static,unsupported-python,malformed}.Dockerfile`
- Delete: `internal/parser/parser.go`
- Modify: `go.mod`
- Delete: `go.sum`

**Interfaces:**
- Consumes: Tasks 1–5.
- Produces: `run(args []string, stdout, stderr io.Writer) int`.

- [ ] **Step 1: Add exact fixtures**

```dockerfile
# clean.Dockerfile
FROM golang:1.24 AS build
RUN CGO_ENABLED=0 go build -o /app
FROM scratch
COPY --from=build /app /app
```

```dockerfile
# warn-latest.Dockerfile
FROM alpine:latest
```

```dockerfile
# error-go-static.Dockerfile
FROM golang:1.24 AS build
RUN go build -o /app
FROM scratch
COPY --from=build /app /app
```

```dockerfile
# unsupported-python.Dockerfile
FROM python:3.12-slim
RUN pip install flask
```

```dockerfile
# malformed.Dockerfile
FROM --platform=linux/amd64
```

- [ ] **Step 2: Write failing CLI tests**

Test `run` with buffers and fixture paths:

- Clean JSON exits `0`, decodes schema `1`, and has non-nil empty findings.
- Warning exits `0` by default and `1` with `--fail-on warn`.
- Static Go error exits `1` by default.
- `--stack golnag` exits `2` with `unknown stack` on stderr.
- Unsupported Python exits `0`, says `generic checks only`, and never says `No issues found`.
- Malformed Dockerfile in JSON mode exits `2` with error kind `parse_error`.
- Missing file exits `2` with an input diagnostic.

Run `go test ./cmd/dockopt`; expect FAIL because `run` is undefined.

- [ ] **Step 3: Implement CLI orchestration**

Use `flag.NewFlagSet(..., flag.ContinueOnError)` with standard-library flags `json`, `stack`, and `fail-on` (default `error`). Require exactly one positional path. Parse/validate failure thresholds and stack overrides before analysis. Open the file, call `dockerfile.Parse`, select detected or overridden stack, call `analyzer.Analyze`, render, then evaluate findings against the threshold.

Implement helpers with these exact signatures:

```go
func run(args []string, stdout, stderr io.Writer) int
func requestsJSON(args []string) bool
func writeFailure(stdout, stderr io.Writer, jsonMode bool, kind string, err error) int
func meetsThreshold(findings []analyzer.Finding, threshold string) bool
```

`writeFailure` emits the v1 JSON error envelope when JSON was requested, otherwise stderr. Reporter write failure always exits `2` and attempts a stderr diagnostic. `none` never fails, `warn` matches warn/error, and `error` matches error only.

Replace `main.go` with:

```go
package main
import "os"
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
```

- [ ] **Step 4: Remove legacy parser and pflag**

Delete `internal/parser/parser.go`. Remove the `require` block from `go.mod`, run `go mod tidy`, and confirm `go.sum` is deleted.

- [ ] **Step 5: Verify and commit**

```bash
gofmt -w cmd internal
go test ./...
go build -o /tmp/dockopt ./cmd/dockopt
/tmp/dockopt --json testdata/dockerfiles/clean.Dockerfile
git add cmd internal testdata go.mod go.sum
git commit -m "feat: ship clean v1 CLI contract"
```

Expected: tests/build PASS and clean JSON contains `"findings": []`.

---

### Task 7: CI, documentation, and release hygiene

**Files:**
- Replace: `.github/workflows/lint-dockerfile.yml`
- Create: `.github/dependabot.yml`, `.gitignore`, `LICENSE`, `CHANGELOG.md`
- Replace: `README.md`
- Format: all Go files

**Interfaces:**
- Consumes: completed v1 binary and fixtures.
- Produces: immutable CI, accurate docs, and release-ready repository metadata.

- [ ] **Step 1: Replace CI**

Use checkout SHA `34e114876b0b11c390a56381ad16ebd13914f8d5` (`v4`) and setup-go SHA `924ae3a1cded613372ab5595356fb5720e22ba16` (`v6`). Set `permissions: contents: read`; trigger on PRs and changes to Go, `go.mod`, workflows, or fixtures. Run:

```yaml
- run: test -z "$(gofmt -l .)"
- run: go vet ./...
- run: go test ./...
- run: go test -race ./...
- run: go build -o dockopt ./cmd/dockopt
```

Then execute clean and unsupported fixtures. With `set +e`, capture error and malformed fixture statuses, restore `set -e`, and assert `1` and `2`. Never reference a root `./Dockerfile`.

- [ ] **Step 2: Add dependency automation and metadata**

`.github/dependabot.yml` schedules monthly `gomod` and `github-actions` updates. `.gitignore` contains `.vibe-scan/`, `dockopt`, and `coverage.out`. Add canonical MIT text with `Copyright (c) 2026 David Grldo`.

`CHANGELOG.md` adds `[1.0.0] - Unreleased`, documenting the internal parser, stable IDs/ranges, JSON schema `1`, `--fail-on`, stack validation, removal of `pflag`, fixed multiline/stage/CGO/PHP behavior, and breaking output/exit changes.

- [ ] **Step 3: Replace README accurately**

README must include installation, exact CLI syntax, default threshold, exit codes, JSON schema, and development commands. State that Go/Java/Rust/.NET/PHP/Ruby have stack rules; Python/Node/C++ get generic checks only. State dependency-free only after `go list -m all` confirms it. Remove completed roadmap items and the stray generated closing block.

- [ ] **Step 4: Run final verification**

```bash
gofmt -w .
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go test -race ./...
go test ./internal/dockerfile -run=^$ -fuzz=FuzzParse -fuzztime=3s
go build -o /tmp/dockopt ./cmd/dockopt
go list -m all
git grep -n 'github.com/spf13/pflag\|internal/parser' -- '*.go' go.mod || true
git grep -n 'actions/checkout@v[0-9]\|actions/setup-go@v[0-9]' -- '.github/workflows/*' || true
```

Expected: all verification passes; module listing contains only this module; both greps have no matches.

- [ ] **Step 5: Reproduce exit contracts**

```bash
/tmp/dockopt --json testdata/dockerfiles/clean.Dockerfile
set +e
/tmp/dockopt --json testdata/dockerfiles/error-go-static.Dockerfile >/tmp/error.json
error_status=$?
/tmp/dockopt --json testdata/dockerfiles/malformed.Dockerfile >/tmp/parse.json
parse_status=$?
set -e
test "$error_status" = "1"
test "$parse_status" = "2"
```

Expected: PASS; clean JSON contains `"findings": []`.

- [ ] **Step 6: Commit**

```bash
git add .github .gitignore LICENSE CHANGELOG.md README.md cmd internal testdata go.mod
git commit -m "chore: complete Dockerfile optimizer v1 delivery"
```

---

## Final Review Checklist

- [ ] Match every acceptance criterion in `docs/superpowers/specs/2026-07-16-dockerfile-optimizer-v1-design.md` to a test or verification command above.
- [ ] Re-run all audit reproductions: comment latest, lowercase `FROM`, multiline Go build, third-stage final Golang, normal CGO runtime, invalid override, JSON empty array, PHP flags, and the old missing-root-Dockerfile workflow case.
- [ ] Confirm no import of `internal/parser` or `pflag` remains.
- [ ] Confirm all GitHub Action references use 40-character SHAs.
- [ ] Confirm `.vibe-scan/` is ignored and not staged.
- [ ] Confirm seven focused implementation commits and run `git diff 21a99cf..HEAD --check`.
