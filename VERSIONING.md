# Versioning Policy

This project follows [Semantic Versioning 2.0.0](https://semver.org/).

## Public Version Format

Released versions use the format:

```text
MAJOR.MINOR.PATCH
```

Examples:

- `1.0.0`
- `1.1.0`
- `1.1.3`

Git tags should use the corresponding `v` prefix:

- `v1.0.0`
- `v1.1.0`

## Compatibility Rules

### MAJOR

Increment `MAJOR` when making incompatible public changes, including:

- breaking HTTP API contract changes
- breaking YAML model schema changes
- removing or changing documented behavior in a non-backward-compatible way
- changing import behavior in a way that requires user migration

### MINOR

Increment `MINOR` when adding backward-compatible functionality, including:

- new endpoints
- new YAML capabilities
- new importer modes
- new optional configuration or operational features

### PATCH

Increment `PATCH` for backward-compatible fixes and non-breaking maintenance work, including:

- bug fixes
- test-only changes
- internal refactors without public behavior change
- documentation-only clarifications

## Public Surface Covered by SemVer

The SemVer policy applies to documented public behavior of:

- HTTP endpoints
- request and response contract
- YAML model format and supported keys
- importer CLI behavior and generated YAML shape
- documented environment variables and operational endpoints

## Pre-Release Versions

If pre-releases are needed, use normal SemVer pre-release identifiers, for example:

- `1.2.0-rc.1`
- `1.2.0-beta.2`

## Release Process

- Update `CHANGELOG.md`
- Choose the next version according to this policy
- Create a Git tag in the form `vMAJOR.MINOR.PATCH`
- Publish release notes that call out breaking changes, if any
