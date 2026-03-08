package prismaimport

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ModelFile struct {
	FileName string
	Content  []byte
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

type prismaModel struct {
	Name        string
	Table       string
	Fields      []prismaField
	PKCols      []string
	BelongsTo   []belongsTo
	ListRels    []listRelation
	ScalarNames map[string]bool
}

type prismaField struct {
	Name       string
	Type       string
	BaseType   string
	IsList     bool
	IsOptional bool
	Attrs      string
}

type belongsTo struct {
	RelName      string
	ToModel      string
	FKColumn     string
	PKColumn     string
	RelationName string
}

type listRelation struct {
	Name         string
	ToModel      string
	RelationName string
}

type incomingFK struct {
	FromModel    string
	FKColumn     string
	PKColumn     string
	RelationName string
}

var (
	modelStartRe = regexp.MustCompile(`^\s*model\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{\s*$`)
	mapRe        = regexp.MustCompile(`@@map\("([^"]+)"\)`)
	relationRe   = regexp.MustCompile(`@relation\((.*)\)`)
	fieldsListRe = regexp.MustCompile(`fields\s*:\s*\[([^\]]+)\]`)
	refsListRe   = regexp.MustCompile(`references\s*:\s*\[([^\]]+)\]`)
	modelIDRe    = regexp.MustCompile(`@@id\(\s*\[([^\]]+)\]`)
	relNameKVRe  = regexp.MustCompile(`name\s*:\s*"([^"]+)"`)
)

func GenerateFromFile(path string) ([]ModelFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prisma schema: %w", err)
	}
	return GenerateFromSchema(string(raw))
}

