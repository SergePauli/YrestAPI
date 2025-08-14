package resolver

import "YrestAPI/internal/model"

var maxLimit = uint64(1000) // максимальный лимит для запросов
type IndexRequest struct {
	Model   string                 `json:"model"`
	Preset  string                 `json:"preset"`
	Filters map[string]interface{} `json:"filters"`
	Sorts []string								 `json:"sorts"`	
	Offset  uint64                 `json:"offset"`
	Limit   uint64                 `json:"limit"`	
	ThroughFor    string `json:"-"` // имя связи в промежуточной модели, которую нужно вернуть (напр. "contact")
	ThroughPreset string `json:"-"` // пресет конечной модели для этой связи (напр. "item")
	// служебные (только для внутренних вызовов)
	PresetObj   *model.DataPreset `json:"-"` // синтетический пресет (если задан — имеет приоритет над Preset)
	UnwrapField string            `json:"-"` // какое preset-поле развернуть в конце (например "contact")
}