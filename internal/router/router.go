package router

import (
	"YrestAPI/internal/auth"
	"YrestAPI/internal/config"
	"YrestAPI/internal/handler"
	"YrestAPI/internal/logger"
	"net/http"
	"strings"
)

// InitRoutes инициализирует маршруты для API
func InitRoutes(cfg *config.Config) error {
	var validator *auth.JWTValidator
	var err error
	if cfg.Auth.Enabled {
		validator, err = auth.NewJWTValidator(cfg.Auth.JWT)
		if err != nil {
			return err
		}
	}

	http.HandleFunc("/api/index", withCORS(cfg.CORS.AllowOrigin, cfg.CORS.AllowCredentials, withLogging(withAuth(validator, handler.IndexHandler))))
	http.HandleFunc("/api/count", withCORS(cfg.CORS.AllowOrigin, cfg.CORS.AllowCredentials, withLogging(withAuth(validator, handler.CountHandler))))
	// Добавьте другие обработчики по мере необходимости
	return nil
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

func withAuth(validator *auth.JWTValidator, next http.HandlerFunc) http.HandlerFunc {
	if validator == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			http.Error(w, "Authorization header must be Bearer token", http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(authHeader[len("Bearer "):])
		claims, err := validator.ValidateToken(token)
		if err != nil {
			logger.Warn("auth_failed", map[string]any{
				"path":  r.URL.Path,
				"error": err.Error(),
			})
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
	}
}
