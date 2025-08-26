# YrestAPI


**YrestAPI** — это декларативный REST API движок на Go, построенный поверх PostgreSQL, с поддержкой Redis-кэширования и параллельной загрузки `has_`-связей.  
Полностью конфигурируется через YAML — без единой строки бизнес-логики в коде.

---

## 🔧 Особенности

- 📁 **Декларативная настройка через YAML** — модели, связи, пресеты
- ⚡ **Высокая производительность** — благодаря Go и конкурентной обработке
- 🚀 **Кэширование с Redis** — ускорение вложенных и повторяющихся запросов
- 🔁 **Поддержка `has_many`, `has_one`, `belongs_to`, `through`**
- 🧩 **Вложенные пресеты** — JSON-структура любых уровней вложенности
- 🔎 **Фильтрация, сортировка, пагинация**
- 🛠️ **Форматирование полей** — с помощью шаблонов в YAML
- 🔐 **Готово к продакшену** — без фреймворков, только `Go`, `pgx`, `Redis`

---

## 🧩 Для каких случаев подходит YrestAPI

YrestAPI будет особенно полезен в следующих ситуациях:

- 🗃 **У вас уже есть PostgreSQL-база данных**, и нужно **быстро развернуть JSON API** без написания серверной логики.
- ⚙️ **Нужен микросервис только для чтения (`index`/`list`) данных**, с вложенными связями и фильтрацией.
- 🐢 **Существующий API на Python, Ruby, Node.js** работает медленно для сложных выборок, и вы хотите ускорить только часть `index`-операций.
- 🧪 **Быстрая разработка прототипов**, где нет времени писать SQL/ORM-код.
- 🧵 **Разделение слоёв**: YrestAPI может быть частью гибридной архитектуры, отдавая данные, которые затем обрабатываются фронтендом или BFF.

---

## ⚙️ Быстрый старт

```bash
git clone https://github.com/your-org/yrestapi
cd yrestapi
go run main.go
```
---

## 1. Синтаксис `where`, `through_where`

- **`.` (точка)** в начале условия заменяется на **уникальный алиас** соответствующей связи в SQL.

- **Пример YAML:**
  ```yaml
  relations:
    phone:
      model: Contact
      type: has_one
      through: PersonContact
      where: .type = 'Phone'
      through_where: .used = true
  ```
- **Результат SQL:**
  ```sql
  LEFT JOIN person_contacts AS pc 
  ON (main.id = pc.person_id) 
  AND (pc.used = true)

  LEFT JOIN contacts AS c 
  ON (pc.contact_id = c.id) 
  AND (c.type = 'Phone')
  ```
- **Назначение:**

  **where** — фильтры для конечной таблицы связи.
  **through_where** — фильтры для промежуточной таблицы при through-связях.
  ---
## 2. Formatter — post-processing of preset fields

Formatters transform or combine field values **after** the SQL query and after merging related data.  
They are useful when you want to **collapse a nested preset into a string** or build a computed text field.
Formatters provide a mini-language for building computed fields inside presets.  
They allow you to combine values from multiple fields, apply character slicing, and add conditional logic.

---

###  Syntax

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

###  Token rules
Inside `{ ... }` you can use:
- **Fields**: `{field}`
- **Nested fields**: `{relation.field}`
- **Character ranges**:  
  `{name}[0]` → first character  
  `{name}[0..1]` → first two characters

---

###  Behaviour by relation type
| Relation type  | Result of formatter |
|----------------|--------------------|
| `belongs_to`   | String from related object |
| `has_one`      | String from child object |
| `has_many`     | Array of strings (one per child) |
| Simple field   | String from current row |

---

###  Example
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
    "name": "Иванов А В",
    "contacts": [
      "Phone: +7 923 331 49 55",
      "Email: example@mail.com"
    ]
  }
]
```
#### 3.Ternary operators
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
### 📌 Notes:

  - Fields with type: formatter must always define an alias.

  - Formatter fields are not included in SQL queries. They are resolved only at the     post-processing stage.
---  
