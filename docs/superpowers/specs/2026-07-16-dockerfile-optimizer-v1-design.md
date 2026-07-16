# Dockerfile Optimizer v1 Design

**Date:** 2026-07-16

**Status:** Approved design

**Source:** `.vibe-scan/scan-report.md`

**Primary deliverable:** A reliable v1 Dockerfile linter with an internal parser, typed rules, a stable JSON schema, enforceable CI exit codes, regression coverage, and corrected repository delivery metadata.

## Problem

The current tool scans raw physical lines with substring checks. As a result, comments can trigger findings, valid lowercase instructions are missed, multiline instructions evade rules, and multistage logic can inspect the wrong stage. The CLI serializes internal structs directly, accepts invalid stack overrides, and exits successfully even when it emits an error-grade finding. Tests replace the production rule registry, while the GitHub workflow analyzes a root Dockerfile that does not exist.

Version 1 will replace these incidental behaviors with explicit parser, rule, output, and process contracts. Breaking changes to the current CLI and JSON output are allowed.

## Goals

- Parse Dockerfile structure without importing BuildKit or another Dockerfile parser.
- Make rules operate on typed instructions and stages rather than raw strings.
- Give every rule and finding a stable identity, severity, and source location.
- Make CLI results enforceable in CI through documented exit codes.
- Publish a stable JSON schema whose empty collections are never `null`.
- Validate stack overrides and report detected-but-unsupported stacks honestly.
- Test production rules and every failure reproduced in the audit.
- Repair CI, formatting, licensing, release documentation, and stale README claims.

## Non-goals

- Auto-fix behavior.
- Web or TUI interfaces.
- Recursive repository scanning.
- SARIF output.
- New Python, Node, or C/C++ optimization rule packs.
- Full semantic parsing of shell programs inside `RUN`.
- Compatibility with the pre-v1 JSON field names or exit behavior.

## Architecture

The v1 flow is:

```text
Dockerfile bytes
  -> internal Dockerfile parser
  -> Document / Stage / Instruction model
  -> stack detection and support registry
  -> immutable rule engine
  -> structured findings
  -> human or JSON reporter
  -> threshold-based process exit code
```

The parser owns Dockerfile syntax. The analyzer owns stack inference and rule semantics. Reporters own presentation only. The CLI coordinates these components and maps typed outcomes to process exit codes.

### Package responsibilities

- `internal/dockerfile`: parser, document model, source ranges, and parser errors.
- `internal/analyzer`: stack registry, severity type, rule interface, rule registry, evaluation context, and findings.
- `internal/report`: versioned output DTO plus human and JSON rendering.
- `cmd/dockopt`: flags, argument validation, file lifecycle, reporter selection, stderr diagnostics, and exit-code mapping.

The analyzer must not import CLI or reporter packages. Reporters must not contain analysis decisions. Rule implementations must not depend on parser-private state.

## Internal Dockerfile Parser

The parser will use the Go standard library and expose a small stable API:

```go
func Parse(name string, r io.Reader) (*Document, error)
```

The public internal model will include:

```go
type Range struct {
    StartLine int
    EndLine   int
}

type Instruction struct {
    Opcode   string
    Original string
    Value    string
    JSON     bool
    Range    Range
}

type Stage struct {
    Index       int
    Name        string
    BaseImage   string
    Platform    string
    From        Instruction
    Instructions []Instruction
}

type Document struct {
    Name         string
    EscapeToken  rune
    Instructions []Instruction
    Stages       []Stage
}
```

Names may change slightly during implementation, but the separation between document, stages, instructions, and source ranges is required.

### Required syntax coverage

The parser must support:

- Case-insensitive instruction opcodes, normalized to uppercase.
- Blank lines and comments without emitting instructions.
- Leading whitespace before instructions and comments.
- Parser directives, including `# escape=`.
- Logical line continuation using the configured escape token.
- Shell and JSON-array instruction forms.
- `FROM` flags, image reference, optional `AS` stage name, and stage ordering.
- Heredoc bodies without treating their contents as top-level instructions.
- Start and end line numbers for every logical instruction.
- Useful parse errors with a source name and line number.

The parser does not need to interpret shell pipelines or prove whether arbitrary shell commands are safe. It only needs to preserve a normalized logical `RUN` value for rules that search for specific command tokens.

