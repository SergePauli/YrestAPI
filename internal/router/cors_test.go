package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORS_AllowsSingleOrigin(t *testing.T) {
	h := withCORS("http://localhost:3000", false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/index", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	h(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("unexpected vary: %q", got)
	}
}

func TestWithCORS_AllowsFromCSVList(t *testing.T) {
	h := withCORS("http://192.168.0.251:3000,http://cbs:3000", false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/index", nil)
	req.Header.Set("Origin", "http://cbs:3000")
	w := httptest.NewRecorder()
	h(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://cbs:3000" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
}

func TestWithCORS_BlocksUnknownOriginFromCSVList(t *testing.T) {
	h := withCORS("http://192.168.0.251:3000,http://cbs:3000", false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/index", nil)
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin for blocked origin: %q", got)
	}
}
