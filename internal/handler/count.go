package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"YrestAPI/internal/db"
	"YrestAPI/internal/model"
)

type CountRequest struct {
	Model   string                 `json:"model"`
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	m, ok := model.Registry[req.Model]
	if !ok {
		http.Error(w, fmt.Sprintf("Model %s not found", req.Model), http.StatusNotFound)
		return
	}

	// Получаем карту алиасов из Redis или строим на лету
	aliasMap, err := m.CreateAliasMap(m, nil, req.Filters, nil);
	if err != nil {
		// БЫЛО: fmt.Sprintf("alias map error: %w", err) — НЕЛЬЗЯ
		http.Error(w, "alias map error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	

	// Строим SQL-запрос для подсчета записей
	query, err := m.BuildCountQuery(aliasMap, req.Filters)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query error: %v", err), http.StatusInternalServerError)
		return
	}

	// Преобразуем запрос в SQL-строку и аргументы
	sqlStr, args, err := query.ToSql()
	if err != nil {
		http.Error(w, fmt.Sprintf("SQL error: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("SQL for count:", sqlStr)
	log.Println("ARGS:", args)

	// Выполняем запрос к базе данных
	row := db.Pool.QueryRow(r.Context(), sqlStr, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		http.Error(w, fmt.Sprintf("DB error: %v", err), http.StatusInternalServerError)
		return
	}

	// Успех: JSON-ответ
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})

	// temporary solution to flush alias maps
	if err := model.FlushAliasMaps(r.Context()); err != nil {
		log.Println("❌ Flush failed: " + err.Error())
		// уже отправили 200 и тело — просто залогируем
	}
}
