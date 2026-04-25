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

#### Request DSL

This request body is also a small DSL. The engine interprets it in the following order:

1. choose model
2. choose preset
3. normalize filters and sorts through aliases
4. build alias map and required joins
5. generate `SELECT`, `WHERE`, `HAVING`, `ORDER BY`, `LIMIT`, `OFFSET`
6. fold rows into nested JSON and apply post-processing

#### `model`

Example:

```json
{ "model": "Person" }
```

Runtime effect:

- selects one model from the registry built from `db/*.yml`
- determines the root SQL table, relation graph, aliases, computable fields, and presets available to the request
- invalid model names fail before SQL generation

#### `preset`

Example:

```json
{ "preset": "card" }
```

Runtime effect:

- selects one preset of the chosen model
- determines which fields are selected, which relations are traversed, which nested presets are used, and which post-processing steps are applied
- invalid preset names fail before SQL generation

#### `filters`

Example:

```json
{
  "filters": {
    "name__cnt": "John",
    "org.name_or_org.full_name__cnt": "IBM"
  }
}
```

Runtime effect:

- filter keys are normalized through model aliases before SQL is built
- paths referenced by filters are added to the alias map, which can create additional joins even if the preset itself does not expose those fields
- non-aggregate predicates become `WHERE`
- aggregate/computable predicates may become `HAVING`

##### Filter key shape

General form:

```text
<field-or-path>__<operator>
```

If operator is omitted, default is `__eq`.

Examples:

- `name__cnt`
- `org.name__eq`
- `id__in`
- `status_id__null`

##### Filter field paths

Allowed path styles:

- direct model fields: `name`
- relation paths: `org.name`
- alias-based paths: `org.full_name`
- computable names: `fio`
- composite paths joined with `_or_` or `_and_`: `org.name_or_org.full_name`

Runtime effect:

- dotted paths traverse relations and force the SQL builder to add the necessary joins
- alias-based paths are expanded before join detection
- composite paths generate grouped predicates combined with SQL `OR` or `AND`

##### Supported filter operators

- `__eq`: equality; default when operator is omitted
- `__in`: membership against a slice
- `__lt`: less than
- `__lte`: less than or equal
- `__gt`: greater than
- `__gte`: greater than or equal
- `__cnt`: substring match
- `__not_cnt`: negative substring match
- `__start`: prefix match
- `__end`: suffix match
- `__null`: `IS NULL` / `IS NOT NULL` depending on boolean value
- `__is_null`: unconditional `IS NULL`
- `__not_null`: unconditional `IS NOT NULL`

##### String matching behavior

Runtime effect:

- `__eq`, `__cnt`, `__not_cnt`, `__start`, `__end` are case-insensitive by default
- case-insensitive equality uses `LOWER(field) = LOWER(?)`
- case-insensitive contains/prefix/suffix use `ILIKE`; `__not_cnt` uses `NOT ILIKE`
- case-sensitive variants are available via suffix `_cs`

Examples:

- `name__eq_cs`
- `name__cnt_cs`
- `name__not_cnt_cs`
- `name__start_cs`
- `name__end_cs`

##### Null handling

Examples:

```json
{
  "filters": {
    "deleted_at__null": true,
    "status_id__not_null": true
  }
}
```

Runtime effect:

- `field__null: true` becomes `field IS NULL`
- `field__null: false` becomes `field IS NOT NULL`
- `field__is_null` becomes `field IS NULL`
- `field__not_null` becomes `field IS NOT NULL`

##### Grouping with `or` / `and`

Example:

```json
{
  "filters": {
    "or": {
      "id__in": [0, 1],
      "id__null": true
    },
    "status_id__null": false
  }
}
```

Runtime effect:

- top-level keys are combined with implicit `AND`
- nested `or` / `and` groups create explicit boolean subexpressions
- array-valued `or` / `and` groups are treated as multiple nested groups

#### `sorts`

Example:

```json
{
  "sorts": ["org.name DESC", "id ASC"]
}
```

Runtime effect:

- each sort entry is parsed as `<path> [ASC|DESC]`
- sort paths are normalized through aliases before SQL generation
- sort paths also contribute to join detection, even if the sorted field is not returned by the preset
- sorting can target direct fields, relation paths, aliases, and computable fields
- if the query requires `DISTINCT`/grouping because of `has_many`, sort expressions may be injected into `SELECT` / `GROUP BY`

#### `offset`

