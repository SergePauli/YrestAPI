# YrestAPI Docs

Detailed reference for YrestAPI configuration, request semantics, post-processing, import tooling, and runtime behavior.

## Running Tests

`make test` runs both unit and integration tests (`go test -v ./...`).

Before running tests:

1. Ensure PostgreSQL is available on local host (`localhost` or `127.0.0.1`).
2. Ensure `POSTGRES_DSN` is valid.
3. Ensure `APP_ENV` is not `production`.

Run:

```bash
make test
```

Integration test behavior:

- test bootstrap derives test DSN from `POSTGRES_DSN`
- creates DB `test`
- applies migrations from `migrations/`
- drops DB `test` after tests
- rejects non-local DB hosts for safety

## HTTP API

All requests are `POST` with JSON bodies.

### `/api/index`

Fetch a list of records using a model preset.

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

Rules:

- `model` is the logical model name from `db/*.yml`
- `preset` is the preset name inside the model
- `filters` is a map of `field__op: value`

Supported filter operators:

- `__eq` default equality
- `__cnt` contains
- `__start` prefix
- `__end` suffix
- `__lt`
- `__lte`
- `__gt`
- `__gte`
- `__in`

String behavior:

- `__eq`, `__cnt`, `__start`, `__end` are case-insensitive by default
- case-sensitive variants: `__eq_cs`, `__cnt_cs`, `__start_cs`, `__end_cs`

Null behavior:

- `field__null: true` -> `IS NULL`
- `field__null: false` -> `IS NOT NULL`
- aliases: `field__is_null`, `field__not_null`

Grouping:

```json
{
  "or": { "id__in": [0, 1], "id__null": true },
  "status_id__null": false
}
```

Composite fields:

- join multiple paths with `_or_` and `_and_`
- example: `org.name_or_org.full_name__cnt`

Aliases and computable fields:

- aliases declared in `aliases:` can be used in filters and sorts
- computable fields declared under `computable:` can also be used directly

Sort syntax:

- array of strings like `["path ASC", "other DESC"]`
- supports aliases and computable fields the same way as filters

Response:

- success: HTTP `200` with JSON array
- invalid JSON / unknown model / unknown preset: HTTP `400`
- SQL/build/runtime issues: HTTP `500`

Example success response:

```json
[
  {
    "id": 1,
    "name": "John Smith",
    "org": { "name": "IBM" }
  }
]
```

### `/api/count`

Returns a single integer (`{"count": N}`) for the same filter semantics.

Payload:

```json
{
  "model": "Person",
  "filters": { "org.name__cnt": "IBM" }
}
```

Notes:

- filters and sorts can traverse relations with dotted paths
- path resolution goes through the alias map
- alias maps are cached in memory
- query execution hits PostgreSQL directly

## Service Configuration

Configuration is read from environment variables.

| Env var | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port for the API server |
| `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/app?sslmode=disable` | PostgreSQL connection string |
| `MODELS_DIR` | `./db` | Path to directory with YAML model files |
| `LOCALE` | `en` | Default locale for localization |
| `AUTH_ENABLED` | `false` | Enable JWT auth middleware |
| `AUTH_JWT_VALIDATION_TYPE` | `HS256` | JWT signature algorithm: `HS256` / `RS256` / `ES256` |
| `AUTH_JWT_ISSUER` | empty | Required `iss` claim value |
| `AUTH_JWT_AUDIENCE` | empty | Required `aud` claim value, single or CSV |
| `AUTH_JWT_HMAC_SECRET` | empty | Shared secret for `HS256` |
| `AUTH_JWT_PUBLIC_KEY` | empty | PEM public key for `RS256` / `ES256` |
| `AUTH_JWT_PUBLIC_KEY_PATH` | empty | Path to PEM public key for `RS256` / `ES256` |
| `AUTH_JWT_CLOCK_SKEW_SEC` | `60` | Allowed clock skew for `exp` / `nbf` / `iat` |
| `CORS_ALLOW_ORIGIN` | `*` | Value for `Access-Control-Allow-Origin` |
| `CORS_ALLOW_CREDENTIALS` | `false` | Set `Access-Control-Allow-Credentials: true` |
| `ALIAS_CACHE_MAX_BYTES` | `0` | Max bytes for in-memory alias cache, `0` means unlimited |

