package router

import (
	"YrestAPI/internal/handler"
	"YrestAPI/internal/logger"
	"net/http"
)

// InitRoutes инициализирует маршруты для API
func InitRoutes() {
	http.HandleFunc("/api/index", withCORS(withLogging(handler.IndexHandler)))
	http.HandleFunc("/api/count", withCORS(withLogging(handler.CountHandler)))
	// Добавьте другие обработчики по мере необходимости
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next(sw, r)
		level := "info"
		if sw.status >= 500 {
			level = "error"
		} else if sw.status >= 400 {
			level = "warn"
		}
		fields := map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": sw.status,
		}
		switch level {
		case "error":
			logger.Error("response", fields)
		case "warn":
			logger.Warn("response", fields)
		default:
			logger.Info("response", fields)
		}
	}
}
