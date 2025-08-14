package handler

import (
	"YrestAPI/internal/resolver"

	"encoding/json"
	"log"
	"net/http"
)



func IndexHandler(w http.ResponseWriter, r *http.Request) {
	// Ограничим только POST-запросы
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req resolver.IndexRequest

	// Попробуем распарсить JSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}


	log.Printf("Incoming index request: %+v\n", req)
	

	// Вызываем Resolver
	result, err := resolver.Resolver(r.Context(), req)
	if err != nil {
		log.Printf("Resolver error: %v", err)
		http.Error(w, "Failed to resolve data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Успешный JSON-ответ
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "Failed to write response: "+err.Error(), http.StatusInternalServerError)
	}
}