Resolution of `MODELS_DIR`:

- if `MODELS_DIR` is explicitly set, that path is used as-is
- otherwise the service first tries `./db`
- if `./db` is missing or contains no model `.yml` files, it falls back to `./test_db`

The repository keeps `db/` as an intentionally empty primary model directory placeholder.

For Docker DX, the image copies both `/app/db` and `/app/test_db`.

## Health Checks

- `GET /healthz` returns `200 OK` while the HTTP loop is alive
- `GET /readyz` returns `200 OK` only when the model registry is initialized and PostgreSQL is reachable
- both endpoints are unauthenticated and intended for liveness/readiness probes

## Operational Diagnostics

- structured logs are written to `log/app.log` in JSONL format
- `GET /debug/logs` returns recent entries from that file
- query params:
  - `level=debug|info|warn|error`
  - `limit=1..100` with default `20`
  - `msg=<substring>` for case-insensitive filtering by `msg`
  - `key=<field>` for exact JSON field-name matches
  - `value=<substring>` for case-insensitive partial matches across any value in the entry
- example: `GET /debug/logs?level=error&key=error&value=tcp&limit=5`
- `/debug/logs` is protected by a shared debug token instead of JWT
- configure `DEBUG_LOGS_TOKEN` and send it as `X-Debug-Token: <token>`

## Authorization

When `AUTH_ENABLED=true`, each API request must include `Authorization: Bearer <token>`.

Token validation is fully local:

- signature
- `iss`
- `aud`
- `exp`
- `nbf`
- `iat`

Example `HS256`:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=HS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_HMAC_SECRET=replace-with-strong-shared-secret
AUTH_JWT_CLOCK_SKEW_SEC=60
```

Multiple audiences may be passed as CSV:

```env
AUTH_JWT_AUDIENCE=service-a,service-b
```

Example `RS256`:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=RS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_PUBLIC_KEY_PATH=/etc/yrestapi/keys/auth_public.pem
AUTH_JWT_CLOCK_SKEW_SEC=60
```

## Import

### Import from DSN

Generate YAML models from a PostgreSQL schema via `POSTGRES_DSN`:

```bash
make import ARGS="-help"
make import ARGS="-dry-run"
make import ARGS="-dry-run -only-simple"
make import ARGS="-out ./db_imported"
make import ARGS="-dsn 'postgres://user:pass@localhost:5432/app?sslmode=disable' -out ./db_imported"
make import ARGS="-prisma-schema ./prisma/schema.prisma -out ./db_imported"
make import ARGS="-graphql-queries ./gateway/queries -models-dir ./db -dry-run"
```

Supported SQL import modes:

- `-only-simple`: phase one, tables without outgoing relations
- without `-only-simple`: imports models with `belongs_to` and reverse `has_many`; also adds related `item` presets into `full_info` for `belongs_to`

Generated `has_many` relations receive a helper preset:

```yaml
presets:
  with_project_members:
    fields:
      - source: project_members
        type: preset
        preset: item
```

### Import from Prisma schema

- pass `-prisma-schema <path>` to read models from `schema.prisma`
- in this mode `-dsn` is optional and unused
- output keeps the same YAML shape as SQL mode
- reverse `has_many` relations are generated automatically
- helper presets `with_<relation>` are generated for each `has_many`
- Prisma enum fields are generated as `type: int` with `localize: true`
- enum dictionaries are merged into `cfg/locales/<LOCALE>.yml` or fallback `cfg/locales/en.yml`

### Import presets from GraphQL queries

- pass `-graphql-queries <path>` to read GraphQL documents and update presets in existing YAML models
- this mode does not create models or relations
- model lookup uses `root field -> model name`
- preset names are generated from root field, operation name, and shape hash
- nested GraphQL selections are imported only when the relation already exists in YAML
- `source` is taken directly from the GraphQL field name

## How The Engine Works

- at startup the service loads `.yml` model files from `MODELS_DIR`
- it builds a registry of models, relations, presets, computable fields, and aliases
- it validates the graph
- the registry stays in memory and is reused for all requests

Validation checks:

