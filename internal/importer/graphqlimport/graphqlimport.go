package graphqlimport

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"YrestAPI/internal/model"

	"gopkg.in/yaml.v3"
)

type Result struct {
	Files    []ModelFile
	Warnings []string
}

type ModelFile struct {
	FileName string
	Content  []byte
}

type presetYAML struct {
	Fields []fieldYAML `yaml:"fields"`
}

type fieldYAML struct {
	Source   string `yaml:"source"`
	Type     string `yaml:"type"`
	Alias    string `yaml:"alias,omitempty"`
	Preset   string `yaml:"preset,omitempty"`
	Localize bool   `yaml:"localize,omitempty"`
}

type ImportOptions struct {
	ModelsDir      string
	ReplacePresets bool
}

type document struct {
	Operations []operation
}

type operation struct {
	Type       string
	Name       string
	Selections []selection
}

type selection struct {
	Alias      string
	Name       string
	Selections []selection
}

type parsedModel struct {
	Name string
	Path string
	Node *yaml.Node
	Data model.Model
}

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenName
	tokenLBrace
	tokenRBrace
	tokenColon
	tokenLParen
	tokenRParen
)

type token struct {
	typ tokenType
	val string
}

type lexer struct {
	src []rune
	pos int
}

type parser struct {
	lx  *lexer
	cur token
}

func ImportFromPath(path string, opts ImportOptions) (*Result, error) {
	raw, err := readGraphQLSources(path)
	if err != nil {
		return nil, err
	}
	doc, err := parseDocument(raw)
	if err != nil {
		return nil, err
	}
	return ApplyDocument(doc, opts)
}

func ApplyDocument(doc *document, opts ImportOptions) (*Result, error) {
	modelsDir := strings.TrimSpace(opts.ModelsDir)
	if modelsDir == "" {
		modelsDir = "./db"
	}
	models, err := loadModels(modelsDir)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	presetCache := map[string]string{}

	for _, op := range doc.Operations {
		if op.Type != "" && op.Type != "query" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skip operation %q: only query operations are supported", op.Name))
			continue
		}
		for _, root := range op.Selections {
			modelName := rootFieldToModelName(root.Name)
			pm, ok := models[modelName]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("skip root field %q: model %q not found", root.Name, modelName))
				continue
			}
			if err := ensurePresetForSelection(models, pm, op, root, nil, opts.ReplacePresets, presetCache, result); err != nil {
				return nil, err
			}
		}
	}

	paths := make([]string, 0, len(models))
	for _, pm := range models {
		paths = append(paths, pm.Path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		pm := findParsedModelByPath(models, path)
		if pm == nil || pm.Node == nil {
			continue
		}
		raw, err := yaml.Marshal(pm.Node)
		if err != nil {
			return nil, fmt.Errorf("marshal model %s: %w", pm.Name, err)
		}
		result.Files = append(result.Files, ModelFile{
			FileName: filepath.Base(path),
			Content:  raw,
		})
	}
	return result, nil
}

func WriteFiles(dir string, files []ModelFile) error {
	for _, f := range files {
		path := filepath.Join(dir, f.FileName)
		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func ensurePresetForSelection(models map[string]*parsedModel, pm *parsedModel, op operation, sel selection, path []string, replace bool, cache map[string]string, result *Result) error {
	presetName := presetNameForSelection(op, sel, path)
	cacheKey := cacheKeyFor(pm.Name, presetName)
	if existing, ok := cache[cacheKey]; ok {
		fields, err := buildPresetFields(models, pm, op, sel, path, replace, cache, result)
		if err != nil {
			return err
		}
		return ensurePresetNode(pm, existing, fields, replace)
	}

	fields, err := buildPresetFields(models, pm, op, sel, path, replace, cache, result)
	if err != nil {
		return err
	}
	if err := ensurePresetNode(pm, presetName, fields, replace); err != nil {
		return err
	}
	cache[cacheKey] = presetName
	return nil
}

func buildPresetFields(models map[string]*parsedModel, pm *parsedModel, op operation, sel selection, path []string, replace bool, cache map[string]string, result *Result) ([]model.Field, error) {
	fields := make([]model.Field, 0, len(sel.Selections))
	typeBySource := collectKnownFieldTypes(&pm.Data)
	for _, child := range sel.Selections {
		if len(child.Selections) == 0 {
			fieldType := typeBySource[child.Name]
			if fieldType == "" {
				fieldType = "string"
				result.Warnings = append(result.Warnings, fmt.Sprintf("model %q preset source %q defaulted to type string; adjust manually if DB type differs", pm.Name, child.Name))
			}
			f := model.Field{
				Source: child.Name,
				Type:   fieldType,
			}
			if child.Alias != "" && child.Alias != child.Name {
				f.Alias = child.Alias
			}
			fields = append(fields, f)
			continue
		}

		rel := pm.Data.Relations[child.Name]
		if rel == nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("model %q skip nested field %q: relation not found", pm.Name, child.Name))
			continue
		}
		targetModel := models[rel.Model]
		if targetModel == nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("model %q skip nested field %q: target model %q not found", pm.Name, child.Name, rel.Model))
			continue
		}
		childPath := append(append([]string{}, path...), child.Name)
		if err := ensurePresetForSelection(models, targetModel, op, child, childPath, replace, cache, result); err != nil {
			return nil, err
		}
		f := model.Field{
			Source:       child.Name,
			Type:         "preset",
			NestedPreset: presetNameForSelection(op, child, childPath),
		}
		if child.Alias != "" && child.Alias != child.Name {
			f.Alias = child.Alias
		}
		fields = append(fields, f)
	}
	return fields, nil
}

