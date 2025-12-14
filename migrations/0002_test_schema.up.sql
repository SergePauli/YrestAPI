-- === Справочники (примитивы) ===
CREATE TABLE IF NOT EXISTS countries (
  id          SERIAL PRIMARY KEY,
  code        TEXT NOT NULL UNIQUE, -- 'PH', 'RU'...
  name        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS areas (
  id          SERIAL PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE
);

-- === Базовые сущности ===
CREATE TABLE IF NOT EXISTS organizations (
  id          SERIAL PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  country_id  INT REFERENCES countries(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS people (
  id          SERIAL PRIMARY KEY,
  first_name  TEXT NOT NULL,
  last_name   TEXT NOT NULL
);

-- 1:1 — уникальная связка с person
CREATE TABLE IF NOT EXISTS person_profiles (
  person_id   INT PRIMARY KEY REFERENCES people(id) ON DELETE CASCADE,
  bio         TEXT,
  birthday    DATE
);

-- Самоссылка (дерево подразделений)
CREATE TABLE IF NOT EXISTS departments (
  id            SERIAL PRIMARY KEY,
  organization_id INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name          TEXT NOT NULL,
  parent_id     INT REFERENCES departments(id) ON DELETE SET NULL,
  UNIQUE (organization_id, name)
);

-- 1:N — сотрудник принадлежит организации и (опционально) отделу
CREATE TABLE IF NOT EXISTS employees (
  id              SERIAL PRIMARY KEY,
  organization_id INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  department_id   INT REFERENCES departments(id) ON DELETE SET NULL,
  person_id       INT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  position        TEXT NOT NULL,
  hired_at        DATE NOT NULL,
  -- один person не может быть дважды нанят в одну и ту же организацию
  UNIQUE (organization_id, person_id)
);

-- Контакты как отдельная сущность
CREATE TABLE IF NOT EXISTS contacts (
  id          SERIAL PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('email','phone','telegram')),
  value       TEXT NOT NULL,
  UNIQUE (kind, value)
);

-- N:N “через” (through) — связи person↔contact
CREATE TABLE IF NOT EXISTS person_contacts (
  person_id   INT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  contact_id  INT NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  is_primary  BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (person_id, contact_id)
);

-- Адреса + связь контрагента/организации с адресами (N:N через)
CREATE TABLE IF NOT EXISTS addresses (
  id          SERIAL PRIMARY KEY,
  line1       TEXT NOT NULL,
  area_id     INT NOT NULL REFERENCES areas(id) ON DELETE RESTRICT,
  postal_code TEXT
);

-- Пусть “contragents” из твоего домена выступят как проверка join-ов
CREATE TABLE IF NOT EXISTS contragents (
  id          SERIAL PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE
);

-- Организации контрагентов (для теста formatters/presets)
CREATE TABLE IF NOT EXISTS contragent_organizations (
  id             SERIAL PRIMARY KEY,
  contragent_id  INT NOT NULL REFERENCES contragents(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  used           BOOLEAN NOT NULL DEFAULT true,
  created_at     TIMESTAMP DEFAULT now()
);

CREATE TABLE IF NOT EXISTS contragent_addresses (
  contragent_id INT NOT NULL REFERENCES contragents(id) ON DELETE CASCADE,
  address_id    INT NOT NULL REFERENCES addresses(id) ON DELETE CASCADE,
  PRIMARY KEY (contragent_id, address_id)
);

-- Ещё один N:N с дополнительными полями и составным PK
CREATE TABLE IF NOT EXISTS projects (
  id           SERIAL PRIMARY KEY,
  organization_id INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS project_members (
  project_id   INT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  person_id    INT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  role         TEXT NOT NULL,
  joined_at    DATE NOT NULL,
  PRIMARY KEY (project_id, person_id)
);

-- Индексы для типичных запросов/сортировок
CREATE INDEX IF NOT EXISTS idx_employees_org ON employees(organization_id);
CREATE INDEX IF NOT EXISTS idx_departments_parent ON departments(parent_id);
CREATE INDEX IF NOT EXISTS idx_addresses_area ON addresses(area_id);
CREATE INDEX IF NOT EXISTS idx_person_contacts_primary ON person_contacts(person_id) WHERE is_primary;