- all referenced models, relations, and presets exist
- relation types are valid
- FK/PK defaults are applied correctly
- `through` chains are consistent
- polymorphic `belongs_to` is allowed only with `polymorphic: true`
- `type: preset` fields reference an existing relation and nested preset
- `type: formatter` fields define an alias
- `type: computable` fields reference an existing computable definition
- unknown YAML keys and invalid types fail startup

If validation fails, startup is aborted.

### Model YAML Structure

```yaml
table: people
aliases:
  org: "contragent.organization"
computable:
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
relations:
  person_name:
    model: PersonName
    type: has_one
    where: .used = true
presets:
  card:
    fields:
      - source: id
        type: int
      - source: person_name
        type: preset
        preset: item
      - source: fio
        type: computable
        alias: full_name
```

Key points:

- `table` is mandatory
- `relations` define the graph, optionally with `through`, `where`, `order`, `polymorphic`
- `presets` describe fields to select and return
- `type: preset` walks relations
- `type: computable` inserts expressions
- `type: formatter` post-processes values
- `type: nested_field` copies nested JSON branches
- `computable` and `aliases` are global per model

### Recursive And Self Relations

Example:

```yaml
table: contracts
relations:
  next:
    model: Contract
    type: has_one
    fk: prev_contract_id
    reentrant: true
    max_depth: 3
presets:
  chain:
    fields:
      - source: id
        type: int
      - source: next
        type: preset
        preset: chain
```

Rules:

- `reentrant: true` is required to return to an already visited model
- `max_depth` limits repeated traversal on one path
- `field.max_depth` overrides relation `max_depth`
- if `reentrant: false`, cyclic re-entry fails startup validation
- if `max_depth` is exceeded, traversal is capped
- if omitted for a reentrant cycle, default `max_depth=3` is applied with a warning

## Reusing Templates With `include`

You can pull relations and presets from `db/templates/*.yml`:

```yaml
include: shared_relations
```

Or:

```yaml
include: [shared_relations, auditable]
```

Rules:

- relations from templates are added if missing
- if a relation exists in the model, empty fields are filled from the template
- template preset fields are applied first
- model fields override or extend by alias/source
- fields marked with `alias: skip` in templates are ignored

## `where` And `through_where`

A leading `.` in a condition is replaced with the unique SQL alias of that relation.

Example:

```yaml
relations:
  phone:
    model: Contact
    type: has_one
    through: PersonContact
    where: .type = 'Phone'
    through_where: .used = true
```

SQL shape:

```sql
LEFT JOIN person_contacts AS pc
ON (main.id = pc.person_id)
AND (pc.used = true)

LEFT JOIN contacts AS c
ON (pc.contact_id = c.id)
AND (c.type = 'Phone')
```

Meaning:

- `where` filters the final relation table
- `through_where` filters the intermediate table

## Formatter

Formatters transform or combine field values after SQL execution and after merging related data.

They are useful when you want to:

- collapse a nested preset into a string
- build computed display text
- derive compact labels from nested objects
- apply conditional display logic without controller code

### Syntax

Inline computed field:

```yaml
- source: "{surname} {name}[0] {patronymic}[0..1]"
  type: formatter
  alias: full_name
```

Formatter on a relation:

```yaml
- source: contacts
  type: preset
  alias: phones
  formatter: "{type}: {value}"
  preset: phone_list
```

### Token Rules

Inside `{...}` you can use:

- `{field}`
- `{relation.field}`
- `{name}[0]`
- `{name}[0..1]`

### Behavior By Relation Type

| Relation type | Result |
| --- | --- |
| `belongs_to` | string from related object |
| `has_one` | string from child object |
| `has_many` | array of strings |
| simple field | string from current row |

### Example

```yaml
presets:
  card:
    fields:
      - source: id
        type: int
        alias: id
      - source: "{person_name.value}"
        type: formatter
        alias: name
      - source: contacts
        type: preset
        alias: contacts
        formatter: "{type}: {value}"
        preset: contact_short
```

Result:

```json
[
  {
    "id": 64,
    "name": "Ivanov A V",
    "contacts": ["Phone: +7 923 331 49 55", "Email: example@mail.com"]
  }
]
```

### Ternary Operators

Syntax:

