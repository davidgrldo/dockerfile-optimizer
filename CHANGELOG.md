# Changelog

All notable changes to this project are documented in this file.

## [1.4.0] - 2026-08-30

### Added

- Cache-order rules `GO004` and `RUST002`, cache-mount rule `GEN009`, digest-pin `GEN010` (info).
- Node rules `NODE003` (yarn/pnpm frozen lockfile) and `NODE004` (`NODE_ENV=production`).
- `DOTNET002` (SDK image as final), `RUST003` (`cargo build --release`), `RUST004` (full rust image as final).
- `.dockopt.yml` (`fail-on`, `ignore`, `stack`) with `--config` / `--no-config`. CLI flags override; `--ignore` merges.
- `--fix` for mechanical `GEN002`, `GEN003`, and `GEN006` (skips heredocs and stdin).

### Changed

- `GO001` and `RUST001` only fire when the single stage actually compiles.
- `--sarif` continues after a parse/input error, writes findings for the files that parsed, and still exits `2`. The GitHub Action uploads that partial SARIF.

## [1.3.0] - 2026-08-29

### Added

- Per-stage stack detection: stack-specific rules run for every matching stage, not only the document-level stack. `stack.detected` prefers the final stage.
- Cache-order rules `PY002` and `NODE002` (`COPY .` before the lock/requirements file).
- `GEN008` flags secret-like `ARG`/`ENV` names.
- Directory walk (`dockopt .`) and stdin (`dockopt -`).
- RUN leading flags such as `--mount` are parsed off the command so checks see the real shell.
- Aggregated SARIF 2.1.0 output through `--sarif`.
- Suggested fixes on every finding and a reusable GitHub Action with optional Code Scanning upload.

### Changed

- `JAVA001` only inspects the final stage, so a JDK builder with a JRE/slim runtime is clean.

### Breaking

- JSON success and error envelopes now use schema version `2`; findings add the required `suggested_fix` field.

## [1.2.0] - 2026-08-29

### Added

- Stack-specific rules for Python (`PY001` pip `--no-cache-dir`), Node.js (`NODE001` prefer `npm ci`), and C/C++ (`CCPP001` compiler image as final stage).
- Generic package-manager rules: `GEN006` (`apk add` without `--no-cache`) and `GEN007` (`yum`/`dnf`/`microdnf install` without cache cleanup).
- Per-instruction `# dockopt:disable ID,ID` comments and a `--ignore ID,ID` flag to suppress findings.

### Changed

- `GEN005` now also flags a missing `USER` in the final stage (implicit root), except when the base image name/tag contains `nonroot`.
- `apt-get` rules recognize flags between the command and subcommand (e.g. `apt-get -y install`).
- Heredoc bodies are included in `RUN` analysis.

## [1.1.0] - 2026-08-29

### Fixed

- `GO002` no longer reports a false error when `CGO_ENABLED=0` is set via a stage-level `ENV`/`ARG` instead of inline in the `RUN`.
- Bash here-strings (`<<<`) are no longer misparsed as heredocs (previously a hard parse error).
- `GEN001` now also flags untagged base images (implicit `:latest`), while exempting `scratch`, digest-pinned images, and prior stage references.

### Added

- New generic rules: `GEN002` (`apt-get install` without `--no-install-recommends`), `GEN003` (`apt-get install` without clearing the apt cache in the same `RUN`), `GEN004` (`ADD <url>`), and `GEN005` (final stage running as `root`).
- Multi-file input: analyze several Dockerfiles in one run. JSON output is emitted as JSON Lines; single-file output is unchanged. The exit code is the most severe outcome across all paths.

### Changed

- `JAVA001` now covers all JDK versions and the `openjdk`, `eclipse-temurin`, and `amazoncorretto` repositories instead of only the exact tag `openjdk:17`.
- Internal analyzer cleanup with no behavior change: removed the unused rule `Context`, collapsed the single-implementation `Rule` interface, and stack detection now runs once per invocation.

## [1.0.0] - 2026-07-16

### Added

- Added an internal, dependency-free Dockerfile parser with typed instructions, stages, and stable source ranges.
- Added stable rule IDs and severity levels, plus JSON schema version `1` with non-null finding arrays and summaries.
- Added `--fail-on none|warn|error`, validated `--stack` overrides, and explicit stack-support reporting.

### Changed

- Stack detection and rules now consume parsed Dockerfile instructions instead of physical-line substring scans.
- Multiline Go builds and the actual final stage are analyzed correctly.
- CGO guidance is limited to static-binary targets such as `scratch`.
- PHP Composer production flags are checked independently.
- Removed the `pflag` dependency in favor of the Go standard library.

### Breaking

- Replaced the pre-v1 JSON fields with the versioned schema `1` success and error envelopes.
- Changed process exits to `0` below the selected threshold, `1` when findings reach it, and `2` for usage, input, parse, or output failures.
