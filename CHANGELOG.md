# Changelog

All notable changes to this project are documented in this file.

## [1.0.0] - Unreleased

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
