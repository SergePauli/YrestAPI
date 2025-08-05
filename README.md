# YrestAPI



**YrestAPI** — это декларативный REST API движок на Go, построенный поверх PostgreSQL, с поддержкой Redis-кэширования и параллельной загрузки `has_many`-связей.  

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



## 📦 Структура конфигурации (YAML)



```yaml

\# models/person.yml

table: people

relations:

&nbsp; person\_name:

&nbsp;   model: PersonName

&nbsp;   type: has\_one

&nbsp;   where: person\_names.used = true

&nbsp;   order: person\_names.id desc

&nbsp; contacts:

&nbsp;   model: Contact

&nbsp;   type: has\_many

presets:

&nbsp; - name: card

&nbsp;   fields:

&nbsp;     - alias: id

&nbsp;       source: people.id

&nbsp;       type: int

&nbsp;     - alias: name

&nbsp;       preset: PersonName.short

&nbsp;       type: preset

&nbsp;     - alias: email

&nbsp;       preset: Contact.email

&nbsp;       type: preset



