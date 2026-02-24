# Release Checklist

Use this checklist for `v1.0.0` and future releases.

## 1. Scope freeze

- [ ] Confirm release scope (what is included and what is explicitly out of scope).
- [ ] Freeze feature changes; only bugfix/docs/test changes allowed.
- [ ] Confirm API contract for this release (`/api/index`, `/api/count`).
- [ ] Confirm supported YAML capabilities for this release (relations, presets, filters, sorting, recursion limits).

## 2. Quality gates

- [x] Run full test suite: `go test ./...`.
- [ ] Run quick startup test from `README.md` (at least one option).
- [ ] Validate startup failure behavior for broken YAML (negative test).
- [ ] Verify recursive relation guardrails (`reentrant`, `max_depth`) with at least one real config case.
- [ ] Check that `/api/index` and `/api/count` both return expected responses on smoke dataset.

## 3. Docs and release notes

- [ ] Update `CHANGELOG.md` with a new version section and date.
- [ ] Ensure `README.md` Quick Start is still executable as written.
- [ ] Ensure `README.md` documents `MODELS_DIR` contents (at least one model file).
- [ ] Ensure `README.md` documents `cfg/locales/<locale>.yml` for selected `LOCALE`.
- [ ] Add/refresh known limitations section (if any).
- [ ] Prepare concise GitHub Release notes (what shipped, why it matters, upgrade notes).

## 4. Versioning and artifacts

- [ ] Pick semantic version (example: `v1.0.0`).
- [ ] Build release image: `docker build -t yrestapi:<version> .`.
- [ ] Smoke test the built image against PostgreSQL.
- [ ] Tag release commit:

```bash
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

- [ ] Push code branch:

```bash
git push origin main
```

- [ ] Publish GitHub Release for the tag.

## 5. Operational readiness

- [ ] Confirm production env template (`PORT`, `POSTGRES_DSN`, `MODELS_DIR`, `LOCALE`, auth and CORS vars).
- [ ] Confirm logging path/permissions in container runtime.
- [ ] Define container health check strategy (endpoint or startup/readiness policy).
- [ ] Validate auth mode used in production (`AUTH_ENABLED` + JWT settings).
- [ ] Validate CORS policy for production (avoid permissive defaults unless intended).

## 6. Post-release

- [ ] Re-run smoke test on published artifact.
- [ ] Announce release with links to docs and changelog.
- [ ] Create follow-up issues for postponed items (`v1.1+` backlog).
- [ ] Monitor first adopter feedback and production errors for 24-72h.

## Definition of done for `v1.0.0`

- [ ] Tests pass.
- [ ] Changelog updated.
- [ ] README Quick Start verified.
- [ ] Docker image published and verified.
- [ ] Git tag and GitHub Release published.
