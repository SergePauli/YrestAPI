# YrestAPI - declarative read-only REST over PostgreSQL (YAML -> JSON)

> TL;DR: run a fast read-only JSON API for PostgreSQL in minutes.  
> Endpoints: `/api/index` and `/api/count`. Contract defined in YAML.

**YrestAPI** is a declarative REST engine in Go for read-heavy PostgreSQL APIs.  
You describe models, relations, and response shapes in YAML, and YrestAPI serves JSON without ORM code or custom read handlers.

Versioning: this project publicly follows [Semantic Versioning 2.0.0](VERSIONING.md).

## Why

- Offload heavy list endpoints from your main API (`PHP`, `Ruby`, `Node`, etc.).
- Remove ORM and server-side business code from read-only API paths.
- Serve nested JSON with filters, sorts, pagination, and relation traversal.
- Shape response fields after fetch with YAML formatters instead of hand-written view code.
- Localize field values and enum-like codes through locale dictionaries, not controller logic.
- Keep the contract explicit in YAML instead of burying it in code.

## Try It In 60 Seconds

The repository includes a ready-to-run smoke stack: PostgreSQL, schema, seed data, and bundled YAML models.

```bash
# 1) clone
git clone https://github.com/SergePauli/YrestAPI.git
cd YrestAPI

# 2) start
docker compose up --build

# 3) call the API
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Project","preset":"with_members","limit":2,"sorts":["id ASC"]}'
```

Useful checks:

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS -X POST http://localhost:8080/api/index \
  -H 'Content-Type: application/json' \
  -d '{"model":"Contragent","preset":"item","sorts":["id ASC"],"limit":2}'
curl -sS -X POST http://localhost:8080/api/count \
  -H 'Content-Type: application/json' \
  -d '{"model":"Person"}'
```

Notes:

- `compose.yaml` starts PostgreSQL and applies the bundled smoke schema and seed automatically.
- The default compose setup uses `MODELS_DIR=/app/test_db` so the seeded database matches the shipped YAML models.
- If port `8080` is busy, run `HOST_PORT=8081 docker compose up --build`.

## Core Concepts

### Model YAML

Each model is a YAML file that maps a logical API model to a PostgreSQL table and declares relations, aliases, computable fields, and presets.

Example:

```yaml
table: people
aliases:
  org: "contragent.organization"
relations:
  contacts:
    model: Contact
    type: has_many
    through: PersonContact
presets:
  item:
    fields:
      - source: id
        type: int
      - source: contacts
        type: preset
        preset: item
```

### Presets

Presets define the exact JSON shape returned to the client. They are intentionally client-shaped, not table-shaped.

Example:

```yaml
presets:
  card:
    fields:
      - source: id
        type: int
      - source: "{person_name.value} {last_name}[0]."
        type: formatter
        alias: short_label
      - source: org
        type: preset
        preset: short
```

## Contributing

Contributions are welcome! If you find a bug or have a feature request, please open an issue. To contribute code:
1. Fork the repository.
2. Create a feature branch (`git checkout -b feature/amazing-feature`).
3. Commit your changes (`git commit -m 'Add amazing feature'`).
4. Push to the branch (`git push origin feature/amazing-feature`).
5. Open a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.