package router

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultLogLimit = 20
	maxLogLimit     = 100
)

var logFilePath = filepath.Join("log", "app.log")

type logEntry map[string]any

type logFilters struct {
	Level string `json:"level,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
	Msg   string `json:"msg,omitempty"`
	Limit int    `json:"limit"`
}

func logsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filters, err := parseLogFilters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries, err := readLogTail(logFilePath, filters)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "log file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to read logs", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"filters": filters,
		"entries": entries,
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func withDebugToken(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(token)
		if expected == "" {
			http.Error(w, "debug logs token is not configured", http.StatusServiceUnavailable)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Debug-Token"))
		if got == "" {
			http.Error(w, "missing X-Debug-Token header", http.StatusUnauthorized)
			return
		}
		if got != expected {
			http.Error(w, "invalid debug token", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func parseLogFilters(r *http.Request) (logFilters, error) {
	q := r.URL.Query()
	filters := logFilters{
		Level: strings.ToLower(strings.TrimSpace(q.Get("level"))),
		Key:   strings.TrimSpace(q.Get("key")),
		Value: strings.TrimSpace(q.Get("value")),
		Msg:   strings.TrimSpace(q.Get("msg")),
		Limit: defaultLogLimit,
	}

	if filters.Level != "" {
		switch filters.Level {
		case "debug", "info", "warn", "error":
		default:
			return logFilters{}, errors.New("invalid level")
		}
	}

	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLogLimit {
			return logFilters{}, errors.New("invalid limit")
		}
		filters.Limit = n
	}

	return filters, nil
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func readLogTail(path string, filters logFilters) ([]logEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	var (
		entries   []logEntry
		remainder []byte
		offset          = stat.Size()
		bufSize   int64 = 4096
	)

	for offset > 0 && len(entries) < filters.Limit {
		chunkSize := bufSize
		if offset < chunkSize {
			chunkSize = offset
		}
		offset -= chunkSize

		buf := make([]byte, chunkSize)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return nil, err
		}

		buf = append(buf, remainder...)
		lines := bytes.Split(buf, []byte{'\n'})
		remainder = append(remainder[:0], lines[0]...)

		for i := len(lines) - 1; i >= 1 && len(entries) < filters.Limit; i-- {
			entry, ok := parseLogLine(lines[i], filters)
			if ok {
				entries = append(entries, entry)
			}
		}
	}

	if len(remainder) > 0 && len(entries) < filters.Limit {
		if entry, ok := parseLogLine(remainder, filters); ok {
			entries = append(entries, entry)
		}
	}

	reverseEntries(entries)
	return entries, nil
}

func parseLogLine(line []byte, filters logFilters) (logEntry, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, false
	}

	var entry logEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, false
	}

	if filters.Level != "" {
		level, _ := entry["level"].(string)
		if strings.ToLower(level) != filters.Level {
			return nil, false
		}
	}

	if filters.Msg != "" {
		msg, _ := entry["msg"].(string)
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(filters.Msg)) {
			return nil, false
		}
	}

	if filters.Key != "" && !entryHasKey(entry, filters.Key) {
		return nil, false
	}

	if filters.Value != "" && !entryHasValue(entry, filters.Value) {
		return nil, false
	}

	return entry, true
}

func entryHasKey(entry logEntry, key string) bool {
	_, ok := entry[key]
	return ok
}

func entryHasValue(entry logEntry, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return true
	}
	for _, value := range entry {
		if containsValue(value, needle) {
			return true
		}
	}
	return false
}

func containsValue(v any, needle string) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.Contains(strings.ToLower(x), needle)
	case []any:
		for _, item := range x {
			if containsValue(item, needle) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, item := range x {
			if containsValue(item, needle) {
				return true
			}
		}
		return false
	default:
		return strings.Contains(strings.ToLower(fmt.Sprint(x)), needle)
	}
}

func reverseEntries(entries []logEntry) {
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
}

func writeLogFile(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		if _, err := w.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}
