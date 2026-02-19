package router

import (
	"net/http"
	"strings"
)

// withCORS adds CORS headers and handles preflight requests.
func withCORS(allowOrigin string, allowCredentials bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		originValue, varyOrigin := resolveAllowOrigin(allowOrigin, allowCredentials, r.Header.Get("Origin"))
		if originValue != "" {
			w.Header().Set("Access-Control-Allow-Origin", originValue)
		}
		if varyOrigin {
			w.Header().Set("Vary", "Origin")
		}
		if allowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h(w, r)
	}
}

func resolveAllowOrigin(allowOrigin string, allowCredentials bool, requestOrigin string) (value string, varyOrigin bool) {
	origins := parseOrigins(allowOrigin)
	if len(origins) == 0 {
		return "*", false
	}

	wildcard := false
	for _, o := range origins {
		if o == "*" {
			wildcard = true
			break
		}
	}

	if wildcard {
		if allowCredentials && requestOrigin != "" {
			return requestOrigin, true
		}
		return "*", false
	}

	if requestOrigin == "" {
		return "", true
	}

	for _, o := range origins {
		if o == requestOrigin {
			return requestOrigin, true
		}
	}
	return "", true
}

func parseOrigins(allowOrigin string) []string {
	parts := strings.Split(allowOrigin, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		res = append(res, p)
	}
	return res
}
