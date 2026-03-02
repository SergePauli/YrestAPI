package sqlimport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

type Column struct {
	Name       string
	DataType   string
	UDTName    string
	OrdinalPos int
}

type ModelFile struct {
	FileName string
	Content  []byte
}

type ForeignKey struct {
	ColumnName    string
	RefTable      string
	RefColumnName string
}

type GenerateOptions struct {
	Schema     string
	OnlySimple bool
}

type modelYAML struct {
	Table     string                  `yaml:"table"`
	Relations map[string]relationYAML `yaml:"relations,omitempty"`
	Presets   map[string]presetYAML   `yaml:"presets"`
}

type presetYAML struct {
	Fields []fieldYAML `yaml:"fields"`
}

type relationYAML struct {
	Type  string `yaml:"type"`
	Model string `yaml:"model"`
	FK    string `yaml:"fk,omitempty"`
	PK    string `yaml:"pk,omitempty"`
}

type fieldYAML struct {
	Source string `yaml:"source"`
	Type   string `yaml:"type"`
	Alias  string `yaml:"alias,omitempty"`
	Preset string `yaml:"preset,omitempty"`
}

func Generate(ctx context.Context, pool *pgxpool.Pool, opts GenerateOptions) ([]ModelFile, error) {
	schema := strings.TrimSpace(opts.Schema)
	if schema == "" {
		schema = "public"
	}

	tables, err := listTables(ctx, pool, schema, opts.OnlySimple)
	if err != nil {
		return nil, err
	}
	out := make([]ModelFile, 0, len(tables))
	for _, table := range tables {
		cols, err := listColumns(ctx, pool, schema, table)
		if err != nil {
			return nil, fmt.Errorf("read columns for %s: %w", table, err)
		}
		if len(cols) == 0 {
			continue
		}
		fks, err := listForeignKeys(ctx, pool, schema, table)
		if err != nil {
			return nil, fmt.Errorf("read foreign keys for %s: %w", table, err)
		}

		pkCols, err := listPrimaryKeyColumns(ctx, pool, schema, table)
		if err != nil {
			return nil, fmt.Errorf("read primary key for %s: %w", table, err)
		}

		idSource, idType, ok := chooseIDColumn(cols, pkCols)
		if !ok {
			// MVP scope: skip tables where we cannot determine ID column.
			continue
		}
		nameCol := chooseNameLikeColumn(cols, idSource)

		itemFields := []fieldYAML{{Source: idSource, Type: idType}}
		if idSource != "id" {
			itemFields[0].Alias = "id"
		}
		if nameCol != "" {
			itemFields = append(itemFields, fieldYAML{
				Source: nameCol,
				Type:   mapColumnType(findColumn(cols, nameCol)),
			})
		}

		presets := map[string]presetYAML{
			"item": {Fields: itemFields},
		}
		relations := map[string]relationYAML{}

		// If more than 2 columns (non-trivial table), also add full_info with all columns.
		if len(cols) > 2 {
			fullFields := make([]fieldYAML, 0, len(cols))
			for _, c := range cols {
				fullFields = append(fullFields, fieldYAML{
					Source: c.Name,
					Type:   mapColumnType(c),
				})
			}
			for _, fk := range fks {
				relName := relationNameFromFK(fk.ColumnName, fk.RefTable)
				relModel := tableToModelName(fk.RefTable)
				relations[relName] = relationYAML{
					Type:  "belongs_to",
					Model: relModel,
					FK:    fk.ColumnName,
					PK:    fk.RefColumnName,
				}
				fullFields = append(fullFields, fieldYAML{
					Source: relName,
					Type:   "preset",
					Preset: "item",
				})
			}
			presets["full_info"] = presetYAML{Fields: fullFields}
		} else {
			for _, fk := range fks {
				relName := relationNameFromFK(fk.ColumnName, fk.RefTable)
				relModel := tableToModelName(fk.RefTable)
				relations[relName] = relationYAML{
					Type:  "belongs_to",
					Model: relModel,
					FK:    fk.ColumnName,
					PK:    fk.RefColumnName,
				}
			}
		}

		model := modelYAML{
			Table:     table,
			Relations: relations,
			Presets:   presets,
		}
		raw, err := yaml.Marshal(&model)
		if err != nil {
			return nil, fmt.Errorf("marshal model %s: %w", table, err)
		}

		modelName := tableToModelName(table)
		out = append(out, ModelFile{
			FileName: modelName + ".yml",
			Content:  raw,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	return out, nil
}

func WriteFiles(dir string, files []ModelFile) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(dir, f.FileName)
		if err := os.WriteFile(path, f.Content, 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func listTables(ctx context.Context, pool *pgxpool.Pool, schema string, onlySimple bool) ([]string, error) {
	q := buildListTablesQuery(onlySimple)
	rows, err := pool.Query(ctx, q, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		out = append(out, table)
	}
	return out, rows.Err()
}

func buildListTablesQuery(onlySimple bool) string {
	q := `
SELECT t.table_name
FROM information_schema.tables t
WHERE t.table_schema = $1
  AND t.table_type = 'BASE TABLE'
  AND t.table_name NOT IN ('schema_migrations', 'ar_internal_metadata')
`
	if onlySimple {
		q += `
  AND NOT EXISTS (
    SELECT 1
    FROM information_schema.table_constraints tc
    WHERE tc.table_schema = t.table_schema
      AND tc.table_name = t.table_name
      AND tc.constraint_type = 'FOREIGN KEY'
  )
`
	}
	q += "ORDER BY t.table_name;"
	return q
}

func listForeignKeys(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]ForeignKey, error) {
	const q = `
SELECT
  kcu.column_name,
  ccu.table_name AS foreign_table_name,
  ccu.column_name AS foreign_column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name
 AND ccu.constraint_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND tc.table_schema = $1
  AND tc.table_name = $2
ORDER BY kcu.ordinal_position;
`
	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ForeignKey
	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.ColumnName, &fk.RefTable, &fk.RefColumnName); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

func listColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]Column, error) {
	const q = `
SELECT column_name, data_type, udt_name, ordinal_position
FROM information_schema.columns
WHERE table_schema = $1
  AND table_name = $2
ORDER BY ordinal_position;
`
	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Column
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Name, &c.DataType, &c.UDTName, &c.OrdinalPos); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func listPrimaryKeyColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]string, error) {
	const q = `
SELECT kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name
 AND tc.table_schema = kcu.table_schema
WHERE tc.table_schema = $1
  AND tc.table_name = $2
  AND tc.constraint_type = 'PRIMARY KEY'
ORDER BY kcu.ordinal_position;
`
	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func chooseIDColumn(cols []Column, pkCols []string) (source, yamlType string, ok bool) {
	if c := findColumn(cols, "id"); c.Name != "" {
		return "id", mapColumnType(c), true
	}
	if len(pkCols) == 1 {
		if c := findColumn(cols, pkCols[0]); c.Name != "" {
			return c.Name, mapColumnType(c), true
		}
	}
	return "", "", false
}

func chooseNameLikeColumn(cols []Column, idSource string) string {
	best := ""
	bestScore := -1
	for _, c := range cols {
		if c.Name == idSource {
			continue
		}
		s := scoreNameColumn(c)
		if s > bestScore {
			bestScore = s
			best = c.Name
		}
	}
	if bestScore < 0 {
		return ""
	}
	return best
}

func scoreNameColumn(c Column) int {
	name := strings.ToLower(c.Name)
	score := 0

	switch name {
	case "name":
		score += 100
	case "full_name":
		score += 95
	case "short_name":
		score += 92
	case "title":
		score += 90
	case "label":
		score += 88
	case "display_name":
		score += 87
	}

	if strings.Contains(name, "name") {
		score += 70
	}
	if strings.Contains(name, "title") || strings.Contains(name, "label") {
		score += 50
	}
	if strings.Contains(name, "code") || strings.Contains(name, "number") {
		score += 30
	}

	if isTextual(c) {
		score += 25
	} else {
		score += 5
	}
	return score
}

func isTextual(c Column) bool {
	switch strings.ToLower(c.DataType) {
	case "character varying", "character", "text":
		return true
	default:
		return false
	}
}

func findColumn(cols []Column, name string) Column {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return Column{}
}

func mapColumnType(c Column) string {
	dt := strings.ToLower(c.DataType)
	udt := strings.ToLower(c.UDTName)
	switch {
	case dt == "smallint" || dt == "integer" || dt == "bigint":
		return "int"
	case dt == "numeric" || dt == "decimal" || dt == "real" || dt == "double precision":
		return "float"
	case dt == "boolean":
		return "bool"
	case dt == "date":
		return "date"
	case dt == "time without time zone" || dt == "time with time zone":
		return "time"
	case dt == "timestamp without time zone" || dt == "timestamp with time zone":
		return "datetime"
	case dt == "uuid" || udt == "uuid":
		return "UUID"
	default:
		return "string"
	}
}

func tableToModelName(table string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(table)), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		p = singularize(p)
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func singularize(s string) string {
	irregular := map[string]string{
		"people":   "person",
		"statuses": "status",
	}
	if v, ok := irregular[s]; ok {
		return v
	}

	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "sses") || strings.HasSuffix(s, "shes") ||
		strings.HasSuffix(s, "ches") || strings.HasSuffix(s, "xes") || strings.HasSuffix(s, "zes") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return s[:len(s)-1]
	}
	return s
}

func relationNameFromFK(columnName, refTable string) string {
	name := strings.TrimSpace(strings.ToLower(columnName))
	if strings.HasSuffix(name, "_id") && len(name) > 3 {
		name = name[:len(name)-3]
	}
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}

	parts := strings.Split(strings.ToLower(strings.TrimSpace(refTable)), "_")
	for i, p := range parts {
		parts[i] = singularize(p)
	}
	return strings.Join(parts, "_")
}
