package itests

import (
	"YrestAPI/internal/db"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// Тест подсчета всех записей модели Person без фильтров
func Test_Count_Person_Item(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// Тело запроса: минимальный валидный для /api/count
	payload := map[string]any{
		"model":   "Person",
		"preset":  "item",
		"filters": map[string]any{},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/count", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/count failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var out map[string]any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	// Ищем поле с количеством
	want := 3 // из фикстур: persons = 3
	got := extractCount(out)
	if got < 0 {
		t.Fatalf("count/total not found in response: %s", string(b))
	}
	if got != want {
		t.Fatalf("wrong count: got %d, want %d; body=%s", got, want, string(b))
	}

	t.Logf("✅ /api/count returned correct count=%d for Person/item", got)
}

func extractCount(m map[string]any) int {
	// Основные варианты
	for _, k := range []string{"count", "total"} {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			}
		}
	}
	// Иногда ответ может быть вложен (например, {\"data\": {\"count\": N}})
	if data, ok := m["data"].(map[string]any); ok {
		if v, ok := data["count"]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			}
		}
	}
	return -1
}

// Тест подсчета контрагентов с фильтром по области адреса (has_many :through)
func Test_Count_Contragent_FilterByAddressArea(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// Истинное значение: contragents, у которых есть адрес в area_id = 1
	// (по сид-данным это должно быть 1 запись: Pacific Trading).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var want int
	const sql = `
		SELECT COUNT(DISTINCT c.id)
		FROM contragents c
		JOIN contragent_addresses ca ON ca.contragent_id = c.id
		JOIN addresses a ON a.id = ca.address_id
		WHERE a.area_id = $1
	`
	if err := db.Pool.QueryRow(ctx, sql, 1).Scan(&want); err != nil {
		t.Fatalf("failed to get expected count from DB: %v", err)
	}

	// Готовим payload для /api/count
	// ВАЖНО: ключ фильтра в точности из твоих логов/движка:
	// "contragent_addresses.address.area_id__in": [1]
	payload := map[string]any{
		"model":  "Contragent",
		"preset": "item",
		"filters": map[string]any{
			"contragent_addresses.address.area_id__in": []int{1},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/count", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/count failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var out map[string]any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	got := extractCount(out)
	if got < 0 {
		t.Fatalf("count/total not found in response: %s", string(b))
	}
	if got != want {
		t.Fatalf("wrong count: got %d, want %d; body=%s", got, want, string(b))
	}

	t.Logf("✅ /api/count with filter area_id=1 returned correct count=%d for Contragent/item", got)
}

// Тест подсчета проектов с фильтром по участникам (has_many :through)
func Test_Count_Project_FilterByPersonLastName(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// Истина из БД: проекты, где есть участник с фамилией = "Chen"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var want int
	const sql = `
		SELECT COUNT(DISTINCT p.id)
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id
		JOIN people pe ON pe.id = pm.person_id
		WHERE pe.last_name = $1
	`
	if err := db.Pool.QueryRow(ctx, sql, "Chen").Scan(&want); err != nil {
		t.Fatalf("failed to get expected count from DB: %v", err)
	}

	// Фильтр через has_many :through (Project -> persons via ProjectMember)
	payload := map[string]any{
		"model":  "Project",
		"preset": "item",
		"filters": map[string]any{
			"persons.last_name__eq": "Chen",
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/count", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/count failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var out map[string]any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	got := extractCount(out)
	if got < 0 {
		t.Fatalf("count/total not found in response: %s", string(b))
	}
	if got != want {
		t.Fatalf("wrong count: got %d, want %d; body=%s", got, want, string(b))
	}

	t.Logf("✅ /api/count with through filter persons.last_name__eq=Chen returned correct count=%d for Project/item", got)
}
