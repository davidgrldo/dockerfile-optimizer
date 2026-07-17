<div align="center">

# 🐳 Dockerfile Optimizer

**A fast, stack-aware Dockerfile linter that catches image bloat and bad-practice patterns — with stable rule IDs and CI-friendly output.**

[![Verify](https://github.com/davidgrldo/dockerfile-optimizer/actions/workflows/lint-dockerfile.yml/badge.svg)](https://github.com/davidgrldo/dockerfile-optimizer/actions/workflows/lint-dockerfile.yml)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/davidgrldo/dockerfile-optimizer)](https://goreportcard.com/report/github.com/davidgrldo/dockerfile-optimizer)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![dependencies: none](https://img.shields.io/badge/dependencies-none-brightgreen)

</div>

`dockopt` parses your Dockerfiles into a real instruction model — stages, multi-line
instructions, heredocs, JSON/exec form, and `# escape` directives — then runs generic and
stack-specific rules over the result. No regex-on-raw-lines guessing, so rules don't misfire
on comments, casing, or line continuations.

## Highlights

- **🎯 Stack-aware** — detects Go, Java, Rust, .NET, PHP, and Ruby and runs targeted rules (plus generic checks for Python, Node.js, and C/C++).
- **🧠 Real parser** — understands stages, continuations, heredocs, and JSON instructions instead of grepping raw lines.
- **⚙️ Built for CI** — stable rule IDs, a configurable failure threshold, and precise exit codes.
- **📦 Batch + streaming** — analyze many files in one run; JSON output is [JSON Lines](https://jsonlines.org/), ready for `jq`.
- **🪶 Zero dependencies** — a single static Go binary. Fuzz- and race-tested.

## Quick start

Install with Go 1.24 or newer:

```bash
go install github.com/davidgrldo/dockerfile-optimizer/cmd/dockopt@latest
```

Or build from source:

```bash
git clone https://github.com/davidgrldo/dockerfile-optimizer.git
cd dockerfile-optimizer
go build -o dockopt ./cmd/dockopt
```

Then point it at a Dockerfile:

```console
$ dockopt Dockerfile
Detected stack: go
Stack-specific checks enabled.
[warn] GEN001 (line 1): Avoid using 'latest' tag in base images
[warn] GEN002 (line 2): Add '--no-install-recommends' to 'apt-get install' to avoid pulling optional packages
[warn] GEN003 (line 2): Remove the apt cache in the same RUN (rm -rf /var/lib/apt/lists/*) to keep the layer small
[warn] GO001 (line 1): Consider using multi-stage builds in Go to reduce final image size
[warn] GO003 (line 1): Avoid using golang image in final stage; copy binary to scratch/distroless/alpine
```

## Usage

```text
dockopt [--json] [--stack <name>] [--fail-on none|warn|error] <Dockerfile>...
```

Options must appear before the Dockerfile paths:

- `--json` writes the versioned JSON result instead of human-readable output.
- `--stack <name>` overrides detection with a validated stack name (applied to every path).
- `--fail-on none|warn|error` selects the failure threshold. The default is `error`.

The threshold controls only the process status; findings below the threshold still appear in the output.

```bash
./dockopt Dockerfile
./dockopt --json Dockerfile
./dockopt --stack go --fail-on warn Dockerfile
```

### Multiple files

Pass more than one path to analyze a batch; use your shell's globbing to expand patterns:

```bash
./dockopt services/*/Dockerfile
./dockopt --json $(git ls-files '*Dockerfile')
```

- Human output prefixes each report with a `==> path <==` header.
- JSON output is emitted as [JSON Lines](https://jsonlines.org/): one result object (same schema below) per line, including a per-file error envelope for any file that fails to parse. This streams cleanly into `jq -c`.
- The exit code is the most severe outcome across all paths (`2` over `1` over `0`), so one unparseable file surfaces as `2` even when the rest are clean.

### In CI

`dockopt` is a single binary with no runtime dependencies, so it drops into any pipeline. A GitHub Actions step that fails the build on any warning or worse:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: "1.24"
- run: go install github.com/davidgrldo/dockerfile-optimizer/cmd/dockopt@latest
- run: dockopt --fail-on warn $(git ls-files '*Dockerfile')
```

## Exit codes

- `0`: analysis completed and no finding reached the configured threshold.
- `1`: one or more findings reached the configured threshold.
- `2`: invalid arguments or stack, parse failure, input error, or output error.

Analysis results are written to stdout. Operational diagnostics are written to stderr.

## Stack support

Go, Java, Rust, .NET, PHP, and Ruby have stack-specific rules. Python, Node.js, and C/C++ are detected but receive generic checks only.

| Stack | Override | Stack-specific rules |
| --- | --- | --- |
| Go | `go` | ✅ |
| Java | `java` | ✅ |
| Rust | `rust` | ✅ |
| .NET | `dotnet` | ✅ |
| PHP | `php` | ✅ |
| Ruby | `ruby` | ✅ |
| Python | `python` | ⬜ generic checks only |
| Node.js | `node` | ⬜ generic checks only |
| C/C++ | `c_cpp` | ⬜ generic checks only |

Generic Dockerfile rules run for every stack.

## Rules

Rule IDs are stable and safe to reference in CI (e.g. to gate on a subset).

| ID | Severity | Applies to | Checks |
| --- | --- | --- | --- |
| `GEN001` | warn | all | Base image uses `:latest`, or is untagged (which defaults to `latest`). Stage references, `scratch`, and digest-pinned images are exempt. |
| `GEN002` | warn | all | `apt-get install` without `--no-install-recommends`. |
| `GEN003` | warn | all | `apt-get install` without clearing `/var/lib/apt/lists` in the same `RUN`. |
| `GEN004` | warn | all | `ADD <url>` instead of `RUN curl/wget` (with a checksum) or `COPY`. |
| `GEN005` | warn | all | Final stage's effective `USER` is `root`. |
| `GO001` | warn | go | Single-stage Go build (multi-stage shrinks the image). |
| `GO002` | error | go | `go build` for a `scratch` final image without `CGO_ENABLED=0` (checked on the `RUN` and on stage-level `ENV`/`ARG`). |
| `GO003` | warn | go | `golang` image used as the final stage. |
| `JAVA001` | info | java | Full JDK base image (`openjdk`, `eclipse-temurin`, `amazoncorretto`) whose tag is not a slim/JRE variant. |
| `RUST001` | warn | rust | Single-stage Rust build. |
| `DOTNET001` | warn | dotnet | `mcr.microsoft.com/dotnet/*` base image without an explicit tag. |
| `PHP001` | warn | php | `composer install` without `--no-dev`. |
| `PHP002` | warn | php | `composer install` without `--optimize-autoloader`. |
| `RUBY001` | info | ruby | `bundle install` without `--deployment`. |

> **Known limits:** the `apt-get` rules match the common `apt-get install ...` form (not `apt-get -y install`, with the flag before the subcommand), and commands inside heredoc bodies are not analyzed.

## JSON schema

Successful JSON output uses schema version `1`:

```json
{
  "schema_version": "1",
  "source": "Dockerfile",
  "stack": {
    "detected": "go",
    "selected": "go",
    "supported": true
  },
  "findings": [
    {
      "id": "GEN001",
      "severity": "warn",
      "message": "Avoid using 'latest' tag in base images",
      "line": 1,
      "end_line": 1,
      "stage": 0
    }
  ],
  "summary": {
    "info": 0,
    "warn": 1,
    "error": 0
  }
}
```

`findings` is always an array, including clean results. `summary` counts every emitted finding regardless of `--fail-on`.

JSON failures use this envelope and exit `2`:

```json
{
  "schema_version": "1",
  "error": {
    "kind": "parse_error",
    "message": "Dockerfile:4: unterminated JSON instruction"
  }
}
```

Error kinds are `usage_error`, `input_error`, `parse_error`, and `output_error` when output is still possible.

## Development

Format, inspect, test, fuzz, and build with:

```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
go test ./internal/dockerfile -run=^$ -fuzz=FuzzParse -fuzztime=3s
go build -o dockopt ./cmd/dockopt
```

The CI workflow runs the same static checks, unit tests, race tests, build, and committed fixture exit contracts. Dependency updates for Go modules and pinned GitHub Actions are proposed monthly through Dependabot.

## License

Dockerfile Optimizer is available under the [MIT License](LICENSE).
