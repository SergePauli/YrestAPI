package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogsHandler_FiltersLastErrors(t *testing.T) {
	tmp := t.TempDir()
	prevLogFilePath := logFilePath
	logFilePath = filepath.Join(tmp, "log", "app.log")
	t.Cleanup(func() {
		logFilePath = prevLogFilePath
	})

	lines := []string{
		`{"ts":"2026-03-25T00:00:00Z","level":"info","msg":"boot"}`,
		`{"ts":"2026-03-25T00:00:01Z","level":"error","msg":"db_failed","error":"dial tcp"}`,
		`{"ts":"2026-03-25T00:00:02Z","level":"warn","msg":"slow_query"}`,
		`{"ts":"2026-03-25T00:00:03Z","level":"error","msg":"resolver_failed","error":"bad field"}`,
		`{"ts":"2026-03-25T00:00:04Z","level":"error","msg":"write_response_failed","error":"broken pipe"}`,
	}
	if err := writeLogFile(logFilePath, lines); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/logs?level=error&limit=2", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"limit":2`) {
		t.Fatalf("body=%q does not include limit", body)
	}
	if !strings.Contains(body, `"msg":"resolver_failed"`) {
		t.Fatalf("body=%q missing older matching entry", body)
	}
	if !strings.Contains(body, `"msg":"write_response_failed"`) {
		t.Fatalf("body=%q missing latest matching entry", body)
	}
	if strings.Contains(body, `"msg":"db_failed"`) {
		t.Fatalf("body=%q should not include third-latest match", body)
	}
}

func TestLogsHandler_FiltersByMessageSubstring(t *testing.T) {
	tmp := t.TempDir()
	prevLogFilePath := logFilePath
	logFilePath = filepath.Join(tmp, "log", "app.log")
	t.Cleanup(func() {
		logFilePath = prevLogFilePath
	})

	lines := []string{
		`{"ts":"2026-03-25T00:00:00Z","level":"error","msg":"db_failed"}`,
		`{"ts":"2026-03-25T00:00:01Z","level":"error","msg":"resolver_failed"}`,
	}
	if err := writeLogFile(logFilePath, lines); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/logs?msg=resolver", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"msg":"resolver_failed"`) {
		t.Fatalf("body=%q missing resolver match", body)
	}
	if strings.Contains(body, `"msg":"db_failed"`) {
		t.Fatalf("body=%q unexpectedly contains db_failed", body)
	}
}

func TestLogsHandler_FiltersByExactKey(t *testing.T) {
	tmp := t.TempDir()
	prevLogFilePath := logFilePath
	logFilePath = filepath.Join(tmp, "log", "app.log")
	t.Cleanup(func() {
		logFilePath = prevLogFilePath
	})

	lines := []string{
		`{"ts":"2026-03-25T00:00:00Z","level":"error","msg":"db_failed"}`,
		`{"ts":"2026-03-25T00:00:01Z","level":"error","msg":"resolver_failed","error":"bad field"}`,
	}
	if err := writeLogFile(logFilePath, lines); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/logs?key=error", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"msg":"resolver_failed"`) {
		t.Fatalf("body=%q missing entry with error key", body)
	}
	if strings.Contains(body, `"msg":"db_failed"`) {
		t.Fatalf("body=%q unexpectedly contains entry without error key", body)
	}
}

func TestLogsHandler_FiltersByPartialValueAcrossFields(t *testing.T) {
	tmp := t.TempDir()
	prevLogFilePath := logFilePath
	logFilePath = filepath.Join(tmp, "log", "app.log")
	t.Cleanup(func() {
		logFilePath = prevLogFilePath
	})

	lines := []string{
		`{"ts":"2026-03-25T00:00:00Z","level":"error","msg":"db_failed","error":"dial tcp 10.0.0.5:5432"}`,
		`{"ts":"2026-03-25T00:00:01Z","level":"error","msg":"resolver_failed","error":"bad field"}`,
	}
	if err := writeLogFile(logFilePath, lines); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/logs?value=10.0.0.5", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"msg":"db_failed"`) {
		t.Fatalf("body=%q missing value match", body)
	}
	if strings.Contains(body, `"msg":"resolver_failed"`) {
		t.Fatalf("body=%q unexpectedly contains unmatched entry", body)
	}
}

func TestLogsHandler_InvalidLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/debug/logs?level=fatal", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogsHandler_LogFileNotFound(t *testing.T) {
	tmp := t.TempDir()
	prevLogFilePath := logFilePath
	logFilePath = filepath.Join(tmp, "log", "missing.log")
	t.Cleanup(func() {
		logFilePath = prevLogFilePath
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/logs?level=error&limit=5", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestWithDebugToken_AllowsValidToken(t *testing.T) {
	h := withDebugToken("secret-token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/logs", nil)
	req.Header.Set("X-Debug-Token", "secret-token")
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusOK)
	}
}

func TestWithDebugToken_RejectsMissingToken(t *testing.T) {
	h := withDebugToken("secret-token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/logs", nil)
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWithDebugToken_RejectsInvalidToken(t *testing.T) {
	h := withDebugToken("secret-token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/logs", nil)
	req.Header.Set("X-Debug-Token", "wrong-token")
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWithDebugToken_RejectsWhenNotConfigured(t *testing.T) {
	h := withDebugToken("", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/debug/logs", nil)
	req.Header.Set("X-Debug-Token", "secret-token")
	w := httptest.NewRecorder()

	h(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