func GenerateFromSchema(schema string) ([]ModelFile, error) {
	models, err := parseModels(schema)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, nil
	}

	incoming := map[string][]incomingFK{}
	for _, m := range models {
		for _, rel := range m.BelongsTo {
			incoming[rel.ToModel] = append(incoming[rel.ToModel], incomingFK{
				FromModel:    m.Name,
				FKColumn:     rel.FKColumn,
				PKColumn:     rel.PKColumn,
				RelationName: rel.RelationName,
			})
		}
	}

	out := make([]ModelFile, 0, len(models))
	for _, m := range models {
		cols := scalarColumns(m)
		idSource, idType, ok := chooseIDColumn(cols, m.PKCols)
		if !ok {
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
				Type:   cols[nameCol],
			})
		}
		presets := map[string]presetYAML{
			"item": {Fields: itemFields},
		}

		relations := map[string]relationYAML{}
		for _, rel := range m.BelongsTo {
			relations[rel.RelName] = relationYAML{
				Type:  "belongs_to",
				Model: rel.ToModel,
				FK:    rel.FKColumn,
				PK:    rel.PKColumn,
			}
		}
		for _, in := range incoming[m.Name] {
			relName := uniqueRelationName(relations, chooseHasManyRelationName(m, in))
			relations[relName] = relationYAML{
				Type:  "has_many",
				Model: in.FromModel,
				FK:    in.FKColumn,
				PK:    in.PKColumn,
			}
		}

		if len(cols) > 2 {
			fullFields := make([]fieldYAML, 0, len(cols)+len(m.BelongsTo))
			colNames := make([]string, 0, len(cols))
			for c := range cols {
				colNames = append(colNames, c)
			}
			sort.Strings(colNames)
			for _, c := range colNames {
				fullFields = append(fullFields, fieldYAML{
					Source: c,
					Type:   cols[c],
				})
			}
			for _, rel := range m.BelongsTo {
				fullFields = append(fullFields, fieldYAML{
					Source: rel.RelName,
					Type:   "preset",
					Preset: "item",
				})
			}
			presets["full_info"] = presetYAML{Fields: fullFields}
		}

		addHasManyRelationPresets(presets, relations)

		y := modelYAML{
			Table:     m.Table,
			Relations: relations,
			Presets:   presets,
		}
		raw, err := yaml.Marshal(&y)
		if err != nil {
			return nil, fmt.Errorf("marshal model %s: %w", m.Name, err)
		}
		out = append(out, ModelFile{
			FileName: m.Name + ".yml",
			Content:  raw,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	return out, nil
}

func parseModels(schema string) ([]prismaModel, error) {
	lines := strings.Split(schema, "\n")
	models := make([]prismaModel, 0)

	inModel := false
	current := prismaModel{}
	for _, raw := range lines {
		line := stripComment(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !inModel {
			m := modelStartRe.FindStringSubmatch(trimmed)
			if len(m) == 2 {
				inModel = true
				current = prismaModel{
					Name:        m[1],
					Table:       m[1],
					ScalarNames: map[string]bool{},
				}
			}
			continue
		}

		if trimmed == "}" {
			models = append(models, current)
			inModel = false
			current = prismaModel{}
			continue
		}

		if strings.HasPrefix(trimmed, "@@") {
			if m := mapRe.FindStringSubmatch(trimmed); len(m) == 2 {
				current.Table = m[1]
			}
			if m := modelIDRe.FindStringSubmatch(trimmed); len(m) == 2 {
				current.PKCols = parseCSVNames(m[1])
			}
			continue
		}

		f, ok := parseField(trimmed)
		if !ok {
			continue
		}
		current.Fields = append(current.Fields, f)
		if isScalarType(f.BaseType) || f.BaseType == "Unsupported" {
			current.ScalarNames[f.Name] = true
			if strings.Contains(f.Attrs, "@id") {
				current.PKCols = append(current.PKCols, f.Name)
			}
			continue
		}
		if rel, ok := parseListRelation(f); ok {
			current.ListRels = append(current.ListRels, rel)
			continue
		}
		rel, ok := parseBelongsToRelation(f, current.Table)
		if ok {
			current.BelongsTo = append(current.BelongsTo, rel)
		}
	}

	if inModel {
		return nil, fmt.Errorf("invalid prisma schema: unclosed model block %q", current.Name)
	}
	return models, nil
}

func parseField(line string) (prismaField, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return prismaField{}, false
	}
	name := strings.TrimSpace(parts[0])
	typ := strings.TrimSpace(parts[1])
	attrs := ""
	if len(parts) > 2 {
		attrs = strings.Join(parts[2:], " ")
	}

	base := typ
	isList := false
	isOptional := false
	if strings.HasSuffix(base, "[]") {
		isList = true
		base = strings.TrimSuffix(base, "[]")
	}
	if strings.HasSuffix(base, "?") {
		isOptional = true
		base = strings.TrimSuffix(base, "?")
	}
	return prismaField{
		Name:       name,
		Type:       typ,
		BaseType:   base,
		IsList:     isList,
		IsOptional: isOptional,
		Attrs:      attrs,
	}, true
}

func parseBelongsToRelation(f prismaField, refTable string) (belongsTo, bool) {
	if f.IsList {
		return belongsTo{}, false
	}
	m := relationRe.FindStringSubmatch(f.Attrs)
	if len(m) != 2 {
		return belongsTo{}, false
	}
	body := m[1]
	fieldsM := fieldsListRe.FindStringSubmatch(body)
	refsM := refsListRe.FindStringSubmatch(body)
	if len(fieldsM) != 2 || len(refsM) != 2 {
		return belongsTo{}, false
	}
	fields := parseCSVNames(fieldsM[1])
	refs := parseCSVNames(refsM[1])
	if len(fields) == 0 || len(refs) == 0 {
		return belongsTo{}, false
	}
	relName := parseRelationName(body)
	return belongsTo{
		RelName:      relationNameFromFK(fields[0], refTable),
		ToModel:      f.BaseType,
		FKColumn:     fields[0],
		PKColumn:     refs[0],
		RelationName: relName,
	}, true
}

func parseListRelation(f prismaField) (listRelation, bool) {
	if !f.IsList {
		return listRelation{}, false
	}
	relName := parseRelationNameFromAttrs(f.Attrs)
	return listRelation{
		Name:         f.Name,
		ToModel:      f.BaseType,
		RelationName: relName,
	}, true
}

func scalarColumns(m prismaModel) map[string]string {
	out := make(map[string]string)
	for _, f := range m.Fields {
		if !m.ScalarNames[f.Name] {
			continue
		}
		out[f.Name] = mapPrismaTypeToYAML(f.BaseType)
	}
	return out
}

func chooseIDColumn(cols map[string]string, pkCols []string) (source, typ string, ok bool) {
	if t, exists := cols["id"]; exists {
		return "id", t, true
	}
	if len(pkCols) == 1 {
		if t, exists := cols[pkCols[0]]; exists {
			return pkCols[0], t, true
		}
	}
	return "", "", false
}

func chooseNameLikeColumn(cols map[string]string, idSource string) string {
	best := ""
	bestScore := -1
	for name, typ := range cols {
		if name == idSource {
			continue
		}
		s := scoreNameColumn(name, typ)
		if s > bestScore {
			bestScore = s
			best = name
		}
	}
	if bestScore < 0 {
		return ""
	}
	return best
}

func scoreNameColumn(name, typ string) int {
	n := strings.ToLower(name)
	score := 0
	switch n {
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
	if strings.Contains(n, "name") {
		score += 70
	}
	if strings.Contains(n, "title") || strings.Contains(n, "label") {
		score += 50
	}
	if strings.Contains(n, "code") || strings.Contains(n, "number") {
		score += 30
	}
	if typ == "string" {
		score += 25
	} else {
		score += 5
	}
	return score
}

func mapPrismaTypeToYAML(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "bigint":
		return "int"
	case "float", "decimal":
		return "float"
	case "boolean":
		return "bool"
	case "datetime":
		return "datetime"
	case "date":
		return "date"
	case "json", "bytes":
		return "string"
	case "string":
		return "string"
	default:
		return "string"
	}
}

func stripComment(s string) string {
	if idx := strings.Index(s, "//"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func parseCSVNames(s string) []string {
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func isScalarType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "string", "boolean", "int", "bigint", "float", "decimal", "datetime", "json", "bytes", "unsupported":
		return true
	default:
		return false
	}
}

func relationNameFromFK(columnName, refTable string) string {
	name := strings.TrimSpace(columnName)
	name = toSnakeCase(name)
	if strings.HasSuffix(name, "_id") && len(name) > 3 {
		name = name[:len(name)-3]
	}
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	return strings.ToLower(strings.TrimSpace(refTable))
}

func relationNameFromModel(model string) string {
	return toSnakeCase(model)
}

func chooseHasManyRelationName(m prismaModel, in incomingFK) string {
	candidates := make([]listRelation, 0)
	for _, rel := range m.ListRels {
		if rel.ToModel == in.FromModel {
			candidates = append(candidates, rel)
		}
	}
	if len(candidates) == 0 {
		return relationNameFromModel(in.FromModel)
	}
	if in.RelationName != "" {
		for _, rel := range candidates {
			if rel.RelationName == in.RelationName {
				return rel.Name
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0].Name
	}
	return relationNameFromModel(in.FromModel)
}

func toSnakeCase(in string) string {
	if in == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range in {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func uniqueRelationName(relations map[string]relationYAML, base string) string {
	if _, ok := relations[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s_%d", base, i)
		if _, ok := relations[name]; !ok {
			return name
		}
	}
}

func addHasManyRelationPresets(presets map[string]presetYAML, relations map[string]relationYAML) {
	for relName, rel := range relations {
		if rel.Type != "has_many" {
			continue
		}
		presetName := uniquePresetName(presets, "with_"+relName)
		presets[presetName] = presetYAML{
			Fields: []fieldYAML{
				{
					Source: relName,
					Type:   "preset",
					Preset: "item",
				},
			},
		}
	}
}

func uniquePresetName(presets map[string]presetYAML, base string) string {
	if _, ok := presets[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s_%d", base, i)
		if _, ok := presets[name]; !ok {
			return name
		}
	}
}

func parseRelationNameFromAttrs(attrs string) string {
	m := relationRe.FindStringSubmatch(attrs)
	if len(m) != 2 {
		return ""
	}
	return parseRelationName(m[1])
}

func parseRelationName(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	parts := strings.SplitN(body, ",", 2)
	first := strings.TrimSpace(parts[0])
	if strings.HasPrefix(first, "\"") && strings.HasSuffix(first, "\"") && len(first) >= 2 {
		return strings.Trim(first, "\"")
	}
	nameM := relNameKVRe.FindStringSubmatch(body)
	if len(nameM) == 2 {
		return nameM[1]
	}
	return ""
}
