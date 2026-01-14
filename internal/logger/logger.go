package logger

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	logger *log.Logger
	debug  bool
)

// Init configures JSONL logging into log/app.log.
func Init(baseDir string) error {
	logDir := filepath.Join(baseDir, "log")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "app.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	logger = log.New(f, "", 0)
	return nil
}

func SetDebug(enabled bool) {
	mu.Lock()
	debug = enabled
	mu.Unlock()
}

func Debug(msg string, fields map[string]any) {
	mu.Lock()
	enabled := debug
	mu.Unlock()
	if !enabled {
		return
	}
	write("debug", msg, fields)
}

func Info(msg string, fields map[string]any) {
	write("info", msg, fields)
}

func Warn(msg string, fields map[string]any) {
	write("warn", msg, fields)
}

func Error(msg string, fields map[string]any) {
	write("error", msg, fields)
}

func write(level, msg string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["msg"] = msg
	enc, err := json.Marshal(fields)
	if err != nil {
		logger.Printf(`{"ts":"%s","level":"error","msg":"log_marshal_failed","error":%q}`, time.Now().UTC().Format(time.RFC3339Nano), err.Error())
		return
	}
	logger.Println(string(enc))
}