Example:

```json
{ "offset": 100 }
```

Runtime effect:

- adds SQL `OFFSET 100`
- shifts the result window after filtering and sorting
- has no effect when omitted or `0`

#### `limit`

Example:

```json
{ "limit": 50 }
```

Runtime effect:

- adds SQL `LIMIT 50`
- caps the number of root rows returned by `/api/index`
- has no effect when omitted or `0`

#### Combined effect of filters, sorts, and pagination

The request:

```json
{
  "model": "Person",
  "preset": "card",
  "filters": {
    "org.name__cnt": "IBM"
  },
  "sorts": ["org.name ASC", "id DESC"],
  "offset": 50,
  "limit": 25
}
```

causes the engine to:

1. resolve model `Person`
2. load preset `card`
3. expand alias paths inside filters and sorts
4. add joins required by `org.name`
5. generate `WHERE` for `org.name ILIKE '%IBM%'`
6. generate `ORDER BY org.name ASC, id DESC`
7. apply `OFFSET 50`
8. apply `LIMIT 25`
9. scan SQL rows, fold nested relations, then run formatter/localization cleanup

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

- uses the same model and filter DSL as `/api/index`
- still normalizes aliases and traverses dotted relation paths
- generates a `COUNT` query instead of row selection
- `sorts`, `offset`, and `limit` do not shape the returned payload, because the result is a single scalar count

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

The service is organized as a staged execution pipeline.

### 1. Router Layer

Runtime responsibility:

- accepts HTTP requests on `/api/index` and `/api/count`
- applies CORS policy
- applies JWT validation when `AUTH_ENABLED=true`
- records request/response logs
- dispatches valid requests to the corresponding handler

Practical effect:

- router does not know business structure of models
- it is only responsible for HTTP concerns and passing normalized request context deeper into the stack

### 2. Handler Layer

Runtime responsibility:

- reads and validates JSON request bodies
- selects the requested model and preset names from the payload
- for `/api/index`, creates an `IndexRequest` and hands it to `Resolver`
- for `/api/count`, builds a count query directly from the same model registry and filter DSL

Practical effect:

- handlers are thin orchestration points
- they do not contain model-specific logic
- all domain structure comes from the registry built from YAML

### 3. Registry Initialization

At startup the service:

1. loads `.yml` model files from `MODELS_DIR`
2. derives logical model names from filenames
3. merges template includes
4. resolves preset inheritance
5. links relation targets and `through` chains
6. validates model graph and field configuration
7. builds alias maps used by presets

The registry then stays in memory and is reused by every request.

Validation checks include:

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

### 4. Resolver Planning

For `/api/index`, the `Resolver` is the execution planner and orchestrator.

Runtime responsibility:

- resolves `model` and `preset` against the registry
- normalizes filters and sorts through aliases
- builds or reuses the alias map for every path needed by:
  - preset fields
  - filters
  - sorts
  - computable expressions
- determines which joins and SQL expressions are required

Practical effect:

- the resolver does not hardcode any model behavior
- it derives its execution plan entirely from the request DSL plus the model DSL already loaded into the registry

### 5. Root SQL Execution

For the selected model and preset, the resolver asks the model layer to build one root SQL query.

This query may include:

- `FROM <table> AS main`
- joins required by preset fields
- extra joins required only by filters or sorts
- computable expressions in `SELECT`
- `WHERE` / `HAVING` from the filter DSL
- `ORDER BY`
- `LIMIT` / `OFFSET`

The query is then executed through the PostgreSQL pool.

### 6. Row Scan And Fold

After the root SQL returns rows:

- the scanner reconstructs flat SQL columns into item maps
- nested `belongs_to` branches already present in the root query are folded into nested JSON objects
- the resolver now has the first complete approximation of the response tree

This is the point where the engine has root objects, but may still be missing `has_one` / `has_many` branches.

### 7. Recursive And Parallel Resolver Branches

To avoid solving relation expansion with naive per-row N+1 queries, the resolver collects tail relations from the preset tree.

Runtime behavior:

- it recursively walks the preset tree through already folded `belongs_to` branches
- every `has_one` and `has_many` branch is collected as a tail specification
- for each tail, parent IDs are collected first
- one child `Resolver` request is then started for that tail
- these child resolver requests are launched in parallel using goroutines and synchronized with `sync.WaitGroup`

Why this matters:

