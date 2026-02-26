# Changelog

All notable changes to this project will be documented in this file.

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
