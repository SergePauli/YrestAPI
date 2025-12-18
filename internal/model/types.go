package model

// Model описывает структуру модели в конфигурации
type Model struct {
	Name        string                    `yaml:"-"` // logical name of the model
	Table       string                    `yaml:"table"`
	Relations   map[string]*ModelRelation `yaml:"relations"`
	Presets     map[string]*DataPreset    `yaml:"presets"`
	PrimaryKeys []string                  `yaml:"primary_keys"` // optional, e.g. ["id"] or ["part1","part2"]

}

// ModelRelation описывает связь между моделями в конфигурации
type ModelRelation struct {
	Type         string `yaml:"type"`          // has_one, has_many, belongs_to
	Model        string `yaml:"model"`         // название связанной модели (логическое)
	Table        string `yaml:"table"`         // имя таблицы в SQL
	FK           string `yaml:"fk"`            // внешний ключ (обычно fk к текущей модели)
	PK           string `yaml:"pk"`            // if not "id", primary key in the current model
	Through      string `yaml:"through"`       // для has_one/has_many :through
	Where        string `yaml:"where"`         // SQL-условие (без WHERE)
	ThroughWhere string `yaml:"through_where"` // SQL-условие для промежуточной модели (без WHERE)
	Order        string `yaml:"order"`         // сортировка по умолчанию
	Reentrant    bool   `yaml:"reentrant"`     // разрешить рекурсивные связи
	MaxDepth     int    `yaml:"max_depth"`     // максимальная глубина рекурсии
	Polymorphic  bool   `yaml:"polymorphic"`   // belongs_to polymorphic
	TypeColumn   string `yaml:"type_column"`   // column with type discriminator (default <rel>_type)

	// для runtime (не сериализуется)
	_ModelRef   *Model `yaml:"-"`
	_ThroughRef *Model `yaml:"-"`
}

// Карта алиасов: путь ↔ алиас.
// Путь задаётся относительно корневой модели пресета (например: "contragent", "contragent.contracts").
type AliasMap struct {
	PathToAlias map[string]string
	AliasToPath map[string]string
}

// DataPreset описывает структуру пресета в конфигурации
type DataPreset struct {
	Name    string  `yaml:"-"`
	Extends string  `yaml:"extends" json:"extends"`
	Fields  []Field `yaml:"fields"` // fields in this preset
	// Предвычисленная карта алиасов, собранная ТОЛЬКО из полей этого пресета (NestedPreset-поля).
	// Не включает пути из фильтров/сортировок; неизменяема после инициализации.
	FieldsAliasMap *AliasMap `yaml:"-" json:"-"`
}

// Preset описывает структуру поля пресета для SQL-запросов
type Field struct {
	Source       string `yaml:"source"`    // example: "id"
	Formatter    string `yaml:"formatter"` // example"{surname} {name}[0].{patrname}[0]."
	Alias        string `yaml:"alias"`     // optional override
	Type         string `yaml:"type"`      // "int", "string", "array", "bool"
	NestedPreset string `yaml:"preset"`    // name of another preset
	Internal     bool   `yaml:"internal"`  // если true, то поле не будет включено в ответ
	Localize     bool   `yaml:"localize"`  // если true, то поле будет локализовано
	MaxDepth     int    `yaml:"max_depth"` // максимальная глубина рекурсии для циклических связей
	// для runtime (не сериализуется)
	_PresetRef *DataPreset `yaml:"-"`
}

type JoinSpec struct {
	Table      string
	Alias      string
	On         string
	JoinType   string // "LEFT JOIN", "INNER JOIN", etc.
	Distinct   bool
	Conditions []string
	Where      string
}

// GetPrimaryKeys возвращает список полей первичного ключа для модели.
// Если не задано в конфиге, по умолчанию возвращает ["id"].
func (m *Model) GetPrimaryKeys() []string {
	if len(m.PrimaryKeys) > 0 {
		return m.PrimaryKeys
	}
	// fallback по умолчанию
	return []string{"id"}
}

// GetModelRef возвращает ссылку на модель, если она уже загружена,
func (m *ModelRelation) GetModelRef() *Model {
	return m._ModelRef
}

// SetModelRef устанавливает ссылку на модель (вызывается из Registry после загрузки всех моделей)
func (m *ModelRelation) SetModelRef(model *Model) {
	m._ModelRef = model
}

// GetThroughRef возвращает ссылку на промежуточную модель, если она задана
// Если не задано, ищет в Registry по имени через "Through".
func (m *ModelRelation) GetThroughRef() *Model {
	return m._ThroughRef
}

func (m *ModelRelation) SetThroughRef(model *Model) {
	m._ThroughRef = model
}

func (f *Field) SetPresetRef(preset *DataPreset) {
	f._PresetRef = preset
}

func (f *Field) GetPresetRef() (preset *DataPreset) {
	return f._PresetRef
}
