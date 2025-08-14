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
  ## Formatter — post-processing of preset fields

Formatters transform or combine field values **after** the SQL query and after merging related data.  
They are useful when you want to **collapse a nested preset into a string** or build a computed text field.

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