### Parser error behavior

Malformed Dockerfiles return a typed parse error. Analysis does not continue on a partially parsed document. Human mode writes the diagnostic to stderr. JSON mode writes a stable error envelope to stdout and diagnostics to stderr only when needed for an output failure. Both modes exit `2`.

## Stack Detection and Support

Stack detection will inspect parsed `FROM` images and normalized `RUN` instructions only. Comments, heredoc payload text, and unrelated file names cannot influence detection.

A single registry defines:

- Valid stack names accepted by `--stack`.
- Detection patterns.
- Whether the stack has a tested stack-specific rule set.
- The rule IDs assigned to that stack.

Generic rules always run. A detected stack with no stack-specific rules is returned with `supported: false`; human output must say that only generic checks ran. An invalid explicit override is a usage error and exits `2`.

Python, Node, and C/C++ remain detectable but unsupported in this release. Go, Java, Rust, .NET, PHP, and Ruby remain supported after their existing rules are migrated and tested.

## Rule Engine

Rules are immutable, named implementations registered once at startup:

```go
type Rule interface {
    ID() string
    Severity() Severity
    Stacks() []Stack
    Evaluate(*dockerfile.Document, Context) []Finding
}
```

Each finding includes:

```go
type Finding struct {
    ID       string
    Severity Severity
    Message  string
    Range    dockerfile.Range
    Stage    *int
}
```

Rule IDs are stable across human and JSON output. Severity is a validated enum with `info`, `warn`, and `error`. The engine initializes result slices to empty non-nil slices.

The duplicate `RunChecks` and `RunChecksDetailed` paths will be replaced by one typed evaluation function. Tests will execute the production registry rather than replace it with test-only rules.

### Migrated rule behavior

- The generic latest-tag rule examines only parsed `FROM` instructions.
- The Go multistage rule uses `Document.Stages`.
- The Go final-image rule examines the actual last stage regardless of stage count.
- CGO guidance applies only when a minimal runtime target demonstrates static-binary intent, such as `scratch`; normal Debian or similar runtime targets do not receive an error.
- The PHP Composer rule checks missing `--no-dev` and missing optimized autoloading independently.
- Java, Rust, Ruby, and .NET rules use typed instructions, stable IDs, and source ranges.
- Stack-specific latest-tag checks that duplicate the generic rule are removed.

## CLI Contract

The v1 invocation is:

```text
dockopt [--json] [--stack <name>] [--fail-on none|warn|error] <Dockerfile>
```

The default failure threshold is `error`. The threshold applies after analysis and does not suppress lower-severity findings from output.

Exit codes are:

- `0`: analysis completed and no finding reached the configured threshold.
- `1`: one or more findings reached the configured threshold.
- `2`: invalid arguments, invalid stack override, parse failure, input error, or output error.

Operational diagnostics go to stderr. Analysis results go to stdout. Reporter functions return errors; the CLI handles broken pipes and failed writes instead of discarding them.

## JSON v1 Contract

Successful analysis produces:

```json
{
  "schema_version": "1",
  "source": "Dockerfile",
  "stack": {
    "detected": "go",
    "selected": "go",
    "supported": true
  },
  "findings": [],
  "summary": {
    "info": 0,
    "warn": 0,
    "error": 0
  }
}
```

Every finding contains `id`, `severity`, `message`, `line`, `end_line`, and `stage`. JSON field names are explicit lowercase snake case. `findings` is always an array. The `summary` counts all emitted findings, independent of the configured failure threshold.

JSON parse or operational errors use:

```json
{
  "schema_version": "1",
  "error": {
    "kind": "parse_error",
    "message": "Dockerfile:4: unterminated JSON instruction"
  }
}
```

The accepted error kinds are `usage_error`, `input_error`, `parse_error`, and `output_error` where output remains possible.

## Human Output

Human output shows the selected stack and whether stack-specific checks ran. Each finding includes its severity, stable rule ID, source line or line range, and message. A clean result is reported only after generic checks and, when supported, stack-specific checks complete successfully.

For a detected unsupported stack, output explicitly says that only generic rules ran; it must not claim comprehensive stack-specific cleanliness.

## Test Strategy

### Parser tests

Table-driven tests and fixtures cover:

