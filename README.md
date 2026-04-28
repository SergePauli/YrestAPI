# YrestAPI - declarative read-only REST over PostgreSQL (YAML -> JSON)

[![CI](https://github.com/SergePauli/YrestAPI/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/SergePauli/YrestAPI/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/SergePauli/YrestAPI)](https://github.com/SergePauli/YrestAPI/releases)
[![License](https://img.shields.io/github/license/SergePauli/YrestAPI)](LICENSE.txt)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io%2Fsergepauli%2Fyrestapi-2496ED?logo=docker)](https://github.com/SergePauli/YrestAPI/pkgs/container/yrestapi)

> TL;DR: run a fast read-only JSON API for PostgreSQL in minutes.  
> Endpoints: `/api/index`, `/api/stats`, and deprecated `/api/count`. Contract defined in YAML.

**YrestAPI** is a declarative REST engine in Go for read-heavy PostgreSQL APIs.  
You describe models, relations, and response shapes in YAML, and YrestAPI serves JSON without ORM code or custom read handlers.

Versioning: this project publicly follows [Semantic Versioning 2.0.0](VERSIONING.md).

## Why

- Offload heavy list endpoints from your main API (`PHP`, `Ruby`, `Node`, etc.).
- Remove ORM and server-side business code from read-only API paths.
- Most MVC-oriented ORMs assume one model super-class per entity and offer only limited eager/lazy loading strategies, which is usually not expressive enough for the full variety of client views.
- YrestAPI solves that by introducing model presets: client-shaped data profiles that define SQL generation and result packing as precisely as a concrete view needs.
- This reduces overfetching, saves network traffic, and lowers database load because each request can select only the fields and relations actually needed by the target UI.
- Serve nested JSON with filters, sorts, pagination, and relation traversal.
- Shape response fields after fetch with YAML formatters instead of hand-written view code.
- Localize field values and enum-like codes through locale dictionaries, not controller logic.
- Keep the contract explicit in YAML instead of burying it in code.

## Try It In 60 Seconds

The repository includes a ready-to-run smoke stack: PostgreSQL, schema, seed data, and bundled YAML models.

```bash
# 1) clone
git clone https://github.com/SergePauli/YrestAPI.git
cd YrestAPI

# 2) start
docker compose up --build

# 3) call the API
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Project","preset":"with_members","limit":2,"sorts":["id ASC"]}'
```

Useful checks:

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Contragent","preset":"item","sorts":["id ASC"],"limit":2}'
curl -sS -X POST http://localhost:8080/api/stats \
  -H 'Content-Type: application/json' \
  -d '{"model":"Person"}'
```

Notes:

- `compose.yaml` starts PostgreSQL and applies the bundled smoke schema and seed automatically.
- The default compose setup uses `MODELS_DIR=/app/test_db` so the seeded database matches the shipped YAML models.
- If port `8080` is busy, run `HOST_PORT=8081 docker compose up --build`.

## Core Concepts

### Model YAML

Each model is a YAML file that maps a logical API model to a PostgreSQL table and declares relations, aliases, computable fields, and presets.

This is the key difference from a conventional ORM model class. In a classic MVC stack, one entity model often has to serve many different screens, widgets, tables, cards, exports, and detail views, while eager/lazy loading toggles are too coarse to describe the exact shape of data needed by each one. YrestAPI moves that view-specific behavior into model presets, so the same logical model can expose multiple precisely tuned data profiles.

Example:

```yaml
table: people
aliases:
  org: "contragent.organization"
relations:
  contacts:
    model: Contact
    type: has_many
    through: PersonContact
presets:
  item:
    fields:
      - source: id
        type: int
      - source: contacts
        type: preset
        preset: item
```

### Presets

Presets define the exact JSON shape returned to the client. They are intentionally client-shaped, not table-shaped.

In practice, a preset controls both:

- how SQL is generated for a concrete request
- how the result is packed back into JSON after fetching

That means presets are not just presentation aliases. They are execution profiles for a model, optimized for the needs of a specific client view.

Example:

```yaml
presets:
  card:
    fields:
      - source: id
        type: int
      - source: "{person_name.value} {last_name}[0]."
        type: formatter
        alias: short_label
      - source: org
        type: preset
        preset: short
```

### Post-Processing: Formatters And Localization

This is one of the strongest parts of the engine.

- `formatter` fields let you compose display values from fetched data after SQL execution
- formatter expressions can traverse nested fields, slice strings, and use ternary logic
- `localize: true` lets you map raw DB values to locale dictionaries from `cfg/locales/<locale>.yml`
- together they let YAML describe not only data selection, but also client-facing presentation fields

That means you can build fields like:

- short names
- labels assembled from related objects
- localized statuses and enum values
- compact display strings for nested relations

See details:

- [Formatter](DOCS.md#formatter)
- [Localization](DOCS.md#localization)
- [HTTP API](DOCS.md#http-api)
- [Import](DOCS.md#import)

### Relations

Supported relation types:

- `has_many`
- `has_one`
- `belongs_to`
- `through`
- polymorphic `belongs_to`

Recursive/self relations are supported, but cyclic traversal must be explicit via `reentrant: true` and bounded via `max_depth`.

### Filters, Sorts, Pagination

Requests can:

- filter by scalar fields and dotted relation paths
- sort by direct, related, alias, or computable fields
- paginate with `offset` and `limit`
- combine conditions with `and` / `or`

Example filter keys:

- `name__cnt`
- `name__not_cnt`
- `org.name__eq`
- `persons.last_name__eq`
- `id__in`
- `status_id__null`

## API

All API requests are `POST` with JSON bodies.

### Client SDKs

- PHP SDK for YrestAPI with PSR-18 support: https://github.com/SergePauli/yrestapi-php

### `POST /api/index`

Returns a JSON array of items shaped by the requested preset.

Payload:

```json
{
  "model": "Person",
  "preset": "card",
  "filters": {
    "name__cnt": "John",
    "org.name_or_org.full_name__cnt": "IBM"
  },
  "sorts": ["org.name DESC", "id ASC"],
  "offset": 0,
  "limit": 50
}
```

Response:

```json
[
  {
    "id": 1,
    "short_label": "John Smith S.",
    "org": {
      "name": "IBM"
    }
  }
]
```

Behavior notes:

- relation traversal uses dotted paths such as `org.name`
- aliases can shorten long paths
- string filters are case-insensitive by default, including `__not_cnt`
- invalid payloads return `400`
- SQL/build/runtime errors return `500`

### `POST /api/stats`

Returns a single integer count for the same filter semantics.
If `aggregates` is omitted, the response stays `{"count": N}`.
`POST /api/count` remains supported as a backward-compatible deprecated alias.

Payload:

```json
{
  "model": "Person",
  "filters": {
    "org.name__cnt": "IBM"
  }
}
```

Response:

```json
{
  "count": 123
}
```

Aggregate payload shape:

```json
{
  "model": "Employee",
  "filters": {
    "organization_id__eq": 1
  },
  "aggregates": {
    "sum_id": { "fn": "sum", "field": "id" },
    "avg_id": { "fn": "avg", "field": "id" },
    "min_hired_at": { "fn": "min", "field": "hired_at" },
    "max_hired_at": { "fn": "max", "field": "hired_at" }
  }
}
```

Aggregate response:

```json
{
  "count": 2,
  "aggregates": {
    "sum_id": 201,
    "avg_id": 100.5,
    "min_hired_at": "2022-02-02",
    "max_hired_at": "2023-03-03"
  }
}
```

Aggregate restrictions:

- only whitelisted model fields from `aggregatable` config are allowed
- supported functions are `sum`, `avg`, `min`, `max`
- arbitrary SQL expressions in request payloads are not accepted

## Security

Authentication is optional and controlled by `AUTH_ENABLED`.

When `AUTH_ENABLED=true`, YrestAPI expects:

```text
Authorization: Bearer <token>
```

Supported JWT validation modes:

- `HS256`
- `RS256`
- `ES256`

Supported claim validation:

- `iss`
- `aud`
- `exp`
- `nbf`
- `iat`

Example `HS256` configuration:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=HS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_HMAC_SECRET=replace-with-strong-shared-secret
AUTH_JWT_CLOCK_SKEW_SEC=60
```

Example `RS256` configuration:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=RS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_PUBLIC_KEY_PATH=/etc/yrestapi/keys/auth_public.pem
AUTH_JWT_CLOCK_SKEW_SEC=60
```

CORS:

- default `CORS_ALLOW_ORIGIN=*`
- default `CORS_ALLOW_CREDENTIALS=false`
- for production, set explicit origins instead of `*`
- only enable credentials when you really need browser cookies or auth propagation

Example localized field in YAML:

```yaml
fields:
  - source: status
    type: int
    localize: true
```

## Observability

### Logs

- structured JSONL logs are written to `log/app.log`
- request logging includes method, path, and status
- startup failures are also emitted to stderr with a short reason
- `GET /debug/logs` returns recent log entries from `log/app.log`
- `/debug/logs` supports `level`, `limit`, `msg`, `key`, and `value` query filters
- `msg` filters by partial match against the log `msg` field
- `key` filters by exact JSON field name in the log entry
- `value` filters by partial match against any value in the log entry
- `/debug/logs` is protected by the `X-Debug-Token` header instead of JWT
- configure `DEBUG_LOGS_TOKEN` with a shared secret and send it as `X-Debug-Token: <token>`

### Health Endpoints

- `GET /healthz` returns `200 OK` when the HTTP process is alive
- `GET /readyz` returns `200 OK` only when the model registry is initialized and PostgreSQL is reachable

These endpoints are unauthenticated and intended for container or orchestrator probes.

## Production Deployment

### Docker Image

Build locally:

```bash
docker build -t yrestapi:local .
```

The repository also includes a release workflow that publishes a Docker image for tags `v*`.

### Configuration

Configuration is read from environment variables:

| Env var | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port |
| `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/app?sslmode=disable` | PostgreSQL DSN |
| `MODELS_DIR` | `./db` | Directory with YAML model files |
| `LOCALE` | `en` | Default locale |
| `AUTH_ENABLED` | `false` | Enable JWT auth |
| `AUTH_JWT_VALIDATION_TYPE` | `HS256` | JWT algorithm |
| `AUTH_JWT_ISSUER` | empty | Required issuer |
| `AUTH_JWT_AUDIENCE` | empty | Required audience |
| `AUTH_JWT_HMAC_SECRET` | empty | Shared secret for `HS256` |
| `AUTH_JWT_PUBLIC_KEY` | empty | Inline PEM public key |
| `AUTH_JWT_PUBLIC_KEY_PATH` | empty | PEM public key path |
| `AUTH_JWT_CLOCK_SKEW_SEC` | `60` | Allowed clock skew |
| `CORS_ALLOW_ORIGIN` | `*` | Allowed CORS origin(s) |
| `DEBUG_LOGS_TOKEN` | empty | Shared token required by `/debug/logs` via `X-Debug-Token` |
| `CORS_ALLOW_CREDENTIALS` | `false` | Send `Access-Control-Allow-Credentials: true` |
| `ALIAS_CACHE_MAX_BYTES` | `0` | Alias cache limit, `0` = unlimited |

Model directory resolution:

- if `MODELS_DIR` is explicitly set, that path is used
- otherwise the service tries `./db`
- if `./db` has no model `.yml` files, it falls back to `./test_db`

In production you will typically:

- mount your own model directory
- point `MODELS_DIR` at it
- provide `cfg/locales/<locale>.yml` for the selected `LOCALE`

### Upgrade Notes

- the public API surface is intentionally small: `/api/index`, `/api/stats`, deprecated `/api/count`, `/healthz`, `/readyz`
- release notes are generated from [CHANGELOG.md](CHANGELOG.md)
- versioning follows [VERSIONING.md](VERSIONING.md)
- detailed engine documentation lives in [DOCS.md](DOCS.md)

## Running Tests

`make test` runs unit and integration tests:

```bash
make test
```

Before running tests:

1. Ensure PostgreSQL is available on `localhost` or `127.0.0.1`.
2. Ensure `POSTGRES_DSN` is valid.
3. Ensure `APP_ENV` is not `production`.

Integration test behavior:

- test bootstrap derives a test DSN from `POSTGRES_DSN`
- it creates database `test`
- it applies migrations from `migrations/`
- it drops database `test` after the run
- non-local DB hosts are rejected for safety

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

YrestAPI is licensed under the GNU General Public License v3.0 or, at your option, any later version (`GPLv3+`).
See [LICENSE.txt](LICENSE.txt).

### License in practice

This section is only a practical summary for engineering discussions. It is not legal advice, not a separate license, and not an additional permission beyond [LICENSE.txt](LICENSE.txt).

- YrestAPI is a copyleft project under `GPLv3+`.
- Internal evaluation and internal use are usually the least complicated scenarios.
- Distribution of the software, modified versions, or products that include GPL-covered code is where GPL obligations usually become relevant.
- If your planned use is commercial and you need a separate licensing conversation, open a GitHub issue with the `licensing` label.
- Dual licensing is not offered by default today. It may be considered later, but only after an explicit, deliberate maintainer decision.
