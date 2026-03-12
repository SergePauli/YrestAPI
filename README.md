# YrestAPI

**YrestAPI** is a declarative REST API engine in Go, built on PostgreSQL with parallel loading of `has_` relations.  
Everything is configured via YAML — no business logic in code.

---

## 🔧 Features

- 📁 **Declarative YAML config** — models, relations, presets
- ⚡ **High performance** — Go + concurrent processing
- ⚡ **In-memory alias cache** — speeds up repeated query planning
- 🔁 **`has_many`, `has_one`, `belongs_to`, `through` support**
- 🧩 **Nested presets** — JSON of arbitrary depth
- 🔎 **Filtering, sorting, pagination**
- 🛠️ **Field formatters** — template-based formatting in YAML
- 🔐 **Production-ready** — plain `Go`, `pgx`

## 🧩 When YrestAPI fits

- 🗃 **You already have PostgreSQL** and need to **spin up a JSON API fast** with zero server code.
- ⚙️ **Read-only microservice (`index`/`list`)** with nested relations and filters.
- 🐢 Existing API in Python/Ruby/Node is slow for complex selects and you want to offload `index` operations.
- 🧪 **Rapid prototyping** when there is no time to write SQL/ORM code.
- 🧵 **Layer separation**: YrestAPI can be a data provider consumed by frontend/BFF.

---

## ⚙️ Quick start

Minimal setup that actually starts the service, including required model and locale config.

Recommended strategy for production: run **YrestAPI in its own container**, next to your main API/BFF and frontend.  
YrestAPI handles fast read-only data delivery; your core API keeps writes and business logic.  
This gives a clean pipeline of narrowly specialized containers.

Prerequisites:

- Go `1.24+` (for local run variant)
- Docker (for container variant)
- curl

### Option A: local run (Go + local PostgreSQL)

```bash
git clone https://github.com/SergePauli/YrestAPI.git
cd YrestAPI

# 1) Start PostgreSQL if needed (example via Docker)
docker run -d --name yrestapi-pg \
  -e POSTGRES_DB=app \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  postgres:16-alpine

# If local port 5432 is busy, use:
# -p 5433:5432
# and set POSTGRES_DSN host port to 5433 in .env.

# 2) Wait until DB is ready
until docker exec yrestapi-pg pg_isready -U postgres -d app >/dev/null 2>&1; do sleep 1; done

# 3) Minimal DB schema for demo model
docker exec -i yrestapi-pg psql -U postgres -d app <<'SQL'
CREATE TABLE IF NOT EXISTS areas (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL
);
INSERT INTO areas (name) VALUES ('Metro Manila'), ('Moscow Oblast') ON CONFLICT DO NOTHING;
SQL

# 4) Create MODELS_DIR with at least one model preset
mkdir -p db
cat > db/Area.yml <<'YAML'
table: areas
presets:
  item:
    fields:
      - source: id
        type: int
      - source: name
        type: string
YAML

# 5) Create locale config (LOCALE=en -> cfg/locales/en.yml)
mkdir -p cfg/locales
cat > cfg/locales/en.yml <<'YAML'
layoutSettings:
  date: "2006-01-02"
  ttime: "15:04:05"
  datetime: "2006-01-02 15:04:05"
YAML

# 6) Minimal runtime config
cat > .env <<'EOF'
PORT=8080
POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/app?sslmode=disable
MODELS_DIR=./db
LOCALE=en
AUTH_ENABLED=false
EOF

# 7) Run API
go run ./cmd -d
```

Smoke check (in another terminal):

```bash
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Area","preset":"item","limit":2}'
```

### Option B: production-like run (PostgreSQL + YrestAPI in Docker)