```yaml
{ <condition> ? <then> : <else> }
```

Condition forms:

- full form: `<field> <op> <value>`
- shorthand: just `<field>` for truthy/falsy

Supported operators:

- `==`
- `=`
- `!=`
- `>`
- `>=`
- `<`
- `<=`

Supported literals:

- numbers
- booleans
- `null`
- quoted strings

Examples:

```yaml
- source: `{? used ? "+" : "-"}`
  type: formatter
  alias: used_flag

- source: `{? age >= 18 ? "adult" : "minor"}`
  type: formatter
  alias: age_group

- source: `{? status == "ok" ? "✔" : "✖"}`
  type: formatter
  alias: status_icon
```

Nested ternaries:

```yaml
- source: `{? used ? "{? age >= 18 ? "adult" : "minor"}" : "-"}`
  type: formatter
  alias: nested_example
```

Combining conditionals and substitutions:

```yaml
- source: '{? used ? "+" : "-"} {naming.surname} {naming.name}[0].'
  type: formatter
  alias: short_name
```

Notes:

- formatter fields must define an alias
- formatter fields are not included in SQL queries
- they are resolved only at post-processing stage

## Field Localization

- dictionaries live in `cfg/locales/<locale>.yml`
- the active locale is loaded into a tree structure
- date/time formats can be customized via `layoutSettings`
- lookup order is `model -> preset -> field`, then fallback to more global matches
- if nothing is found, the original value is returned
- to localize a field set `localize: true`
- numeric codes are matched as numbers when used with `type: int`

Example dictionary:

```yaml
Person:
  list:
    status:
      0: "Inactive"
      1: "Active"
  gender:
    male: "Male"
    female: "Female"
```

Example fields:

```yaml
fields:
  - source: status
    type: int
    localize: true
  - source: gender
    type: string
    localize: true
```

Example locale layouts:

```yaml
layoutSettings:
  date: "02.01.2006"
  ttime: "15:04:05"
  datetime: "02.01.2006 15:04:05"
```

## Nested Fields

Use `type: nested_field` with a path in `{...}` to lift nested data into the current preset without SQL joins.

Example:

```yaml
- source: "{person.contacts}"
  type: nested_field
  alias: contacts
```

The `contacts` array from nested `person` will be copied to the current item even if `person` itself is not exposed.

## Computable Fields

Calculated fields are declared at model level and are available in all presets.

```yaml
computable:
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
  stages_sum:
    source: "(select sum({stages}.amount))"
    where: "(select sum({stages}.amount))"
    type: float
presets:
  card:
    fields:
      - source: fio
        alias: full_name
        type: computable
      - source: stages_sum
        type: computable
```

Rules:

- `{path}` placeholders are replaced with SQL aliases from alias map
- wrap subqueries in parentheses so they can be safely used in `SELECT`
- for filters and sorts, refer to the computable field by name

## DRY Config: Inheritance, Templates, Nested Fields

### Multiple preset inheritance

```yaml
presets:
  base:
    fields:
      - source: id
        type: int
      - source: name
        type: string
        alias: name

  head:
    fields:
      - source: full_name
        type: string
        alias: name
      - source: head_only
        type: string
        alias: head_only

  item:
    extends: base, head
    fields:
      - source: okopf
        type: int
        alias: okopf
      - source: item_only
        type: string
        alias: item_only
```

## Polymorphic Relations

Declare a polymorphic `belongs_to` like this:

```yaml
relations:
  auditable:
    model: "*"
    type: belongs_to
    polymorphic: true
```

Rules:

- parent table must have `<relation>_id` and `<relation>_type`
- `type_column` may override the default type column name
- resolver batches child queries by type
- nested preset name must exist on each target model

## Known Limitations

- the service is read-only by design: only `/api/index` and `/api/count` are provided
- PostgreSQL is the only supported database backend
- model configuration is loaded and validated on startup; changing YAML files requires restart
- polymorphic relation resolution is based on `<relation>_type` values present in data
- integration tests are safety-scoped to local PostgreSQL hosts and create/drop a temporary `test` database

## License

YrestAPI is licensed under the GNU General Public License v3.0 or later (`GPL-3.0-or-later`).
See [LICENSE.txt](LICENSE.txt).
