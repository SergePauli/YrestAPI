package itests

import (
	"YrestAPI/internal"
	"YrestAPI/internal/config"
	"YrestAPI/internal/db"
	"YrestAPI/internal/model"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	composeProject   = "yrestapi-itest"
	testHTTPPort     = "18080"
	testPostgresPort = "15432"
	composeTimeout   = 2 * time.Minute
)

var (
	testBaseURL       string
	httpSrv           *http.Server
	managedHTTPServer bool
)

func TestMain(m *testing.M) {
	root, err := internal.FindRepoRoot()
	if err != nil {
		println("findRepoRoot failed:", err.Error())
		os.Exit(1)
	}

	if !dockerComposeAvailable(root) {
		println("skip integration tests: docker compose is not available")
		os.Exit(0)
	}

	teardownCompose, err := setupComposeStack(root)
	if err != nil {
		println("setup compose stack failed:", err.Error())
		os.Exit(1)
	}

	cfg := config.LoadConfig()
	cfg.PostgresDSN = fmt.Sprintf("postgres://postgres:postgres@localhost:%s/app?sslmode=disable", testPostgresPort)
	cfg.ModelsDir = filepath.Join(root, "test_db")

	if err := db.InitPostgres(cfg.PostgresDSN); err != nil {
		println("InitPostgres failed:", err.Error())
		os.Exit(1)
	}

	if err := model.InitRegistry(cfg.ModelsDir); err != nil {
		println("InitRegistry failed:", err.Error())
		os.Exit(1)
	}

	testBaseURL = fmt.Sprintf("http://localhost:%s", testHTTPPort)
	httpSrv = &http.Server{}
	managedHTTPServer = false

	var ok bool
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name='people')`,
	).Scan(&ok); err != nil {
		log.Printf("sanity check failed: %v", err)
	} else {
		log.Printf("people table exists: %v", ok)
	}

	code := m.Run()

	if managedHTTPServer && httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = httpSrv.Shutdown(ctx)
		cancel()
	}

	if err := teardownCompose(); err != nil {
		log.Printf("compose teardown failed: %v", err)
	}

	os.Exit(code)
}

func dockerComposeAvailable(root string) bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}

	cmd := exec.Command("docker", "compose", "version")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return false
	}

	cmd = exec.Command("docker", "info")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

func setupComposeStack(root string) (func() error, error) {
	down := func() error {
		return runCompose(root, "down", "-v", "--remove-orphans")
	}

	_ = down()
	if err := runCompose(root, "up", "--build", "-d"); err != nil {
		return nil, explainComposePortConflict(err)
	}

	if err := waitForTCP("localhost", testPostgresPort, 40*time.Second); err != nil {
		_ = down()
		return nil, fmt.Errorf("postgres port not ready: %w", err)
	}

	if err := waitForTCP("localhost", testHTTPPort, 40*time.Second); err != nil {
		_ = down()
		return nil, fmt.Errorf("service port not ready: %w", err)
	}

	if err := waitForHTTPStatus(
		fmt.Sprintf("http://localhost:%s/healthz", testHTTPPort),
		http.StatusOK,
		40*time.Second,
	); err != nil {
		_ = down()
		return nil, fmt.Errorf("healthz not ready: %w", err)
	}

	if err := waitForHTTPStatus(
		fmt.Sprintf("http://localhost:%s/readyz", testHTTPPort),
		http.StatusOK,
		40*time.Second,
	); err != nil {
		_ = down()
		return nil, fmt.Errorf("readyz not ready: %w", err)
	}

	return down, nil
}

func runCompose(root string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), composeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "-p", composeProject}, args...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOST_PORT="+testHTTPPort,
		"PORT=8080",
		"POSTGRES_PORT="+testPostgresPort,
		"POSTGRES_DB=app",
		"POSTGRES_USER=postgres",
		"POSTGRES_PASSWORD=postgres",
		"POSTGRES_DSN=postgres://postgres:postgres@db:5432/app?sslmode=disable",
		"MODELS_DIR=/app/test_db",
		"LOCALE=en",
		"AUTH_ENABLED=false",
	)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("docker %v timed out after %s\n%s", append([]string{"compose", "-p", composeProject}, args...), composeTimeout, string(out))
	}
	if err != nil {
		return fmt.Errorf("docker %v failed: %w\n%s", append([]string{"compose", "-p", composeProject}, args...), err, string(out))
	}
	return nil
}

func explainComposePortConflict(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	if !strings.Contains(msg, "address already in use") && !strings.Contains(msg, "port is already allocated") {
		return err
	}

	return fmt.Errorf(
		"docker compose could not start because one of the test ports is already in use.\n"+
			"Current test ports: HTTP host port %s, Postgres host port %s.\n"+
			"Change them in %s constants `testHTTPPort` and `testPostgresPort`, or free the conflicting local ports.\n\nOriginal error:\n%w",
		testHTTPPort,
		testPostgresPort,
		"/home/serge/Projects/YrestAPI/internal/itests/main_test.go",
		err,
	)
}

func waitForHTTPStatus(url string, want int, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("%s did not return %d within %s", url, want, timeout)
}

func waitForTCP(host, port string, timeout time.Duration) error {
	addr := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("%s not reachable within %s", addr, timeout)
}