```bash
git clone https://github.com/SergePauli/YrestAPI.git
cd YrestAPI

# 1) Create network and start PostgreSQL
docker network create yrestapi-net
docker run -d --name yrestapi-pg \
  --network yrestapi-net \
  -e POSTGRES_DB=app \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  postgres:16-alpine

# 2) Wait until DB is ready
until docker exec yrestapi-pg pg_isready -U postgres -d app >/dev/null 2>&1; do sleep 1; done

# 3) Minimal DB schema for demo model
docker exec -i yrestapi-pg psql -U postgres -d app <<'SQL'
CREATE TABLE IF NOT EXISTS areas (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL
);
INSERT INTO areas (name) VALUES ('Metro Manila'), ('Moscow Oblast') ON CONFLICT DO NOTHING;
SQL

# 4) Create MODELS_DIR and locale config in project
mkdir -p db cfg/locales
cat > db/Area.yml <<'YAML'
table: areas
presets:
  item:
    fields:
      - source: id
        type: int
      - source: name
        type: string
YAML
cat > cfg/locales/en.yml <<'YAML'
layoutSettings:
  date: "2006-01-02"
  ttime: "15:04:05"
  datetime: "2006-01-02 15:04:05"
YAML

# 5) Build and run YrestAPI container
docker build -t yrestapi:local .
docker run --rm --name yrestapi \
  --network yrestapi-net \
  -p 8080:8080 \
  -e PORT=8080 \
  -e POSTGRES_DSN=postgres://postgres:postgres@yrestapi-pg:5432/app?sslmode=disable \
  -e MODELS_DIR=/app/db \
  -e LOCALE=en \
  -e AUTH_ENABLED=false \
  yrestapi:local
```

Smoke check (in another terminal):

```bash
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Area","preset":"item","limit":2}'
```

Optional cleanup:

```bash
docker rm -f yrestapi yrestapi-pg 2>/dev/null || true
docker network rm yrestapi-net 2>/dev/null || true
```

---

## 🧪 Running tests

`make test` runs both unit and integration tests (`go test -v ./...`).

Before running tests:

1. Ensure PostgreSQL is available on local host (`localhost` or `127.0.0.1`).
2. Ensure `POSTGRES_DSN` is valid (default: `postgres://postgres:postgres@localhost:5432/app?sslmode=disable`).
3. Ensure `APP_ENV` is not `production`.

Run:

```bash
make test
```

Integration test behavior:

- Test bootstrap derives test DSN from `POSTGRES_DSN`, creates DB `test`, applies migrations from `migrations/`, and drops DB `test` after tests.
- Non-local DB hosts are rejected for safety.

---

## ⚙️ How it works (short version)

After startup, YrestAPI does four things:

1. Loads YAML models from `MODELS_DIR`.
2. Builds and validates an in-memory registry:
   - models and relations;
   - available JSON presets for each model.
3. Starts HTTP server with only two read endpoints: `/api/index` and `/api/count`.
4. For each request, resolves model + preset, generates SQL, fetches rows from PostgreSQL, and returns JSON in the requested preset shape.

In practice, this means the API contract is declared in YAML, while runtime execution stays fast thanks to compiled Go code and prebuilt registry metadata.

---

## 📡 HTTP API

All requests are `POST` with JSON bodies. Two main endpoints are available:

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

- `model` — logical model name from `db/*.yml`.
- `preset` — preset name inside the model.
- `filters` — map of `field__op: value`.
  - Operators: `__eq` (default), `__cnt` (contains), `__start` (prefix), `__end` (suffix), `__lt`, `__lte`, `__gt`, `__gte`, `__in`.
  - String operators `__eq`, `__cnt`, `__start`, `__end` are case-insensitive by default.
  - Case-sensitive override for strings: `__eq_cs`, `__cnt_cs`, `__start_cs`, `__end_cs`.
  - Null checks: `field__null: true` → `IS NULL`, `field__null: false` → `IS NOT NULL` (aliases: `field__is_null`, `field__not_null`).
  - Grouping: use `or` / `and` keys to nest conditions, e.g.:
    ```json
    {
      "or": { "id__in": [0, 1], "id__null": true },
      "status_id__null": false
    }
    ```
  - Composite fields: join multiple paths with `_or_` / `_and_`, e.g. `org.name_or_org.full_name__cnt` → `(org.name LIKE ...) OR (org.full_name LIKE ...)`.
  - Short aliases: if a model defines `aliases:` (e.g. `org: "contragent.organization"`), you can use the short name in filters/sorts; it is expanded automatically.
  - Computable fields: declared under `computable:` in the model and usable in filters/sorts by name (`fio__cnt`).