- the engine does not run one SQL query per parent row
- instead, it batches each relation branch by parent IDs
- this keeps the relation expansion closer to:
  - one root resolver branch
  - plus one batched child resolver branch per `has_` relation
- this is how the service mitigates the classic N+1 query problem

Recursive effect:

- child resolver branches may themselves discover deeper `has_` tails
- in that case the same resolver logic repeats recursively
- so the overall execution plan is a tree of resolver branches, not a flat one-shot query

### 8. Merge And Stitch Phase

When child resolver branches finish:

- each child result set is grouped by relation foreign key
- grouped child rows are stitched back into the correct parent item
- `has_one` attaches the first grouped row or `nil`
- `has_many` attaches the full grouped slice
- polymorphic `belongs_to` branches are grouped by type and resolved by additional typed child resolver requests

Practical effect:

- the caller receives one coherent JSON document
- under the hood that JSON may have been assembled from multiple batched resolver branches running concurrently

### 9. Finalization Phase

Only after all relation branches are stitched together does the resolver run finalization.

This phase applies:

- formatter fields
- nested_field extraction
- localization
- collapse of empty `belongs_to` containers
- removal of `internal: true` helper fields
- preset alias application for final output keys

Practical effect:

- all post-processing sees the fully assembled object tree
- formatters can safely reference nested values that were fetched by child resolver branches
- the final JSON is produced after data acquisition and structural merge are complete

### 10. Response Emission

Finally:

- `/api/index` returns the finalized array of JSON objects
- `/api/count` returns a scalar count payload
- router middleware logs the resulting HTTP status and returns the encoded response to the client

## Model DSL

This section documents the YAML DSL in the same hierarchy in which the engine loads and uses it:

1. model file
2. root model keys
3. relations and computable fields
4. presets
5. preset fields
6. post-processing and localization

Naming conventions in the schema and model relations are intentionally aligned with ActiveRecord conventions from Ruby on Rails:

- logical model names are singular class-like names such as `Person`, `Contract`, `Organization`
- SQL table names are usually pluralized snake_case names such as `people`, `contracts`, `organizations`
- foreign keys normally follow the `<model>_id` convention
- relation names are expected to be readable ActiveRecord-style association names

Reference:

- Active Record Basics, Object Relational Mapping: https://guides.rubyonrails.org/active_record_basics.html#object-relational-mapping

### 1. Model File

Each file `MODELS_DIR/<Name>.yml` creates one model entry in the registry.

Example:

```text
db/Person.yml
```

Runtime effect:

- the filename without extension becomes the logical model name in the registry
- `Person.yml` creates registry key `Person`
- this logical name is what the API expects in request payloads: `"model": "Person"`
- there is no DSL key `model_name`; the name is derived from the filename

### 2. Root Model Mapping

Example:

```yaml
table: people
include: shared_relations
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
presets:
  card:
    fields:
      - source: id
        type: int
```

Supported root keys are:

- `table`
- `include`
- `aliases`
- `computable`
- `relations`
- `presets`

#### `table`

Example:

```yaml
table: people
```

Runtime effect:

- binds the logical model to a concrete SQL table
- the query builder uses this value in the root `FROM`
- all relation joins and field resolution start from this table alias
- this key is mandatory for a concrete model

#### `include`

Example:

```yaml
include: shared_relations
```

Or:

```yaml
include: [shared_relations, auditable]
```

Runtime effect:

- loads reusable fragments from `db/templates/<name>.yml`
- merges template `relations` and `presets` into the current model before linking
- model-local values win on conflicts
- template fields marked with `alias: skip` are ignored during merge

Use `include` when:

- multiple models share the same relation graph
- multiple models reuse the same preset skeleton

#### `aliases`

Example:

```yaml
aliases:
  org: "contragent.organization"
```

Runtime effect:

- creates shortcut names for long relation paths
- these shortcuts are available to filter and sort parsing
- `org.name__cnt` is resolved as `contragent.organization.name__cnt`
- aliases do not change the SQL shape by themselves; they change path resolution

#### `computable`

Example:

```yaml
computable:
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
```

Runtime effect:

- declares virtual model-level fields available to all presets of the model
- the engine inserts `source` into `SELECT` when a preset field uses `type: computable`
- `{path}` placeholders inside `source` are replaced with SQL aliases from the alias map
- if `where` is specified, it is used for filter/sort SQL when different from the `SELECT` expression

Nested keys for each computable entry:

- `source`: SQL expression or subquery used in `SELECT`
- `where`: alternative expression for `WHERE` / `ORDER BY`
- `type`: result type used by the scanner and formatter pipeline

#### `relations`

Runtime effect:

- defines the navigable graph between models
- the query builder uses relations to create joins
- nested preset fields use relation entries to know how to fetch related rows
- filter and sort paths also resolve through this graph
- relation type also determines which phase of `Resolver` will materialize the data

Resolver pipeline for relations:

1. the root `Resolver` builds one main SQL query for the requested model and preset
2. `belongs_to` branches are resolved inside that main query and folded immediately into nested objects
3. after root rows are scanned, the resolver walks the preset tree and collects all `has_one` / `has_many` tails, recursively descending through already folded `belongs_to` branches
4. each collected tail starts its own child `Resolver` call in a separate goroutine
5. child results are grouped by the relation foreign key and stitched back into the parent items
6. only after that merge step does the final post-processing run: formatter, nested_field, localization, internal cleanup, alias application

Practical consequence:

- `belongs_to` behaves like part of one root fetch pipeline
- `has_one` / `has_many` behave like child fetch pipelines scheduled in parallel and merged back later
- the final JSON still looks uniform because all branches go through one common finalization stage

See `3. Relations` below.

#### `presets`

Runtime effect:

- defines named response shapes for the model
- the resolver reads one preset and uses its fields to build `SELECT`, joins, folding, formatting, and output cleanup
- the API payload field `"preset"` selects one entry from this map

See `4. Presets` below.

### 3. Relations

Example:

```yaml
relations:
  person_name:
    model: PersonName
    type: has_one
    fk: person_id
    where: .used = true
```

Each relation name becomes a path segment that can be used:

- in preset fields with `type: preset`
- in alias definitions
- in filters and sorts
- in nested formatters and nested-field paths

#### `type`

Example:

```yaml
type: has_one
```

Supported values:

- `belongs_to`
- `has_one`
- `has_many`

Runtime effect:

- determines join direction and cardinality
- determines whether the folded result is a scalar object or an array
- affects formatter behavior for relation-based formatting
- chooses whether the relation is materialized inside the main root query or by child resolver branches

Type-specific runtime behavior:

- `belongs_to`: resolved as part of the main root query; nested objects are folded directly from the scanned SQL rows
- `has_one`: registered as a tail relation; fetched by a child resolver, grouped by parent key, then only the first grouped item is attached back
- `has_many`: registered as a tail relation; fetched by a child resolver, grouped by parent key, then the full grouped slice is attached back

#### `model`

Example:

```yaml
model: PersonName
```

Runtime effect:

- points to another registry model by logical name
- the linker resolves this string to an actual model reference during startup
- invalid model names fail startup validation
- child resolver calls for `has_one` / `has_many` use this logical model name to start the next resolver branch

#### `table`

Example:

```yaml
table: person_names
```

Runtime effect:

- overrides the SQL table used by the relation
- useful when the related SQL source differs from the target model default

#### `fk`

Example:

```yaml
fk: person_id
```

Runtime effect:

- sets the foreign key used in join generation
- changes the `ON` clause produced by the SQL builder
- for `has_one` / `has_many`, this same key is later used to group child resolver rows before stitching them back into parent items

#### `pk`

Example:

```yaml
pk: code
```

Runtime effect:

- overrides the current-model key used in the join instead of default `id`
- changes how the engine links the current row to the related table
- also determines which parent value the resolver collects before launching child resolver branches for `has_one` / `has_many`

#### `through`

Example:

```yaml
through: PersonContact
```

Runtime effect:

- tells the engine to build a join through an intermediate model
- instead of direct join `main -> target`, the SQL becomes `main -> through -> target`
- for tail relations, the child resolver request is synthesized through the intermediate model before results are unwrapped back into the final branch

#### `where`

Example:

```yaml
where: .type = 'Phone'
```

Runtime effect:

- adds an extra condition to the final relation join
- leading `.` is replaced with the generated SQL alias of that relation table

#### `through_where`

Example:

```yaml
through_where: .used = true
```

Runtime effect:

- adds an extra condition to the intermediate `through` join
- filters rows before the final related model is joined

Example SQL shape:

```sql
LEFT JOIN person_contacts AS pc
ON (main.id = pc.person_id)
AND (pc.used = true)

LEFT JOIN contacts AS c
ON (pc.contact_id = c.id)
AND (c.type = 'Phone')
```

