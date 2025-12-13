package itests

import (
	"YrestAPI/internal"
	"YrestAPI/internal/config"
	"YrestAPI/internal/db" // где лежит db.InitPostgres
	"YrestAPI/internal/model"
	"YrestAPI/internal/router"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var (
	testBaseURL string
	httpSrv     *http.Server
)

func TestMain(m *testing.M) {
	cfg := config.LoadConfig()

	teardownDB, err := SetupAndTeardownTestDB(cfg.PostgresDSN, db.InitPostgres)
	log.Printf("TestMain: setup test DB")
	if err != nil {
		// печатаем и выходим кодом 1, чтобы CI/локально это было видно
		println("setup test DB failed:", err.Error())
		os.Exit(1)
	}
	
	// 2) Указываем каталог тестовых моделей
	root, err := internal.FindRepoRoot()
	if err != nil {
		println("❌ findRepoRoot failed:", err.Error())
		os.Exit(1)
	}
	cfg.ModelsDir = filepath.Join(root, "test_db")

	// 3) Пытаемся загрузить реестр
	if err := model.InitRegistry(cfg.ModelsDir); err != nil {
		println("❌ InitRegistry failed:", err.Error())
		os.Exit(1) // критично: прекращаем ВЕСЬ пакет тестов
	}
	println("✅ Registry initialized from:", cfg.ModelsDir)

	// 3) Поднимаем HTTP-сервис на порту из конфига
	router.InitRoutes() // регистрирует маршруты на http.DefaultServeMux (ожидается)
	httpSrv = &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: http.DefaultServeMux,
	}
	go func() {
		// ListenAndServe вернет ошибку только при фатальном сбое или Shutdown
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			println("❌ HTTP server failed:", err.Error())
			os.Exit(1)
		}
	}()

	// ждём, пока порт начнет слушаться
	if err := waitForPort("localhost", cfg.Port, 3*time.Second); err != nil {
		println("❌ HTTP port not ready:", err.Error())
		_ = httpSrv.Close()
		os.Exit(1)
	}
	testBaseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	println("🚀 HTTP started at", testBaseURL)

	var ok bool
if err := db.Pool.QueryRow(context.Background(),
    `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='people')`,
).Scan(&ok); err != nil {
    log.Printf("sanity check failed: %v", err)
} else {
    log.Printf("people table exists: %v", ok)
}
	// На этом шаге можно сразу выйти, если "до тестов далеко".
	// Но чтобы `go test` был доволен, прогоняем m.Run().
	code := m.Run()

	// явный порядок завершения: сначала HTTP, потом БД, потом Exit
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    _ = httpSrv.Shutdown(ctx)
    cancel()

    if err := teardownDB(); err != nil {
        println("⚠️ drop test DB failed:", err.Error())
    } else {
        log.Printf("TestMain: test DB dropped")
    }
	os.Exit(code)
}

func waitForPort(host, port string, timeout time.Duration) error {
	addr := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("port %s not reachable within %s", port, timeout)
}