- `sorts` — array of strings `["path [ASC|DESC]"]`; supports aliases and computable fields the same way as filters.
- `offset` / `limit` — pagination.

- Success response: HTTP 200 with JSON array of objects.
  ```json
  [
    {
      "id": 1,
      "name": "John Smith",
      "org": { "name": "IBM" }
    }
  ]
  ```
- Error response examples:
  - 400 Bad Request — invalid JSON / unknown model or preset:
    ```json
    { "error": "preset not found: Person.card" }
    ```
  - 500 Internal Server Error — SQL/build issues:
    ```json
    { "error": "Failed to resolve data: ERROR: column t4.fio does not exist (SQLSTATE 42703)" }
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

- Success response: HTTP 200 with `{"count": 123}`.
- Errors mirror `/api/index` (bad request/validation → 400, build/DB errors → 500).

Notes:

- Filters and sorts can traverse relations using dotted paths (`relation.field`), including polymorphic and through relations defined in YAML.
- All path resolution goes through the alias map; invalid paths are logged and ignored.
- Alias maps are cached in-memory; query execution hits PostgreSQL directly.

---

## ⚙️ Service configuration

Configuration is read from environment variables (see `internal/config/config.go`):

| Env var                    | Default                                                           | Description                                          |
| -------------------------- | ----------------------------------------------------------------- | ---------------------------------------------------- |
| `PORT`                     | `8080`                                                            | HTTP port for the API server                         |
| `POSTGRES_DSN`             | `postgres://postgres:postgres@localhost:5432/app?sslmode=disable` | PostgreSQL connection string                         |
| `MODELS_DIR`               | `./db`                                                            | Path to directory with YAML model files              |
| `LOCALE`                   | `en`                                                              | Default locale for localization                      |
| `AUTH_ENABLED`             | `false`                                                           | Enable JWT auth middleware                           |
| `AUTH_JWT_VALIDATION_TYPE` | `HS256`                                                           | JWT signature algorithm: `HS256`/`RS256`/`ES256`     |
| `AUTH_JWT_ISSUER`          | _(empty)_                                                         | Required `iss` claim value                           |
| `AUTH_JWT_AUDIENCE`        | _(empty)_                                                         | Required `aud` claim value (single value or CSV)     |
| `AUTH_JWT_HMAC_SECRET`     | _(empty)_                                                         | Shared secret for `HS256`                            |
| `AUTH_JWT_PUBLIC_KEY`      | _(empty)_                                                         | PEM public key for `RS256`/`ES256`                   |
| `AUTH_JWT_PUBLIC_KEY_PATH` | _(empty)_                                                         | Path to PEM public key for `RS256`/`ES256`           |
| `AUTH_JWT_CLOCK_SKEW_SEC`  | `60`                                                              | Allowed clock skew when validating `exp`/`nbf`/`iat` |
| `CORS_ALLOW_ORIGIN`        | `*`                                                               | Value for `Access-Control-Allow-Origin`              |
| `CORS_ALLOW_CREDENTIALS`   | `false`                                                           | Set `Access-Control-Allow-Credentials: true`         |
| `ALIAS_CACHE_MAX_BYTES`    | `0`                                                               | Max bytes for in-memory alias cache (0 = unlimited)  |

You can provide a `.env` file in the project root; variables from it override defaults. `MODELS_DIR` controls where YAML models are loaded from; adjust it when running in other environments or with mounted configs.

