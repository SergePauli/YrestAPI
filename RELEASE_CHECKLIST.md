# Release Checklist

Use this checklist for `v1.0.0` and future releases.

## 1. Scope freeze

- [Х] Confirm release scope (what is included and what is explicitly out of scope).
- [Х] Freeze feature changes; only bugfix/docs/test changes allowed.
- [Х] Confirm API contract for this release (`/api/index`, `/api/count`).
- [Х] Confirm supported YAML capabilities for this release (relations, presets, filters, sorting, recursion limits).

## 2. Quality gates

- [x] Run full test suite: `go test ./...`.
- [x] Run quick startup test from `README.md` (at least one option).
- [x] Validate startup failure behavior for broken YAML (negative test).
- [x] Verify recursive relation guardrails (`reentrant`, `max_depth`) with at least one real config case.
- [x] Check that `/api/index` and `/api/count` both return expected responses on smoke dataset.

## 3. Docs and release notes

- [Х] Update `CHANGELOG.md` with a new version section and date.
- [Х] Ensure `README.md` documents `cfg/locales/<locale>.yml` for selected `LOCALE`.
- [Х] Add/refresh known limitations section (if any).
- [x] Prepare concise GitHub Release notes (what shipped, why it matters, upgrade notes).

## 4. Versioning and artifacts

- [x] Pick semantic version (example: `v1.0.0`).
- [x] Build release image: `docker build -t yrestapi:<version> .`.
- [x] Smoke test the built image against PostgreSQL.
- [x] Tag release commit:

```bash
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

- [x] Push code branch:

```bash
git push origin main
```

- [Х] Publish GitHub Release for the tag.

## 5. Operational readiness

- [Х] Confirm production env template (`PORT`, `POSTGRES_DSN`, `MODELS_DIR`, `LOCALE`, auth and CORS vars).
- [Х] Confirm logging path/permissions in container runtime.
- [Х] Define container health check strategy (endpoint or startup/readiness policy).
- [Х] Validate auth mode used in production (`AUTH_ENABLED` + JWT settings).
- [Х] Validate CORS policy for production (avoid permissive defaults unless intended).

## 6. Post-release

- [x] Re-run smoke test on published artifact.
- [x] Announce release with links to docs and changelog.
- [x] Create follow-up issues for postponed items (`v1.1+` backlog).
- [x] Monitor first adopter feedback and production errors for 24-72h.

## Definition of done for `v1.0.0`

- [Х] Tests pass.
- [Х] Changelog updated.
- [Х] README Quick Start verified.
- [Х] Docker image published and verified.
- [Х] Git tag and GitHub Release published.