#### `order`

Example:

```yaml
order: priority ASC, id DESC
```

Runtime effect:

- defines default ordering for rows fetched through that relation
- primarily affects the order inside `has_many` collections
- child resolver requests for tail relations inherit this order when building the nested branch query

#### `reentrant`

Example:

```yaml
reentrant: true
```

Runtime effect:

- explicitly allows returning to an already visited model on the current traversal path
- required for recursive/self-referential relation graphs
- if omitted or `false`, cyclic re-entry fails startup validation

#### `max_depth`

Example:

```yaml
max_depth: 3
```

Runtime effect:

- caps how deep a recursive traversal may go on one path
- protects SQL building and result folding from unbounded recursion
- field-level `max_depth` overrides relation-level `max_depth`
- if a reentrant cycle omits `max_depth`, default `3` is applied with a warning

#### `polymorphic`

Example:

```yaml
polymorphic: true
```

Runtime effect:

- enables polymorphic `belongs_to`
- the resolver reads `<relation>_type` or `type_column` to decide which target model to load
- resolver batches child fetches by concrete type

#### `type_column`

Example:

```yaml
type_column: auditable_kind
```

Runtime effect:

- overrides the default discriminator column `<relation>_type` for polymorphic relations

### 4. Presets

Example:

```yaml
presets:
  card:
    extends: item, head
    fields:
      - source: id
        type: int
```

Each preset is a named output shape for one API use case.

#### Preset name

Example:

```yaml
presets:
  card:
```

Runtime effect:

- becomes the value used in API requests: `"preset": "card"`
- selects the exact field tree to query, fold, localize, and return

#### `extends`

Example:

```yaml
extends: base, head
```

Runtime effect:

- merges parent preset fields into the current preset before runtime use
- parents are applied left to right
- later parents override earlier ones by alias/source but preserve field order
- local fields are applied last and also override inherited fields

#### `fields`

Runtime effect:

- ordered list of output instructions
- each field affects SQL generation, join planning, folding, formatting, or final cleanup depending on its `type`

### 5. Fields

Example:

```yaml
fields:
  - source: person_name
    type: preset
    preset: item
    alias: name
```

#### `source`

Runtime effect by field type:

- for simple scalar fields, names a SQL column to select
- for `preset`, names a relation to traverse
- for `computable`, names a key from the model `computable:` map
- for `formatter`, contains the formatter template itself
- for `nested_field`, contains a `{path}` expression to copy from already folded nested data

#### `type`

Supported values:

- `int`
- `string`
- `bool`
- `float`
- `date`
- `time`
- `datetime`
- `UUID`
- `preset`
- `computable`
- `formatter`
- `nested_field`

Runtime effect:

- tells the engine how to build SQL and how to interpret the field later
- scalar types become direct `SELECT` columns
- `preset` adds joins or related fetches
- `computable` injects SQL expressions
- `formatter` skips SQL column generation and runs in post-processing
- `nested_field` copies existing nested data after folding

#### `alias`

Example:

```yaml
alias: full_name
```

Runtime effect:

- renames the output key in the final JSON
- is also used as the stable key for field override/merge during preset inheritance
- for `formatter`, alias is required because the value does not come from a physical column

#### `preset`

Example:

```yaml
preset: item
```

Runtime effect:

- only meaningful for `type: preset`
- tells the engine which preset of the related model to apply when folding nested data

#### `internal`

Example:

```yaml
internal: true
```

Runtime effect:

- the field is still available during formatting, nested-field extraction, and intermediate folding
- after post-processing it is removed from the final response
- useful for helper fields that should influence rendering but not leak into JSON output

#### `localize`

Example:

```yaml
localize: true
```

Runtime effect:

- enables locale lookup or locale-based date/time formatting for this field
- lookup order is `model -> preset -> field`, then broader fallbacks
- if no translation is found, the original value is returned unchanged

#### `max_depth`

Example:

```yaml
max_depth: 2
```

Runtime effect:

- only relevant for recursive `type: preset` traversals
- overrides relation-level recursion depth for this field branch

### 6. Field Type Semantics

#### Scalar fields: `int`, `string`, `bool`, `float`, `date`, `time`, `datetime`, `UUID`

Example:

```yaml
- source: id
  type: int
```

Runtime effect:

- generates one `SELECT` expression from a physical column
- the scanner reads the SQL result into the output row
- `localize: true` on temporal types formats values according to locale layouts