When `AUTH_ENABLED=true`, each API request must include `Authorization: Bearer <token>`. Token validation is fully local: the service checks signature and claims (`iss`, `aud`, `exp`, `nbf`, `iat`) without network calls.

Example `.env` for `HS256`:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=HS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_HMAC_SECRET=replace-with-strong-shared-secret
AUTH_JWT_CLOCK_SKEW_SEC=60
```

You can also pass multiple audiences as a comma-separated list:

```env
AUTH_JWT_AUDIENCE=service-a,service-b
```

Example `.env` for `RS256`:

```env
AUTH_ENABLED=true
AUTH_JWT_VALIDATION_TYPE=RS256
AUTH_JWT_ISSUER=auth-service
AUTH_JWT_AUDIENCE=yrest-api
AUTH_JWT_PUBLIC_KEY_PATH=/etc/yrestapi/keys/auth_public.pem
AUTH_JWT_CLOCK_SKEW_SEC=60
```

---

## 🔄 Import

### Import from DSN

Generate YAML models from a PostgreSQL schema via `POSTGRES_DSN`:

```bash
# help
make import ARGS="-help"

# dry-run: print generated models to stdout
make import ARGS="-dry-run"

# only simple tables (without outgoing FKs)
make import ARGS="-dry-run -only-simple"

# write files to an output directory
make import ARGS="-out ./db_imported"

# explicit DSN (if you do not want to use .env)
make import ARGS="-dsn 'postgres://user:pass@localhost:5432/app?sslmode=disable' -out ./db_imported"

# import from Prisma schema.prisma (no DSN required)
make import ARGS="-prisma-schema ./prisma/schema.prisma -out ./db_imported"

# import presets from GraphQL queries into existing YAML models
make import ARGS="-graphql-queries ./gateway/queries -models-dir ./db -dry-run"
```

Supported SQL import modes:
- `-only-simple` - phase one: tables without outgoing relations.
- without `-only-simple` - imports models with `belongs_to` and reverse `has_many` relations; also adds related `item` presets into `full_info` for `belongs_to`.

After import, each generated `has_many` relation gets a helper preset `with_<relation>` with one field:

```yaml
presets:
  with_project_members:
    fields:
      - source: project_members
        type: preset
        preset: item
```

This makes post-import setup simple: keep `full_info` as a base and extend it via `extends` with any generated `has_many` preset, or with all of them when all multiplicity relations are needed in output.

### Import from Prisma schema

- Pass `-prisma-schema <path>` to read models from `schema.prisma` instead of SQL introspection.
- In this mode `-dsn` is optional and not used.
- Generated output keeps the same YAML shape as SQL mode:
  - `belongs_to` relations from `@relation(fields: [...], references: [...])`;
  - reverse `has_many` relations generated automatically;
  - helper presets `with_<relation>` for each `has_many`.
- Prisma `enum` fields are generated as `type: int` with `localize: true` in presets (`item`/`full_info`).
- Enum dictionaries are merged into default locale file `cfg/locales/<LOCALE>.yml` (fallback `cfg/locales/en.yml`) as numeric maps, for example:
  - `role: {0: USER, 1: ADMIN}`

### Import presets from GraphQL queries

- Pass `-graphql-queries <path>` to read GraphQL query documents and update presets in existing YAML models.
- GraphQL import does not create models or relations. It only adds presets to models already present in `-models-dir` (default `./db`).
- Model lookup uses the convention `root field -> model name`, for example `user -> User`, `project_member -> ProjectMember`.
- Preset names are generated automatically from `root field + operation name + shape hash`, so the same query shape yields the same preset name.
- Nested GraphQL selections are imported only when the corresponding relation already exists in YAML; otherwise the importer leaves a warning and skips that nested branch.
- `source` is taken directly from the GraphQL field name. If the real DB/YAML field name differs, adjust `source` manually after import.

---

## 🏗️ How the engine works

- At startup the service loads all `.yml` model files from `MODELS_DIR`, builds a registry of models, relations, presets, computable fields, aliases, and validates the graph. This registry is kept in memory and reused for all requests; database connections come from the pgx pool.
- Validation checks:
  - All referenced models/relations/presets exist.
  - Relations have valid types (`has_one/has_many/belongs_to`), FK/PK defaults are applied, `through` chains are consistent.
  - Polymorphic `belongs_to` are allowed only with `polymorphic: true`.
  - Preset fields: `type: preset` must reference an existing relation and nested preset; `type: formatter` must have an alias; `type: computable` must exist under `computable`.
  - YAML keys are validated; unknown keys/types raise errors on startup.
- If validation fails, the service logs the reason and aborts startup; fix the YAML and restart.

### Model YAML structure (critical nodes)

```yaml
table: people # required: DB table name
aliases: # optional: short paths → full relation paths
  org: "contragent.organization"
