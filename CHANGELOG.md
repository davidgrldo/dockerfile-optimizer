# Changelog

All notable changes to this project are documented in this file.

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