- Comments containing instruction or stack keywords.
- Lowercase and mixed-case opcodes.
- Leading whitespace.
- Default and custom escape-token continuations.
- Shell and JSON instruction forms.
- `FROM --platform` and named stages.
- Three-or-more-stage files.
- Heredocs whose bodies contain Dockerfile-looking text.
- Accurate start and end source lines.
- Malformed JSON, continuation, heredoc, and `FROM` input.

Fuzz tests assert that arbitrary input never panics and that returned ranges are internally valid.

### Production rule tests

Every registered rule has positive and negative cases using parsed documents. Required regression cases include:

- A comment cannot trigger latest-tag or stack findings.
- Lowercase `from ...:latest` is detected.
- A multiline Go build is analyzed.
- A three-stage Dockerfile with Golang as the final image is detected.
- A normal CGO build copied into a Debian runtime does not receive a static-binary error.
- Composer production flags are checked independently.

### CLI integration tests

Integration tests build or invoke the CLI against committed fixtures and verify stdout, stderr, JSON shape, and exit status for:

- Clean analysis.
- Warning below and at the configured threshold.
- Error at the configured threshold.
- Invalid `--stack`.
- Detected unsupported stack.
- Parse failure.
- Missing input file.
- Empty findings encoded as `[]`.

## CI

The GitHub Actions workflow will trigger for Go source, module files, workflow changes, and Dockerfile fixtures. It will:

- Verify `gofmt` cleanliness.
- Run `go vet ./...`.
- Run `go test ./...`.
- Run `go test -race ./...`.
- Build `./cmd/dockopt`.
- Run CLI integration fixtures rather than a nonexistent root Dockerfile.

External actions are pinned to reviewed full commit SHAs. Automated dependency update configuration keeps those pins and Go modules current through reviewable pull requests.

## Repository and Release Hygiene

- Apply `gofmt` to all Go files.
- Remove unused wrappers after callers migrate to the typed API.
- Add the canonical MIT license text.
- Add a changelog describing the breaking v1 contract.
- Update README features, support table, JSON schema, exit codes, installation, and roadmap.
- Remove the incorrect dependency-free claim unless the final module truly has no third-party dependencies.
- Add `.vibe-scan/` to `.gitignore` so local audit output is not committed accidentally.
- Document that Python, Node, and C/C++ currently receive generic checks only.

Release tagging and publishing remain human-controlled and are not performed by the implementation task.

## Delivery Sequence

1. Introduce the internal document model and parser with parser tests.
2. Introduce typed stack, severity, rule, finding, and registry models.
3. Migrate and correct production rules with regression tests.
4. Introduce the v1 reporter DTO and human/JSON renderers.
5. Rework CLI validation, error routing, and threshold exit codes.
6. Add integration fixtures and CI validation.
7. Complete formatting, dead-code removal, licensing, changelog, README, and ignore rules.

Each step must keep `go test ./...` passing. The final verification includes formatting, vet, unit tests, race tests, build, and every audit reproduction.

## Acceptance Criteria

- Comments cannot trigger stack detection or `FROM` rules.
- Lowercase and whitespace-prefixed instructions are handled.
- Continuation and heredoc instructions retain correct source ranges.
- Multiline Go builds are analyzed.
- The actual final stage is inspected.
- Normal CGO builds do not receive false error findings.
- Invalid stack overrides exit `2`.
- Findings meeting `--fail-on` exit `1`.
- Empty JSON analysis contains `"findings": []`.
- JSON fields conform to the documented v1 schema.
- Detected unsupported stacks are never presented as fully checked.
- PHP production flags are checked independently.
- Tests execute production rules.
- CI succeeds against committed fixtures and no longer references a missing root Dockerfile.
- `gofmt`, vet, tests, race tests, and build pass.
- Actions are SHA-pinned; license, changelog, README, and `.gitignore` are correct.

## Rejected Alternatives

### BuildKit parser dependency

BuildKit provides comprehensive parsing, but the project owner chose an internal parser to minimize dependencies and retain control of the linter's syntax model.

### Logical-line normalization only

Joining continuation lines would address individual symptoms while preserving raw-string rule semantics. It was rejected because it would not provide stage modeling, reliable locations, heredoc isolation, or a durable rule API.
