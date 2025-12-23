package router

import (
	"YrestAPI/internal/handler"
	"net/http"
)

// InitRoutes инициализирует маршруты для API
func InitRoutes() {
	http.HandleFunc("/api/index", withCORS(handler.IndexHandler))
	http.HandleFunc("/api/count", withCORS(handler.CountHandler))
	// Добавьте другие обработчики по мере необходимости
}
