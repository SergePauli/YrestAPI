-- Страны
INSERT INTO countries (id, code, name) VALUES
  (1,'PH','Philippines'),
  (2,'RU','Russia')
ON CONFLICT DO NOTHING;

-- Регионы/Areas
INSERT INTO areas (id, name) VALUES
  (1,'Metro Manila'),
  (2,'Moscow Oblast')
ON CONFLICT DO NOTHING;

-- Организации
INSERT INTO organizations (id, name, country_id) VALUES
  (1,'Acme Corp', 1),
  (2,'Globex LLC', 2)
ON CONFLICT DO NOTHING;

-- Люди
INSERT INTO people (id, first_name, last_name) VALUES
  (1,'Serge','Ivankov'),
  (2,'Mira','Tan'),
  (3,'Alex','Chen')
ON CONFLICT DO NOTHING;

-- 1:1 профили
INSERT INTO person_profiles (person_id, bio, birthday) VALUES
  (1,'Go/JS/Ruby dev','1990-01-01'),
  (2,'Data wrangler','1992-05-10')
ON CONFLICT DO NOTHING;

-- Департаменты (самоссылка)
INSERT INTO departments (id, organization_id, name, parent_id) VALUES
  (10,1,'Engineering', NULL),
  (11,1,'Platform', 10),
  (12,1,'Frontend', 10),
  (20,2,'R&D', NULL)
ON CONFLICT DO NOTHING;

-- Сотрудники (1→N)
INSERT INTO employees (id, organization_id, department_id, person_id, position, hired_at) VALUES
  (100,1,11,1,'Senior Backend Engineer','2022-02-02'),
  (101,1,12,3,'Frontend Engineer','2023-03-03'),
  (200,2,20,2,'Researcher','2021-01-15')
ON CONFLICT DO NOTHING;

-- Контакты
INSERT INTO contacts (id, kind, value) VALUES
  (1000,'email','serge@example.com'),
  (1001,'phone','+639171234567'),
  (1002,'telegram','@mira_tan'),
  (1003,'email','alex@example.com')
ON CONFLICT DO NOTHING;

-- N:N person↔contact
INSERT INTO person_contacts (person_id, contact_id, is_primary) VALUES
  (1,1000,true),
  (1,1001,false),
  (2,1002,true),
  (3,1003,true)
ON CONFLICT DO NOTHING;

-- Адреса и контрагенты
INSERT INTO addresses (id, line1, area_id, postal_code) VALUES
  (500,'123 Ayala Ave',1,'1226'),
  (501,'Kutuzovsky Prospekt, 10',2,'121248')
ON CONFLICT DO NOTHING;

INSERT INTO contragents (id, name) VALUES
  (300,'Innotech JSC'),
  (301,'Pacific Trading')
ON CONFLICT DO NOTHING;

-- Организации контрагентов (используются в пресете Contragent.head)
INSERT INTO contragent_organizations (id, contragent_id, name, used) VALUES
  (1,300,'Innotech JSC HQ', true),
  (2,300,'Innotech JSC Alt', false), -- unused
  (3,301,'Pacific Trading LLC', true)
ON CONFLICT DO NOTHING;

-- N:N contragent↔address (через)
INSERT INTO contragent_addresses (contragent_id, address_id) VALUES
  (300,501),
  (301,500)
ON CONFLICT DO NOTHING;

-- Проекты и участники (N:N с данными)
INSERT INTO projects (id, organization_id, name) VALUES
  (900,1,'Data Lake'),
  (901,1,'UI Overhaul'),
  (902,2,'NLP Research')
ON CONFLICT DO NOTHING;

INSERT INTO project_members (project_id, person_id, role, joined_at) VALUES
  (900,1,'Lead','2024-01-01'),
  (900,3,'Contributor','2024-02-01'),
  (901,3,'Owner','2024-05-20'),
  (902,2,'Researcher','2023-10-10')
ON CONFLICT DO NOTHING;