#### Nested relation fields: `type: preset`

Example:

```yaml
- source: person_name
  type: preset
  preset: item
```

Runtime effect:

- resolves `source` as a relation name
- adds the necessary joins or related fetch plan
- folds flat SQL rows into nested JSON objects or arrays
- uses the child preset to determine the nested shape

If the field also has `formatter`, then:

```yaml
- source: contacts
  type: preset
  preset: phone_list
  alias: phones
  formatter: "{type}: {value}"
```

Runtime effect:

- related rows are fetched and folded first
- formatter then converts the nested object or array into string or string array

#### Computed SQL fields: `type: computable`

Example:

```yaml
- source: fio
  type: computable
  alias: full_name
```

Runtime effect:

- resolves `fio` from the model `computable:` map
- injects the computable SQL into `SELECT`
- returns it under the chosen alias

#### Post-processing fields: `type: formatter`

Example:

```yaml
- source: "{surname} {name}[0] {patronymic}[0..1]"
  type: formatter
  alias: short_name
```

Runtime effect:

- no standalone SQL column is emitted for this field
- after SQL execution and nested folding, formatter evaluates the template against current item data
- final string is written into `alias`

Formatter tokens may reference:

- current scalar fields: `{field}`
- nested fields: `{relation.field}`
- slices: `{name}[0]`
- ranges: `{name}[0..1]`

Formatter ternary syntax:

```yaml
{? used ? "+" : "-"}
```

Supported operators:

- `==`
- `=`
- `!=`
- `>`
- `>=`
- `<`
- `<=`

#### Structural copy fields: `type: nested_field`

Example:

```yaml
- source: "{person.contacts}"
  type: nested_field
  alias: contacts
```

Runtime effect:

- does not add SQL by itself
- copies an already available nested branch into the current object
- useful when you want to expose a nested value at a flatter output path

### 7. Localization And Layouts

Locale dictionaries live in `cfg/locales/<locale>.yml`.

Runtime effect:

- loaded once at startup for the active `LOCALE`
- used only by fields marked `localize: true`
- dictionary lookup is keyed by model name, preset name, field name, and field value

Example dictionary:

```yaml
Person:
  list:
    status:
      0: "Inactive"
      1: "Active"
layoutSettings:
  date: "02.01.2006"
  ttime: "15:04:05"
  datetime: "02.01.2006 15:04:05"
```

Runtime effect of `layoutSettings`:

- `date` changes formatting of `type: date`
- `ttime` changes formatting of `type: time`
- `datetime` changes formatting of `type: datetime`

### 8. Example: Full Request Path

```yaml
table: people
aliases:
  org: "contragent.organization"
relations:
  person_name:
    model: PersonName
    type: has_one
    where: .used = true
computable:
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
presets:
  card:
    fields:
      - source: id
        type: int
      - source: person_name
        type: preset
        preset: item
        internal: true
      - source: "{person_name.name}"
        type: formatter
        alias: name
      - source: fio
        type: computable
        alias: full_name
```

What the engine does:

1. registers model `Person` from filename `Person.yml`
2. uses `table: people` as the SQL root
3. links `person_name` to model `PersonName`
4. loads preset `card`
5. adds SQL for scalar field `id`
6. joins and folds nested data for `person_name`
7. removes `person_name` from output because `internal: true`
8. builds `name` from the formatter using nested folded data
9. injects computable SQL for `fio`
10. returns final JSON keys `id`, `name`, `full_name`

### 9. Polymorphic Relations

Example:

```yaml
relations:
  auditable:
    model: "*"
    type: belongs_to
    polymorphic: true
```

Runtime effect:

- parent table must contain `<relation>_id` and `<relation>_type`, or a custom `type_column`
- resolver reads the discriminator value to choose the concrete target model
- only `belongs_to` may be polymorphic
- polymorphic relations cannot use `through`

## Known Limitations

- the service is read-only by design: only `/api/index` and `/api/count` are provided
- PostgreSQL is the only supported database backend
- model configuration is loaded and validated on startup; changing YAML files requires restart
- polymorphic relation resolution is based on `<relation>_type` values present in data
- integration tests are safety-scoped to local PostgreSQL hosts and create/drop a temporary `test` database

## License

YrestAPI is licensed under the GNU General Public License v3.0 or later (`GPL-3.0-or-later`).
See [LICENSE.txt](LICENSE.txt).
