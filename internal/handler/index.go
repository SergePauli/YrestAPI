package handler

import (
	"YrestAPI/internal/logger"
	"YrestAPI/internal/resolver"

	"encoding/json"
	"io"
	"net/http"
)

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	// Ограничим только POST-запросы
	if r.Method != http.MethodPost {
		logger.Warn("method_not_allowed", map[string]any{
			"endpoint": "/api/index",
			"method":   r.Method,
		})
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req resolver.IndexRequest

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn("read_body_failed", map[string]any{
			"endpoint": "/api/index",
			"error":    err.Error(),
		})
		http.Error(w, "Failed to read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Попробуем распарсить JSON
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Warn("invalid_json", map[string]any{
			"endpoint": "/api/index",
			"error":    err.Error(),
		})
		http.Error(w, "Invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	logger.Info("request", map[string]any{
		"endpoint": "/api/index",
		"payload":  json.RawMessage(body),
	})

	// Вызываем Resolver
	result, err := resolver.Resolver(r.Context(), req)
	if err != nil {
		logger.Error("resolver_error", map[string]any{
			"endpoint": "/api/index",
			"error":    err.Error(),
		})
		http.Error(w, "Failed to resolve data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Успешный JSON-ответ
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		logger.Error("write_response_failed", map[string]any{
			"endpoint": "/api/index",
			"error":    err.Error(),
		})
		http.Error(w, "Failed to write response: "+err.Error(), http.StatusInternalServerError)
	}
}
