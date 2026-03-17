# Changelog

All notable changes to this project will be documented in this file.
This project follows [Semantic Versioning 2.0.0](VERSIONING.md).

## [Unreleased]

### Added

- `sqlimport` CLI for generating YAML model files from a live PostgreSQL schema in `simple` and `full` modes.
- Prisma schema import support in `sqlimport`, including generation of model files from `.prisma` sources.
- GraphQL query import support in `sqlimport`, including preset generation from GraphQL operations.
- Automatic locale defaults generation/merge for Prisma enum fields during Prisma-based imports.
- Operational endpoints: `GET /healthz` for liveness and `GET /readyz` for readiness checks.
- Zero-config Docker Compose smoke stack for quick local startup and validation.
- Docker Compose-backed integration test run for the smoke suite.

### Changed

- Importer-generated relations now include `has_many` presets and improved `has_many` naming.
- Docker/runtime startup now tolerates an empty `db/` by falling back to `test_db` for smoke and quick-start scenarios.

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
