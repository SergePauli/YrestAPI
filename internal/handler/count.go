package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"YrestAPI/internal/db"
	"YrestAPI/internal/logger"
	"YrestAPI/internal/model"
)

type CountRequest struct {
	Model   string                 `json:"model"`
	Preset  string                 `json:"preset"`
	Filters map[string]interface{} `json:"filters"`
}

// CountHandler обрабатывает запросы на подсчет количества записей
// Ожидает JSON с полями Model, Preset, Filters
// Возвращает JSON с полем count, содержащим количество записей
// CountHandler обрабатывает запросы на подсчет количества записей
// Ожидает JSON с полями Model, Preset, Filters
// Возвращает JSON с полем count, содержащим количество записей
func CountHandler(w http.ResponseWriter, r *http.Request) {
	var req CountRequest

	// Декод тела
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("read_body_failed", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		http.Error(w, "Failed to read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Warn("invalid_json", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	logger.Info("request", map[string]any{
		"endpoint": "/api/count",
		"payload":  json.RawMessage(body),
	})

	m, ok := model.Registry[req.Model]
	if !ok {
		logger.Warn("model_not_found", map[string]any{
			"endpoint": "/api/count",
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
				"endpoint": "/api/count",
				"model":    req.Model,
				"preset":   req.Preset,
			})
			http.Error(w, fmt.Sprintf("Preset %s not found", req.Preset), http.StatusBadRequest)
			return
		}
	}

	// Разворачиваем короткие алиасы в фильтрах, чтобы карта алиасов и WHERE работали с одними ключами
	filters := model.NormalizeFiltersWithAliases(m, req.Filters)

	// Получаем карту алиасов из Redis или строим на лету
	aliasMap, err := m.CreateAliasMap(m, preset, filters, nil)
	if err != nil {
		logger.Error("alias_map_error", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		// БЫЛО: fmt.Sprintf("alias map error: %w", err) — НЕЛЬЗЯ
		http.Error(w, "alias map error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Строим SQL-запрос для подсчета записей
	query, err := m.BuildCountQuery(aliasMap, preset, filters)
	if err != nil {
		logger.Error("query_error", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		http.Error(w, fmt.Sprintf("Query error: %v", err), http.StatusInternalServerError)
		return
	}

	// Преобразуем запрос в SQL-строку и аргументы
	sqlStr, args, err := query.ToSql()
	if err != nil {
		logger.Error("sql_error", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		http.Error(w, fmt.Sprintf("SQL error: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Debug("sql", map[string]any{
		"endpoint": "/api/count",
		"sql":      sqlStr,
		"args":     args,
	})

	// Выполняем запрос к базе данных
	row := db.Pool.QueryRow(r.Context(), sqlStr, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		logger.Error("db_error", map[string]any{
			"endpoint": "/api/count",
			"error":    err.Error(),
		})
		http.Error(w, fmt.Sprintf("DB error: %v", err), http.StatusInternalServerError)
		return
	}

	// Успех: JSON-ответ
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})

}
