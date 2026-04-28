package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"YrestAPI/internal/db"
	"YrestAPI/internal/logger"
	"YrestAPI/internal/model"
)

type StatsRequest struct {
	Model      string                        `json:"model"`
	Preset     string                        `json:"preset"`
	Filters    map[string]interface{}        `json:"filters"`
	Aggregates map[string]StatsAggregateSpec `json:"aggregates"`
}

type StatsAggregateSpec struct {
	Fn    string `json:"fn"`
	Field string `json:"field"`
}

// StatsHandler handles aggregate stats requests and plain count requests.
// It accepts the same filter semantics as /api/index and returns {"count": N}
// unless aggregates are explicitly requested.
func StatsHandler(w http.ResponseWriter, r *http.Request) {
	var req StatsRequest
	endpoint := r.URL.Path

	// Декод тела
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("read_body_failed", map[string]any{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
		http.Error(w, "Failed to read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Warn("invalid_json", map[string]any{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.Info("request", map[string]any{
		"endpoint": endpoint,
		"payload":  json.RawMessage(body),
	})

	m, ok := model.Registry[req.Model]
	if !ok {
		logger.Warn("model_not_found", map[string]any{
			"endpoint": endpoint,
			"model":    req.Model,
		})
		http.Error(w, fmt.Sprintf("Model %s not found", req.Model), http.StatusNotFound)
		return
	}

	var preset *model.DataPreset
	if strings.TrimSpace(req.Preset) != "" {
		preset = m.Presets[req.Preset]
		if preset == nil {
			logger.Warn("preset_not_found", map[string]any{
				"endpoint": endpoint,
				"model":    req.Model,
				"preset":   req.Preset,
			})
			http.Error(w, fmt.Sprintf("Preset %s not found", req.Preset), http.StatusBadRequest)
			return
		}
	}

	// Разворачиваем короткие алиасы в фильтрах, чтобы карта алиасов и WHERE работали с одними ключами
	filters := model.NormalizeFiltersWithAliases(m, req.Filters)
	aggregateSpecs := make([]model.AggregateSpec, 0, len(req.Aggregates))
	for name, spec := range req.Aggregates {
		aggregateSpecs = append(aggregateSpecs, model.AggregateSpec{
			Name:  name,
			Fn:    spec.Fn,
			Field: spec.Field,
		})
	}

	// Получаем карту алиасов из кэша или строим на лету
	aliasMap, err := m.CreateAliasMapForAggregates(preset, filters, aggregateSpecs)
	if err != nil {
		logger.Error("alias_map_error", map[string]any{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
		// БЫЛО: fmt.Sprintf("alias map error: %w", err) — НЕЛЬЗЯ
		http.Error(w, "alias map error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(aggregateSpecs) == 0 {
		if err := writeStatsOnlyResponse(r, w, endpoint, m, aliasMap, preset, filters); err != nil {
			logger.Error("stats_error", map[string]any{
				"endpoint": endpoint,
				"error":    err.Error(),
			})
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := writeStatsAggregateResponse(r, w, endpoint, m, aliasMap, preset, filters, aggregateSpecs); err != nil {
		status := http.StatusInternalServerError
		if isAggregateValidationError(err) {
			status = http.StatusBadRequest
		}
		logger.Error("stats_aggregate_error", map[string]any{
			"endpoint": endpoint,
			"error":    err.Error(),
		})
		http.Error(w, err.Error(), status)
	}
}

// CountHandler is a deprecated backward-compatible alias for /api/count.
func CountHandler(w http.ResponseWriter, r *http.Request) {
	StatsHandler(w, r)
}

func isAggregateValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	markers := []string{
		"aggregate name is required",
		"duplicate aggregate name",
		"unsupported function",
		"is not aggregatable",
		"is not allowed",
		"field is required",
		"traverses has_many relation",
		"relation",
		"could not resolve field",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func writeStatsOnlyResponse(r *http.Request, w http.ResponseWriter, endpoint string, m *model.Model, aliasMap *model.AliasMap, preset *model.DataPreset, filters map[string]interface{}) error {
	query, err := m.BuildCountQuery(aliasMap, preset, filters)
	if err != nil {
		return fmt.Errorf("Query error: %v", err)
	}
	sqlStr, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("SQL error: %v", err)
	}
	logger.Debug("sql", map[string]any{
		"endpoint": endpoint,
		"sql":      sqlStr,
		"args":     args,
	})
	row := db.Pool.QueryRow(r.Context(), sqlStr, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("DB error: %v", err)
	}
	return json.NewEncoder(w).Encode(map[string]int{"count": count})
}

func writeStatsAggregateResponse(r *http.Request, w http.ResponseWriter, endpoint string, m *model.Model, aliasMap *model.AliasMap, preset *model.DataPreset, filters map[string]interface{}, aggregateSpecs []model.AggregateSpec) error {
	resolved, err := m.ValidateAndResolveAggregates(aliasMap, aggregateSpecs)
	if err != nil {
		return err
	}
	query, err := m.BuildCountAggregateQuery(aliasMap, preset, filters, resolved)
	if err != nil {
		return fmt.Errorf("Query error: %v", err)
	}
	sqlStr, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("SQL error: %v", err)
	}
	logger.Debug("sql", map[string]any{
		"endpoint": endpoint,
		"sql":      sqlStr,
		"args":     args,
	})

	rows, err := db.Pool.Query(r.Context(), sqlStr, args...)
	if err != nil {
		return fmt.Errorf("DB error: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return fmt.Errorf("DB error: empty aggregate result")
	}
	values, err := rows.Values()
	if err != nil {
		return fmt.Errorf("DB error: %v", err)
	}
	if rows.Err() != nil {
		return fmt.Errorf("DB error: %v", rows.Err())
	}
	if len(values) == 0 {
		return fmt.Errorf("DB error: empty aggregate row")
	}

	resp := map[string]any{
		"count":      normalizeNumericValue(values[0]),
		"aggregates": map[string]any{},
	}
	aggs := resp["aggregates"].(map[string]any)
	for i, spec := range resolved {
		if i+1 >= len(values) {
			break
		}
		aggs[spec.Name] = normalizeAggregateValue(values[i+1], spec.Type)
	}
	return json.NewEncoder(w).Encode(resp)
}

func normalizeAggregateValue(v any, fieldType string) any {
	kind := strings.ToLower(strings.TrimSpace(fieldType))
	switch vv := v.(type) {
	case nil:
		return nil
	case time.Time:
		if kind == "date" {
			return vv.Format("2006-01-02")
		}
		return vv.Format(time.RFC3339)
	case []byte:
		return string(vv)
	case string:
		return vv
	default:
		return normalizeNumericValue(v)
	}
}

func normalizeNumericValue(v any) any {
	switch vv := v.(type) {
	case int64:
		return vv
	case int32:
		return int64(vv)
	case int16:
		return int64(vv)
	case int8:
		return int64(vv)
	case int:
		return vv
	case float32:
		return float64(vv)
	case float64:
		return vv
	case []byte:
		return string(vv)
	default:
		return vv
	}
}
