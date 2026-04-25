# Changelog

All notable changes to this project will be documented in this file.
This project follows [Semantic Versioning 2.0.0](VERSIONING.md).

## [Unreleased]

### Added

- Negative substring filter operator `__not_cnt` for request filters, with case-sensitive variant `__not_cnt_cs`.

## [1.1.1] - 2026-03-29

### Fixed

- Release workflow changelog lookup now strips the `v` prefix from Git tags before searching release notes sections in `CHANGELOG.md`.

## [1.1.0] - 2026-03-29

### Added

- `sqlimport` CLI for generating YAML model files from a live PostgreSQL schema in `simple` and `full` modes.
- Prisma schema import support in `sqlimport`, including generation of model files from `.prisma` sources.
- GraphQL query import support in `sqlimport`, including preset generation from GraphQL operations.
- Automatic locale defaults generation/merge for Prisma enum fields during Prisma-based imports.
- Operational endpoints: `GET /healthz` for liveness and `GET /readyz` for readiness checks.
- Operational endpoint: `GET /debug/logs` for reading recent structured application log entries over HTTP.
- `DEBUG_LOGS_TOKEN` configuration for protecting `/debug/logs` via the `X-Debug-Token` header.
- `/debug/logs` query filters: `level`, `limit`, `msg`, `key`, and `value`.
- Zero-config Docker Compose smoke stack for quick local startup and validation.
- Docker Compose-backed integration test run for the smoke suite.
- CI and release workflows for automated validation, release builds, and container publishing.

### Changed

- Importer-generated relations now include `has_many` presets and improved `has_many` naming.
- Docker/runtime startup now tolerates an empty `db/` by falling back to `test_db` for smoke and quick-start scenarios.
- Local Docker image builds now correctly overlay `cfg/` on top of `def_cfg/`, including locale files.
- `DOCS.md` was substantially rewritten into a structured reference for:
  - request DSL and runtime effects
  - model YAML DSL and runtime effects
  - resolver execution pipeline and relation materialization
- `README.md` was substantially reworked to better explain:
  - why presets exist beyond classic ORM eager/lazy loading
  - core architectural concepts
  - operational usage and client integration
  - project metadata in the first screen, including status badges and PHP SDK reference

## [1.0.0] - 2026-02-24

### Added

- Initial stable release of YrestAPI: universal API for high-speed delivery of JSON object lists, defined by `.yml` model config files at the database query layer.
- Read-only JSON API endpoints: `/api/index` and `/api/count`.
- `/api/index` supports filters, sorting, pagination, and nested relation paths.
- `/api/count` supports the same filter semantics as `/api/index`.
- Declarative YAML model system with relations: `has_one`, `has_many`, `belongs_to`, polymorphic `belongs_to`, `through`.
- Preset-driven JSON shaping, including nested presets.
- Formatter and computable field support.
- Integration test dataset and smoke coverage for complex relations.
- Recursive relation guardrails with `reentrant` and `max_depth`.
