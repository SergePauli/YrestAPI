# YrestAPI

**YrestAPI** is a declarative REST API engine in Go, built on PostgreSQL with Redis caching and parallel loading of `has_` relations.  
Everything is configured via YAML — no business logic in code.

---

## 🔧 Features

- 📁 **Declarative YAML config** — models, relations, presets
- ⚡ **High performance** — Go + concurrent processing
- 🚀 **Redis caching** — speeds up nested and repeated queries
- 🔁 **`has_many`, `has_one`, `belongs_to`, `through` support**
- 🧩 **Nested presets** — JSON of arbitrary depth
- 🔎 **Filtering, sorting, pagination**
- 🛠️ **Field formatters** — template-based formatting in YAML
- 🔐 **Production-ready** — plain `Go`, `pgx`, `Redis`

## 🧩 When YrestAPI fits

- 🗃 **You already have PostgreSQL** and need to **spin up a JSON API fast** with zero server code.
- ⚙️ **Read-only microservice (`index`/`list`)** with nested relations and filters.
- 🐢 Existing API in Python/Ruby/Node is slow for complex selects and you want to offload `index` operations.
- 🧪 **Rapid prototyping** when there is no time to write SQL/ORM code.
- 🧵 **Layer separation**: YrestAPI can be a data provider consumed by frontend/BFF.

---

## ⚙️ Quick start

```bash
git clone https://github.com/your-org/yrestapi
cd yrestapi
go run main.go
```

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
  - Operators: `__eq` (default), `__cnt` (LIKE %v%), `__start` (LIKE v%), `__end` (%v), `__lt`, `__lte`, `__gt`, `__gte`, `__in`.
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
- Redis is used only for caching alias maps (if enabled); query execution hits PostgreSQL directly.

---

## ⚙️ Service configuration

Configuration is read from environment variables (see `internal/config/config.go`):

| Env var        | Default                                                           | Description                             |
| -------------- | ----------------------------------------------------------------- | --------------------------------------- |
| `PORT`         | `8080`                                                            | HTTP port for the API server            |
| `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/app?sslmode=disable` | PostgreSQL connection string            |
| `REDIS_ADDR`   | `localhost:6379`                                                  | Redis address for alias map caching     |
| `MODELS_DIR`   | `./db`                                                            | Path to directory with YAML model files |
| `LOCALE`       | `en`                                                              | Default locale for localization         |
| `AUTH_ENABLED` | `false`                                                           | Enable JWT auth middleware              |
| `AUTH_JWT_VALIDATION_TYPE` | `HS256`                                                | JWT signature algorithm: `HS256`/`RS256`/`ES256` |
| `AUTH_JWT_ISSUER` | *(empty)*                                                      | Required `iss` claim value              |
| `AUTH_JWT_AUDIENCE` | *(empty)*                                                    | Required `aud` claim value (single value or CSV) |
| `AUTH_JWT_HMAC_SECRET` | *(empty)*                                                  | Shared secret for `HS256`               |
| `AUTH_JWT_PUBLIC_KEY` | *(empty)*                                                   | PEM public key for `RS256`/`ES256`      |
| `AUTH_JWT_PUBLIC_KEY_PATH` | *(empty)*                                              | Path to PEM public key for `RS256`/`ES256` |
| `AUTH_JWT_CLOCK_SKEW_SEC` | `60`                                                    | Allowed clock skew when validating `exp`/`nbf`/`iat` |
| `CORS_ALLOW_ORIGIN` | `*`                                                          | Value for `Access-Control-Allow-Origin` |
| `CORS_ALLOW_CREDENTIALS` | `false`                                               | Set `Access-Control-Allow-Credentials: true` |

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
