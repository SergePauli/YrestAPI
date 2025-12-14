TRUNCATE TABLE project_members, projects,
  contragent_addresses, contragent_organizations, contragents,
  addresses,
  person_contacts, contacts,
  employees, departments,
  person_profiles, people,
  organizations,
  areas, countries
RESTART IDENTITY CASCADE;
