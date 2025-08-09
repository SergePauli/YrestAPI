package router

import (
	"YrestAPI/internal/handler"
	"net/http"
)

// InitRoutes инициализирует маршруты для API
func InitRoutes() {
	http.HandleFunc("/api/index", handler.IndexHandler)
	http.HandleFunc("/api/count", handler.CountHandler)
	// Добавьте другие обработчики по мере необходимости
}