func ensurePresetNode(pm *parsedModel, presetName string, fields []model.Field, replace bool) error {
	presetsNode := ensureMapValue(pm.Node.Content[0], "presets")
	if presetsNode.Kind != yaml.MappingNode {
		return fmt.Errorf("model %s: presets must be mapping", pm.Name)
	}
	idx := mapValueIndex(presetsNode, presetName)
	if idx >= 0 && !replace {
		return nil
	}
	preset := &model.DataPreset{Fields: fields}
	fieldNode, err := encodeNode(toPresetYAML(fields))
	if err != nil {
		return fmt.Errorf("encode preset %s.%s: %w", pm.Name, presetName, err)
	}
	if idx >= 0 {
		presetsNode.Content[idx] = fieldNode
	} else {
		presetsNode.Content = append(presetsNode.Content, scalarNode(presetName), fieldNode)
	}
	if pm.Data.Presets == nil {
		pm.Data.Presets = map[string]*model.DataPreset{}
	}
	pm.Data.Presets[presetName] = preset
	return nil
}

func loadModels(dir string) (map[string]*parsedModel, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	out := make(map[string]*parsedModel, len(files))
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var root yaml.Node
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return nil, fmt.Errorf("parse model %s: %w", path, err)
		}
		if len(root.Content) == 0 {
			continue
		}
		var m model.Model
		if err := root.Decode(&m); err != nil {
			return nil, fmt.Errorf("decode model %s: %w", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		m.Name = name
		out[name] = &parsedModel{
			Name: name,
			Path: path,
			Node: &root,
			Data: m,
		}
	}
	return out, nil
}

func readGraphQLSources(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	files, err := filepath.Glob(filepath.Join(path, "*.graphql"))
	if err != nil {
		return "", err
	}
	gqlFiles, err := filepath.Glob(filepath.Join(path, "*.gql"))
	if err != nil {
		return "", err
	}
	files = append(files, gqlFiles...)
	sort.Strings(files)
	parts := make([]string, 0, len(files))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(raw))
	}
	return strings.Join(parts, "\n"), nil
}

func parseDocument(src string) (*document, error) {
	p := &parser{lx: &lexer{src: []rune(src)}}
	p.next()
	doc := &document{}
	for p.cur.typ != tokenEOF {
		op, err := p.parseOperation()
		if err != nil {
			return nil, err
		}
		doc.Operations = append(doc.Operations, op)
	}
	return doc, nil
}

func (p *parser) parseOperation() (operation, error) {
	op := operation{Type: "query"}
	if p.cur.typ == tokenLBrace {
		sels, err := p.parseSelectionSet()
		if err != nil {
			return op, err
		}
		op.Selections = sels
		return op, nil
	}
	if p.cur.typ != tokenName {
		return op, fmt.Errorf("unexpected token %q", p.cur.val)
	}
	if p.cur.val == "fragment" {
		return op, fmt.Errorf("fragments are not supported in graphql import")
	}
	op.Type = p.cur.val
	p.next()
	if p.cur.typ == tokenName {
		op.Name = p.cur.val
		p.next()
	}
	if p.cur.typ == tokenLParen {
		p.skipParens()
	}
	sels, err := p.parseSelectionSet()
	if err != nil {
		return op, err
	}
	op.Selections = sels
	return op, nil
}

func (p *parser) parseSelectionSet() ([]selection, error) {
	if p.cur.typ != tokenLBrace {
		return nil, fmt.Errorf("expected '{', got %q", p.cur.val)
	}
	p.next()
	var out []selection
	for p.cur.typ != tokenRBrace && p.cur.typ != tokenEOF {
		sel, err := p.parseSelection()
		if err != nil {
			return nil, err
		}
		out = append(out, sel)
	}
	if p.cur.typ != tokenRBrace {
		return nil, fmt.Errorf("expected '}', got %q", p.cur.val)
	}
	p.next()
	return out, nil
}

