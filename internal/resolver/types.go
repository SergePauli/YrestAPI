package resolver


type IndexRequest struct {
	Model   string                 `json:"model"`
	Preset  string                 `json:"preset"`
	Filters map[string]interface{} `json:"filters"`
	Sorts []string								 `json:"sorts"`	
	Offset  uint64                 `json:"offset"`
	Limit   uint64                 `json:"limit"`	
}