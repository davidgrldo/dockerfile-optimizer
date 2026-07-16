# Dockerfile Optimizer

`dockopt` analyzes one Dockerfile with generic and stack-aware rules. It understands Dockerfile stages, multiline instructions, source ranges, and JSON instructions, and emits stable rule IDs for CI use.

## Installation

Build from source with Go 1.24 or newer:

```bash
git clone https://github.com/davidgrldo/dockerfile-optimizer.git
cd dockerfile-optimizer
go build -o dockopt ./cmd/dockopt
```

The module has no third-party Go dependencies.

## Usage

```text
dockopt [--json] [--stack <name>] [--fail-on none|warn|error] <Dockerfile>
```

Options must appear before the Dockerfile path:

- `--json` writes the versioned JSON result instead of human-readable output.
- `--stack <name>` overrides detection with a validated stack name.
- `--fail-on none|warn|error` selects the failure threshold. The default failure threshold is `error`.

The threshold controls only the process status; findings below the threshold remain in the output.

Examples:

```bash
./dockopt Dockerfile
./dockopt --json Dockerfile
./dockopt --stack go --fail-on warn Dockerfile
```

## Exit codes

- `0`: analysis completed and no finding reached the configured threshold.
- `1`: one or more findings reached the configured threshold.
- `2`: invalid arguments or stack, parse failure, input error, or output error.

Analysis results are written to stdout. Operational diagnostics are written to stderr.

## Stack support

Go, Java, Rust, .NET, PHP, and Ruby have stack-specific rules. Python, Node, and C/C++ are detected but receive generic checks only.

| Stack | Override | Stack-specific rules |
| --- | --- | --- |
| Go | `go` | Yes |
| Java | `java` | Yes |
| Rust | `rust` | Yes |
| .NET | `dotnet` | Yes |
| PHP | `php` | Yes |
| Ruby | `ruby` | Yes |
| Python | `python` | No; generic checks only |
| Node.js | `node` | No; generic checks only |
| C/C++ | `c_cpp` | No; generic checks only |

Generic Dockerfile rules run for every stack.

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
