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

---

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
      localize: true  # numeric codes from DB map to strings from the dictionary
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
| Relation type  | Result of formatter |
|----------------|--------------------|
| `belongs_to`   | String from related object |
| `has_one`      | String from child object |
| `has_many`     | Array of strings (one per child) |
| Simple field   | String from current row |

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
    "contacts": [
      "Phone: +7 923 331 49 55",
      "Email: example@mail.com"
    ]
  }
]
```

#### 3. Ternary operators
**Syntax:**
```yaml
  {? <condition> ? <then> : <else>}
```
**Condition forms:**

-  Full form: <field> <op> <value>
-  Supported operators: ==, =, !=, >, >=, <, <=.

-  Shorthand form: just <field> → evaluates truthy/falsy.

-  Supported literals:

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