computable: # optional: global computed fields
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
relations: # required if presets reference other models
  person_name:
    model: PersonName
    type: has_one # has_one / has_many / belongs_to
    where: .used = true # optional; leading dot is replaced by SQL alias
presets: # at least one preset to serve data
  card:
    fields:
      - source: id
        type: int
      - source: person_name
        type: preset
        preset: item
      - source: "fio" # computable usage
        type: computable
        alias: full_name
```

Key points:

- `table` is mandatory; `relations` define the graph (with optional `through`, `where`, `order`, `polymorphic`).
- `presets` describe which fields to select/return; `type: preset` walks relations, `type: computable` inserts expressions, `type: formatter` post-processes values, `type: nested_field` copies nested JSON branches.
- `computable` and `aliases` are global per model and can be used in any preset, filter, or sort.
- Validation occurs once at startup; malformed configs prevent the server from running, ensuring bad schemas don’t reach production.
- **Design principle:** presets are intentionally _client-shaped_. The engine is optimized to return the smallest JSON needed for client-form, and nothing more. This means there is no “one true” preset naming scheme; the best practice is to define presets that _exactly_ match frontend requirements, even if they differ between projects or screens.

### Recursive/self relations and depth limits

Self-links are supported (for example `Contract -> next -> Contract`), but recursion must be explicitly controlled.

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

- `reentrant: true` is required for returning to an already visited model in the preset graph.
- `max_depth` limits how many times the same model may appear on one traversal path.
- You can set `max_depth` on the relation and, if needed, override it on a specific preset field (`field.max_depth` has priority).
- If `reentrant: false`, startup validation fails with a clear error on cyclic re-entry.
- If `max_depth` is exceeded, traversal is capped at that depth (no startup failure).
- If `max_depth` is omitted for a reentrant cycle, default `max_depth=3` is applied and logged as a warning.

Why it matters:

- Protects the service from unbounded recursion.
- Keeps SQL planning and JSON shaping predictable for recursive data structures.

---

## 🌐 Localization of strings and constants

- Dictionaries live in `cfg/locales/<locale>.yml`; the active locale is loaded into a tree structure.
- Date/time formats can be customized per locale via `layoutSettings`:
  ```yaml
  layoutSettings:
    date: "02.01.2006"
    ttime: "15:04:05"
    datetime: "02.01.2006 15:04:05"
  ```
  These layouts are used when `localize: true` is set on fields with `type: date`, `time`, or `datetime`.
- Lookup order: `model → preset → field`, then falls back to a global preset and a global field; if nothing is found, the original value is returned.
- To localize a field set `localize: true`; for numeric codes set `type: int` — numeric keys are normalized (int/int64/uint32, etc.) and matched in the dictionary as numbers.
- Sample dictionary:
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
- Sample fields:
  ```yaml
  fields:
    - source: status
      type: int
      localize: true # numeric codes from DB map to strings from the dictionary
    - source: gender
      type: string
      localize: true
  ```

---

## 🔀 Polymorphic relations

- Declare a polymorphic `belongs_to` by setting `polymorphic: true` and `model: "*"`. Example (`db/Audit.yml`):
  ```yaml
  relations:
    auditable:
      model: "*"
      type: belongs_to
      polymorphic: true
  ```
- The parent table must have `<relation>_id` and `<relation>_type` columns (override type column with `type_column` if needed).
- Resolver batches child queries by type: for each distinct `<relation>_type` it issues one query to the target model using the provided nested preset.
- In presets, refer to the polymorphic relation like a normal `preset` field; nested preset name must exist on each target model.

---

## 🧱 Reusing templates with `include` and skipping fields

- You can pull relations/presets from template files in `db/templates/*.yml` via:
  ```yaml
  include: shared_relations # or [shared_relations, auditable]
  ```
- Relations from templates are added if missing; if a relation exists in the model, empty fields are filled from the template.
- Presets from templates merge with model presets: template fields are applied first, then model fields override/extend by alias/source. Fields marked with `alias: skip` in templates are ignored.

---

## 1. Syntax of `where`, `through_where`

- A leading **`.` (dot)** in a condition is replaced with the **unique SQL alias** of that relation.

- **YAML example:**
  ```yaml
  relations:
    phone:
      model: Contact
      type: has_one
      through: PersonContact
      where: .type = 'Phone'
      through_where: .used = true
  ```
- **SQL result:**

  ```sql
  LEFT JOIN person_contacts AS pc
  ON (main.id = pc.person_id)
  AND (pc.used = true)

  LEFT JOIN contacts AS c
  ON (pc.contact_id = c.id)
  AND (c.type = 'Phone')
  ```

- **Purpose:**

  **where** — filters for the final relation table.  
  **through_where** — filters for the intermediate table in through-relations.

---

## 2. Formatter — post-processing of preset fields

Formatters transform or combine field values **after** the SQL query and after merging related data.  
They are useful when you want to **collapse a nested preset into a string** or build a computed text field.  
Formatters are a mini-language for computed fields, allowing value composition, character slicing, and conditionals.

---

### Syntax

#### 1. Inline computed field

```yaml
- source: "{surname} {name}[0] {patronymic}[0..1]"
  type: formatter
  alias: full_name
```

#### 2. Formatter for a relation (preset)

```yaml
- source: contacts
  type: preset
  alias: phones
  formatter: "{type}: {value}"
  preset: phone_list
```

---

### Token rules

Inside `{ ... }` you can use:

- **Fields**: `{field}`
- **Nested fields**: `{relation.field}`
- **Character ranges**:  
  `{name}[0]` → first character  
  `{name}[0..1]` → first two characters

---

### Behaviour by relation type

| Relation type | Result of formatter              |
| ------------- | -------------------------------- |
| `belongs_to`  | String from related object       |
| `has_one`     | String from child object         |
| `has_many`    | Array of strings (one per child) |
| Simple field  | String from current row          |

---

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

**Result:**

```json
[
  {
    "id": 64,
    "name": "Ivanov A V",
    "contacts": ["Phone: +7 923 331 49 55", "Email: example@mail.com"]
  }
]
```

#### 3. Ternary operators

**Syntax:**

```yaml
{ <condition> ? <then>: <else> }
```

**Condition forms:**

- Full form: <field> <op> <value>
- Supported operators: ==, =, !=, >, >=, <, <=.

- Shorthand form: just <field> → evaluates truthy/falsy.

- Supported literals:
  - Numbers: **10**, **3.14**

  - Booleans: **true**, **false**

  - Null: **null**

  - Strings: **"ok"**, **'fail'**

**Examples:**

```yaml
- source: `{? used ? "+" : "-"}`
  type: formatter
  alias: used_flag
# true  → "+"
# false → "-"

- source: `{? age >= 18 ? "adult" : "minor"}`
  type: formatter
  alias: age_group
# age=20 → "adult"
# age=15 → "minor"

- source: `{? status == "ok" ? "✔" : "✖"}`
  type: formatter
  alias: status_icon
```

### Nested ternaries

Ternary expressions can be nested:

```yaml
- source: `{? used ? "{? age >= 18 ? "adult" : "minor"}" : "-"}`
  type: formatter
  alias: nested_example
# used=false        → "-"
# used=true, age=20 → "adult"
# used=true, age=15 → "minor"
```

### Combining with substitutions

Formatters can combine conditional logic and substitutions:

```yaml
- source: '{? used ? "+" : "-"} {naming.surname} {naming.name}[0].'
  type: formatter
  alias: short_name
# used=true  → "+ Ivanov I."
# used=false → "- Ivanov I."
```

### 📌 Notes

- Fields with `type: formatter` must always define an alias.
- Formatter fields are not included in SQL queries. They are resolved only at the post-processing stage.

---

## 🧭 Nested fields (copying nested data up)

- Use `type: nested_field` with a path in `{...}` to lift nested data into the current preset without SQL joins.
- Example:
  ```yaml
  - source: "{person.contacts}"
    type: nested_field
    alias: contacts
  ```
  The `contacts` array from the nested `person` branch will be copied to the current item, even if `person` itself is not exposed in the response.
- Works for arrays or scalars; alias is optional (defaults to the source path).

---

---

## 🧮 Computable (virtual) fields

Calculated fields are declared at the model level and are available in all presets. They allow you to use subqueries or expressions like regular columns: in selections, filters, sorts, and formatters.

```yaml
computable:
  fio:
    source: "(select concat({surname}, ' ', {name}, ' ', {patrname}))"
    type: string
  stages_sum:
    source: "(select sum({stages}.amount))" # {relation}.col → the alias of the connection will be substituted
    where: "(select sum({stages}.amount))" # optional: separate expression for filters/sorts
    type: float
presets:
  card:
    fields:
      - source: "fio"
        alias: "full_name"
        type: computable
      - source: "stages_sum"
        type: computable
```

Rules:

- Placeholders of the type `{path}` are replaced with SQL aliases of links from alias map. `{relation}.col` will turn into `tN.col'.
- Put parentheses in subqueries so that you can safely include in SELECT: `(select ...)` → `... AS "alias"`.
- For use in filters/sorts, it is enough to refer to the name computable (`fio__cnt`, `stages_sum DESC`) — the engine will substitute the expression.

---

## 🧱 DRY for config files: inheritance, templates, nested fields

### Multiple preset inheritance

You can inherit from multiple presets using a comma-separated list:

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

### Reusing templates with `include` and skipping fields

- Pull relations/presets from `db/templates/*.yml` via `include: shared` (or list). Model overrides/fills template fields; empty relation fields (type/fk/etc.) are filled from the template.
- Template preset fields are applied first, then model fields override/extend by alias/source. Fields marked with `alias: skip` in templates are ignored.

### Nested fields (copying nested data up)

- Use `type: nested_field` with a path in `{...}` to lift nested data into the current preset without SQL joins.
- Example:
  ```yaml
  - source: "{person.contacts}"
    type: nested_field
    alias: contacts
  ```
  The `contacts` array from the nested `person` branch will be copied to the current item, even if `person` itself is not exposed in the response.
- Works for arrays or scalars; alias is optional (defaults to the source path).

---

## 📄 License

YrestAPI is licensed under the GNU General Public License v3.0 or later (GPL-3.0-or-later).
See [LICENSE.txt](LICENSE.txt) for the full text.

---

## Known limitations

- The service is read-only by design: only `/api/index` and `/api/count` are provided.
- PostgreSQL is the only supported database backend.
- Model configuration is loaded and validated on startup; changing YAML files requires service restart.
- Polymorphic relation resolution is local and based on `<relation>_type` values present in data; invalid type values are skipped at runtime.
- Integration tests are safety-scoped to local PostgreSQL hosts (`localhost` / `127.0.0.1`) and create/drop a temporary `test` database.
