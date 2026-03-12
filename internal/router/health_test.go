package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzHandler_OK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	healthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "ok" {
		t.Fatalf("body=%q, want %q", got, "ok")
	}
}

func TestReadyzHandler_NotReadyWithoutRegistry(t *testing.T) {
	prevRegistryReady := registryReadyFunc
	prevDBReady := dbReadyFunc
	t.Cleanup(func() {
		registryReadyFunc = prevRegistryReady
		dbReadyFunc = prevDBReady
	})

	registryReadyFunc = func() bool { return false }
	dbReadyFunc = func(context.Context) bool { return true }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	readyzHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyzHandler_NotReadyWithoutDB(t *testing.T) {
	prevRegistryReady := registryReadyFunc
	prevDBReady := dbReadyFunc
	t.Cleanup(func() {
		registryReadyFunc = prevRegistryReady
		dbReadyFunc = prevDBReady
	})

	registryReadyFunc = func() bool { return true }
	dbReadyFunc = func(context.Context) bool { return false }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	readyzHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyzHandler_OK(t *testing.T) {
	prevRegistryReady := registryReadyFunc
	prevDBReady := dbReadyFunc
	t.Cleanup(func() {
		registryReadyFunc = prevRegistryReady
		dbReadyFunc = prevDBReady
	})

	registryReadyFunc = func() bool { return true }
	dbReadyFunc = func(context.Context) bool { return true }

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	readyzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != "ready" {
		t.Fatalf("body=%q, want %q", got, "ready")
	}
}