func (p *parser) parseSelection() (selection, error) {
	if p.cur.typ != tokenName {
		return selection{}, fmt.Errorf("expected field name, got %q", p.cur.val)
	}
	sel := selection{Name: p.cur.val}
	p.next()
	if p.cur.typ == tokenColon {
		sel.Alias = sel.Name
		p.next()
		if p.cur.typ != tokenName {
			return selection{}, fmt.Errorf("expected field name after alias, got %q", p.cur.val)
		}
		sel.Name = p.cur.val
		p.next()
	}
	if p.cur.typ == tokenLParen {
		p.skipParens()
	}
	if p.cur.typ == tokenLBrace {
		children, err := p.parseSelectionSet()
		if err != nil {
			return selection{}, err
		}
		sel.Selections = children
	}
	return sel, nil
}

func (p *parser) skipParens() {
	depth := 0
	for {
		if p.cur.typ == tokenLParen {
			depth++
		} else if p.cur.typ == tokenRParen {
			depth--
			if depth == 0 {
				p.next()
				return
			}
		} else if p.cur.typ == tokenEOF {
			return
		}
		p.next()
	}
}

func (p *parser) next() {
	p.cur = p.lx.nextToken()
}

func (lx *lexer) nextToken() token {
	for lx.pos < len(lx.src) {
		r := lx.src[lx.pos]
		if unicode.IsSpace(r) || r == ',' {
			lx.pos++
			continue
		}
		if r == '#' {
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.pos++
			}
			continue
		}
		if lx.pos+2 < len(lx.src) && string(lx.src[lx.pos:lx.pos+3]) == "..." {
			return token{typ: tokenEOF, val: "..."}
		}
		switch r {
		case '{':
			lx.pos++
			return token{typ: tokenLBrace, val: "{"}
		case '}':
			lx.pos++
			return token{typ: tokenRBrace, val: "}"}
		case ':':
			lx.pos++
			return token{typ: tokenColon, val: ":"}
		case '(':
			lx.pos++
			return token{typ: tokenLParen, val: "("}
		case ')':
			lx.pos++
			return token{typ: tokenRParen, val: ")"}
		}
		if isNameStart(r) {
			start := lx.pos
			lx.pos++
			for lx.pos < len(lx.src) && isNamePart(lx.src[lx.pos]) {
				lx.pos++
			}
			return token{typ: tokenName, val: string(lx.src[start:lx.pos])}
		}
		lx.pos++
	}
	return token{typ: tokenEOF}
}

func isNameStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isNamePart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func presetNameForSelection(op operation, sel selection, path []string) string {
	root := sel.Name
	if len(path) > 0 {
		root = path[0]
	}
	base := root
	if op.Name != "" {
		base += "__" + op.Name
	}
	base += "__" + shapeHash(selectionSignature(sel))
	if len(path) > 0 {
		base += "__" + strings.Join(path, "__")
	}
	return base
}

func selectionSignature(sel selection) string {
	parts := make([]string, 0, len(sel.Selections))
	for _, child := range sel.Selections {
		name := child.Name
		if child.Alias != "" && child.Alias != child.Name {
			name = child.Alias + ":" + child.Name
		}
		if len(child.Selections) > 0 {
			name += "{" + selectionSignature(child) + "}"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ",")
}

func shapeHash(sig string) string {
	sum := sha1.Sum([]byte(sig))
	return hex.EncodeToString(sum[:])[:8]
}

func rootFieldToModelName(root string) string {
	parts := strings.Split(strings.TrimSpace(root), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func collectKnownFieldTypes(m *model.Model) map[string]string {
	out := map[string]string{}
	if m == nil {
		return out
	}
	for _, preset := range m.Presets {
		for _, f := range preset.Fields {
			if f.Type == "" || f.Type == "preset" || f.Source == "" {
				continue
			}
			if _, ok := out[f.Source]; !ok {
				out[f.Source] = f.Type
			}
		}
	}
	return out
}

func findParsedModelByPath(models map[string]*parsedModel, path string) *parsedModel {
	for _, pm := range models {
		if pm.Path == path {
			return pm
		}
	}
	return nil
}

func cacheKeyFor(modelName, presetName string) string {
	return modelName + "::" + presetName
}

func ensureMapValue(root *yaml.Node, key string) *yaml.Node {
	if root.Kind != yaml.MappingNode {
		root.Kind = yaml.MappingNode
	}
	if idx := mapValueIndex(root, key); idx >= 0 {
		return root.Content[idx]
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = append(root.Content, scalarNode(key), node)
	return node
}

func mapValueIndex(root *yaml.Node, key string) int {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return i + 1
		}
	}
	return -1
}

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func encodeNode(v any) (*yaml.Node, error) {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	return root.Content[0], nil
}

func toPresetYAML(fields []model.Field) presetYAML {
	out := presetYAML{
		Fields: make([]fieldYAML, 0, len(fields)),
	}
	for _, f := range fields {
		out.Fields = append(out.Fields, fieldYAML{
			Source:   f.Source,
			Type:     f.Type,
			Alias:    f.Alias,
			Preset:   f.NestedPreset,
			Localize: f.Localize,
		})
	}
	return out
}
