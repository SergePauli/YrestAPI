# Changelog

## [0.1.0] - 2025-08-14
### Added
- **/index endpoint**
  - Recursive relation resolution (`has_one`, `has_many`, `belongs_to`, `through`)
  - Formatters and computed fields
  - Filters and sorting
  - Grouping and merging of child data
  - Removal of `internal` fields and alias prefixes

- **YAML model validator**
  - Checks allowed keys at all structural levels
  - Validates allowed values for `type` in fields
  - Errors for typos in keys and values
  - Warnings for empty `fields` in presets

### Changed
- Refactored SQL query generation into dedicated functions
- Improved alias handling and prefix removal

### Fixed
- Fixed overwriting of formatted values when applying formatters multiple times
- Fixed crash when `nested_preset` is missing for a `preset`-type field
