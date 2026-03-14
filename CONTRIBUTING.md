# Contributing

## Scope

This repository contains a Go service with declarative YAML models and import tooling. Contributions should prefer small, reviewable changes with tests when behavior changes.

## Before You Start

- Search existing issues and pull requests before opening a new one.
- For security issues, do not use public issues first. Follow [SECURITY.md](SECURITY.md).
- If the change affects generated/imported YAML, include an example or fixture that shows the new behavior.

## Local Setup

```bash
make build
make test
```

Useful commands:

```bash
make run
make import ARGS="-help"
```

## Change Guidelines

- Keep changes focused. Separate refactors from behavioral changes when possible.
- Add or update tests for new behavior, bug fixes, and importer rules.
- Update `README.md` when user-facing behavior, configuration, endpoints, or import flows change.
- Follow [VERSIONING.md](VERSIONING.md) when proposing release scope or version bumps.
- Prefer ASCII in source and docs unless the file already requires non-ASCII text.

## Pull Requests

- Describe what changed and why.
- Call out risks, tradeoffs, and any compatibility impact.
- Include verification steps or command output summary.
- If a change is incomplete by design, state what is intentionally left out.

## Commit Style

Short imperative commit messages are preferred, for example:

- `router: add healthz and readyz endpoints`
- `graphqlimport: import presets from GraphQL queries`
- `prismaimport: localize enum fields and merge locale defaults`
