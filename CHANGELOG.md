# Changelog

All notable changes to this project will be documented in this file.

## [1.0.0] - 2026-02-24

### Added
- Initial stable release of YrestAPI.
- Read-only JSON API endpoints: `/api/index` and `/api/count`.
- Declarative YAML model system with relations: `has_one`, `has_many`, `belongs_to`, `through`.
- Preset-driven JSON shaping, including nested presets.
- Formatter and computable field support.
- Integration test dataset and smoke coverage for complex relations.
- Recursive relation guardrails with `reentrant` and `max_depth`.

### Changed
- Startup validation for recursive presets now caps traversal by `max_depth` instead of failing on depth overflow.
- Default recursive depth policy introduced for reentrant cycles when `max_depth` is omitted (`max_depth=3`), with warning logs.

### Fixed
- Startup errors now print actionable messages to stderr (not only `exit status 1`).
- README test instructions clarified for non-default PostgreSQL port mapping.
