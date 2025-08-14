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
func CountHandler(w http.ResponseWriter, r *http.Request) {
	var req CountRequest
	// Проверяем  запрос
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
	err := m.GetAliasMapFromRedisOrBuild(r.Context(), req.Model)
	if err != nil {
		http.Error(w, fmt.Sprintf("alias map error: %w", err), http.StatusInternalServerError)
	 	return 
	}
	for k, v := range m.GetAliasMap().PathToAlias {
		log.Printf("aliasMap[%s] = %s", k, v)
	}
	
	// Строим SQL-запрос для подсчета записей
	query, err := m.BuildCountQuery(req.Filters)
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

	//
	log.Println("SQL for count:", sqlStr)
	log.Println("ARGS:", args)

	// Выполняем запрос к базе данных
	// Используем QueryRow, так как нам нужен только один результат
	// Это оптимально для подсчета количества записей
	row := db.Pool.QueryRow(r.Context(), sqlStr, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		http.Error(w, fmt.Sprintf("DB error: %v", err), http.StatusInternalServerError)
		return
	}
	// Возвращаем результат в формате JSON
	json.NewEncoder(w).Encode(map[string]int{"count": count})
	
	//temporary solution to flush alias maps
	err = model.FlushAliasMaps(r.Context())
	if err != nil {
		log.Println("❌ Flush failed: "+err.Error())
		return
	}
}
