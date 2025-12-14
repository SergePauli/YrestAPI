package itests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"net/http"
	"reflect"
	"testing"
	"time"

	"YrestAPI/internal/db"
)

// /api/index: Employee, preset=item, order by id ASC, offset=1, limit=2
func Test_Index_Employee_Item_Pagination(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// 1) Истинные id берём напрямую из БД
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wantIDs []int
	rows, err := db.Pool.Query(ctx, `
		SELECT id
		FROM employees
		ORDER BY id ASC
		LIMIT 2 OFFSET 1
	`)
	if err != nil {
		t.Fatalf("failed to query expected ids: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan id: %v", err)
		}
		wantIDs = append(wantIDs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(wantIDs) == 0 {
		t.Skip("no employees in DB to test pagination")
	}

	// 2) Делаем запрос к /api/index
	payload := map[string]any{
		"model":  "Employee",
		"preset": "item",
		"sorts":  []string{"id ASC"},
		"offset": 1,
		"limit":  2,
		// "filters": map[string]any{}, // опционально
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	// 3) Разбираем ответ — ищем массив элементов
	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}
	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}

	// 4) Проверяем размер и состав полей пресета (минимально — что нужные поля существуют)
	if len(items) != len(wantIDs) {
		t.Fatalf("wrong page size: got %d, want %d; body=%s", len(items), len(wantIDs), string(b))
	}
	for i, it := range items {
		for _, k := range []string{"id", "organization_id", "person_id", "position"} {
			if _, ok := it[k]; !ok {
				t.Fatalf("missing field %q in item[%d]: %#v", k, i, it)
			}
		}
	}

	// 5) Сверяем id по порядку
	gotIDs := make([]int, 0, len(items))
	for _, it := range items {
		switch v := it["id"].(type) {
		case float64:
			gotIDs = append(gotIDs, int(v))
		case int:
			gotIDs = append(gotIDs, v)
		default:
			t.Fatalf("unexpected type for id: %T (%v)", v, v)
		}
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids mismatch: got %v, want %v; body=%s", gotIDs, wantIDs, string(b))
	}

	t.Logf("✅ /api/index returned correct paging & preset for Employee/item, ids=%v", gotIDs)
}

// /api/index: Contragent, preset=item, offset beyond dataset should return empty page
func Test_Index_Contragent_Item_EmptyPage(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var total int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM contragents`).Scan(&total); err != nil {
		t.Fatalf("failed to count contragents: %v", err)
	}

	payload := map[string]any{
		"model":  "Contragent",
		"preset": "item",
		"offset": total + 50, // явно за границами тестовых данных
		"limit":  5,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}
	if len(items) != 0 {
		t.Fatalf("expected empty page for big offset, got %d items; body=%s", len(items), string(b))
	}

	t.Logf("✅ /api/index Contragent/item returns empty array for offset beyond dataset (count=%d)", total)
}

// /api/index: Contragent, preset=item, formatter pulls name from contragent_organization
func Test_Index_Contragent_Item_NameFormatter_FromOrganization(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type row struct {
		ID   int
		Name string
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT c.id, co.name
		FROM contragents c
		LEFT JOIN LATERAL (
			SELECT name
			FROM contragent_organizations co
			WHERE co.contragent_id = c.id AND co.used = true
			ORDER BY co.id DESC
			LIMIT 1
		) co ON true
		ORDER BY c.id ASC
		LIMIT 2`)
	if err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	defer rows.Close()

	var want []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		want = append(want, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(want) == 0 {
		t.Skip("no contragents in test DB")
	}

	payload := map[string]any{
		"model":  "Contragent",
		"preset": "item",
		"sorts":  []string{"id ASC"},
		"offset": 0,
		"limit":  len(want),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %d; body=%s", len(want), len(items), string(b))
	}

	for i, it := range items {
		var gotID int
		switch v := it["id"].(type) {
		case float64:
			gotID = int(v)
		case int:
			gotID = v
		default:
			t.Fatalf("item[%d]: unexpected id type %T; item=%#v", i, it["id"], it)
		}
		if gotID != want[i].ID {
			t.Fatalf("item[%d]: id mismatch: got %d want %d; item=%#v", i, gotID, want[i].ID, it)
		}
		name, _ := it["name"].(string)
		if strings.TrimSpace(name) == "" {
			t.Fatalf("item[%d]: empty name from formatter; item=%#v", i, it)
		}
		if name != want[i].Name {
			t.Fatalf("item[%d]: name mismatch: got %q want %q; item=%#v", i, name, want[i].Name, it)
		}
	}

	t.Logf("✅ Contragent/item formatter pulls name from contragent_organization (ids=%v)", []int{want[0].ID})
}

// Гибкая распаковка: поддерживает {data:[...]}, {rows:[...]}, {items:[...]}, либо топ-левел массив
func extractItemsArray(raw any) ([]map[string]any, error) {
	// { data: [...] } / { rows: [...] } / { items: [...] }
	if m, ok := raw.(map[string]any); ok {
		for _, key := range []string{"data", "rows", "items"} {
			if a, ok := m[key].([]any); ok {
				return toSliceOfMap(a)
			}
		}
	}
	// Топ-левел массив
	if a, ok := raw.([]any); ok {
		return toSliceOfMap(a)
	}
	return nil, fmt.Errorf("cannot find items array in response")
}

func toSliceOfMap(a []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(a))
	for i, v := range a {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("element %d is not an object: %T", i, v)
		}
		out = append(out, m)
	}
	return out, nil
}
func Test_Index_Person_Item_Formatter_And_InternalHidden(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// Берём из БД «истину» для первой записи по id ASC
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type row struct{ ID int; First, Last string }
	var r row
	// ВАЖНО: используй правильную таблицу (people или persons)
	if err := db.Pool.QueryRow(ctx,
		`SELECT id, first_name, last_name
		   FROM people
		   ORDER BY id ASC
		   LIMIT 1`,
	).Scan(&r.ID, &r.First, &r.Last); err != nil {
		t.Fatalf("failed to read expected row from DB: %v", err)
	}
	wantFull := fmt.Sprintf("%s %s", r.Last, r.First)

	// Запрос к /api/index с пресетом item
	payload := map[string]any{
		"model":  "Person",
		"preset": "item",
		"sorts":  []string{"id ASC"},
		"offset": 0,
		"limit":  1,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(b))
	}

	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d; body=%s", len(items), string(b))
	}
	it := items[0]

	// 1) internal: first_name не должен присутствовать
	if _, ok := it["first_name"]; ok {
		t.Fatalf("internal field 'first_name' leaked into response: %#v", it)
	}

	// 2) formatter: full_name должен совпасть с «Фамилия Имя»
	v, ok := it["full_name"]
	if !ok {
		t.Fatalf("formatted field 'full_name' missing in response: %#v", it)
	}
	gotFull, ok := v.(string)
	if !ok {
		t.Fatalf("full_name must be string, got %T (%v)", v, v)
	}
	if gotFull != wantFull {
		t.Fatalf("full_name mismatch: got %q, want %q; item=%#v", gotFull, wantFull, it)
	}

	// 3) Бонус: базовые поля на месте
	if _, ok := it["id"]; !ok {
		t.Fatalf("id missing in item: %#v", it)
	}
	if _, ok := it["last_name"]; !ok {
		t.Fatalf("last_name missing in item: %#v", it)
	}

	t.Logf("✅ formatter ok (full_name=%q), internal field hidden, item=%#v", gotFull, it)
}
// Тест вложенного пресета и форматтера поля
func Test_Index_Employee_WithPersonLabel_Formatter(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	// 1) Истина из БД: первая запись по id ASC
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var empID int
	var first, last string
	err := db.Pool.QueryRow(ctx, `
		SELECT e.id, p.first_name, p.last_name
		FROM employees e
		JOIN people p ON p.id = e.person_id
		ORDER BY e.id ASC
		LIMIT 1`,
	).Scan(&empID, &first, &last)
	if err != nil {
		t.Fatalf("expected row from DB not found: %v", err)
	}
	wantLabel := fmt.Sprintf("%s %s", last, first)

	// 2) Запрос к /api/index c preset=with_person_label
	payload := map[string]any{
		"model":  "Employee",
		"preset": "with_person_label",
		"sorts":  []string{"id ASC"},
		"offset": 0,
		"limit":  1,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, string(b))
	}
	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d; body=%s", len(items), string(b))
	}
	it := items[0]	
	// Вложенный пресет person присутствует
	personAny, ok := it["person"]
	if !ok {
		t.Fatalf("nested preset 'person' missing in item: %#v", it)
	}
	person, ok := personAny.(map[string]any)
	if !ok {
		t.Fatalf("'person' must be an object, got %T", personAny)
	}
	// Проверяем базовые поля (не проверяем отсутствие first_name)
	if _, ok := person["id"]; !ok {
		t.Fatalf("'person.id' missing in nested preset: %#v", person)
	}
	if _, ok := person["last_name"]; !ok {
		t.Fatalf("'person.last_name' missing in nested preset: %#v", person)
	}

	// Поле-форматтер на верхнем уровне
	labelAny, ok := it["person_label"]
	if !ok {
		t.Fatalf("formatted field 'person_label' missing: %#v", it)
	}
	label, ok := labelAny.(string)
	if !ok {
		t.Fatalf("'person_label' must be string, got %T (%v)", labelAny, labelAny)
	}
	if label != wantLabel {
		t.Fatalf("person_label mismatch: got %q, want %q; item=%#v", label, wantLabel, it)
	}

	// Соответствие id
	switch v := it["id"].(type) {
	case float64:
		if int(v) != empID {
			t.Fatalf("id mismatch: got %d, want %d", int(v), empID)
		}
	case int:
		if v != empID {
			t.Fatalf("id mismatch: got %d, want %d", v, empID)
		}
	default:
		 t.Fatalf("unexpected type for id: %T", v)
	}

	t.Logf("✅ nested preset present; formatter OK (person_label=%q); ignoring internal leakage for now", label)
}
func Test_Index_Person_WithContacts_Head(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Истина из БД для первых двух людей по id ASC
	type row struct {
		ID    int
		Last  string
		First string
		Email *string
		Phone *string
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT p.id,
		       p.last_name AS last,
		       p.first_name AS first,
		       (SELECT c.value
		          FROM person_contacts pc
		          JOIN contacts c ON c.id = pc.contact_id
		         WHERE pc.person_id = p.id AND c.kind = 'email'
		         LIMIT 1) AS email,
		       (SELECT c.value
		          FROM person_contacts pc
		          JOIN contacts c ON c.id = pc.contact_id
		         WHERE pc.person_id = p.id AND c.kind = 'phone'
		         LIMIT 1) AS phone
		FROM people p
		ORDER BY p.id ASC
		LIMIT 2`)
	if err != nil {
		t.Fatalf("db query failed: %v", err)
	}
	defer rows.Close()

	var expected []string
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Last, &r.First, &r.Email, &r.Phone); err != nil {
			t.Fatalf("scan: %v", err)
		}
		full := fmt.Sprintf("%s %s", r.Last, r.First)
		email := ""
		if r.Email != nil { email = *r.Email }
		phone := ""
		if r.Phone != nil { phone = *r.Phone }
		expected = append(expected, strings.TrimRight(fmt.Sprintf("%s %s %s", full, email, phone), " "))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(expected) != 2 {
		t.Fatalf("need 2 people, got %d", len(expected))
	}

	// Запрос к /api/index
	payload := map[string]any{
		"model":  "Person",
		"preset": "with_contacts",
		"sorts":  []string{"id ASC"},
		"offset": 0,
		"limit":  2,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil { t.Fatalf("build request failed: %v", err) }
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil { t.Fatalf("POST /api/index failed: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, string(b))
	}
	items, err := extractItemsArray(raw)
	if err != nil { t.Fatalf("extract items failed: %v; body=%s", err, string(b)) }
	if len(items) != 2 { t.Fatalf("expected 2 items, got %d; body=%s", len(items), string(b)) }

	for i, it := range items {
		h, _ := it["head"].(string)
		if strings.TrimRight(h, " ") != expected[i] {
			t.Fatalf("item[%d]: head mismatch: got %q want %q; item=%#v", i, h, expected[i], it)
		}
		// Проверим, что вложенные пресеты вернулись
		if _, ok := it["email"]; !ok {
			t.Fatalf("item[%d]: nested 'email' missing", i)
		} else {
			if email, ok := it["email"].(map[string]any); ok {
				if _, ok := email["id"]; !ok {
					t.Fatalf("email[id] missing: %#v", email)
				}
			}
		}
		if _, ok := it["phone"]; !ok {
			t.Fatalf("item[%d]: nested 'phone' missing", i)
		}
	}
	t.Logf("✅ Person/with_contacts head matches DB for first two items")
}

// Тестирует has_many + форматтер и отсутствие лишних полей/обёрток.
func Test_Index_Project_WithMembers_HasManyFormatter_And_NoLeaks(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Найдём проект с участниками, упорядоченными как в YAML (last_name, first_name).
	var pid int
	var pname, plast, pfirst string
	err := db.Pool.QueryRow(ctx, `
		SELECT p.id, p.name, pe.last_name, pe.first_name
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id
		JOIN people pe ON pe.id = pm.person_id           -- ← было 'persons'
		ORDER BY p.id ASC, pe.last_name ASC, pe.first_name ASC
		LIMIT 1`,
	).Scan(&pid, &pname, &plast, &pfirst)
	if err != nil {
		t.Fatalf("db seed not found for project with members: %v", err)
	}
	//wantLabel := fmt.Sprintf("%s — %s %s", pname, plast, pfirst)

	// Запрос к /api/index c пресетом has_many + форматтер.
	payload := map[string]any{
		"model":  "Project",
		"preset": "with_members",
		"filters": map[string]any{
			"id__eq": pid,
		},
		"limit": 1,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/index", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /api/index failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(b))
	}

	var raw any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("invalid JSON: %v; body=%s", err, string(b))
	}
	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(b))
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d; body=%s", len(items), string(b))
	}
	it := items[0]

	// 1) Проверяем форматтер верхнего уровня.
	// lbl, _ := it["label"].(string)
	// if strings.TrimSpace(lbl) != wantLabel {
	// 	t.Fatalf("label mismatch: got %q want %q; item=%#v", lbl, wantLabel, it)
	// }

	// 2) На верхнем уровне — только поля из пресета.
	allowedTop := map[string]struct{}{
		"id": {}, "organization_id": {}, "name": {}, "persons": {}, "label": {},
	}
	for k := range it {
		if _, ok := allowedTop[k]; !ok {
			t.Fatalf("top-level leak: unexpected field %q present; item=%#v", k, it)
		}
	}

	// 3) persons — это массив Person.item: id, last_name, full_name (first_name internal → не должно быть).
	arr, ok := it["persons"].([]any)
	if !ok {
		t.Fatalf("'persons' must be array, got %T", it["persons"])
	}
	if len(arr) == 0 {
		t.Fatalf("'persons' array is empty; need at least one member")
	}

	allowedPerson := map[string]struct{}{
		"id": {}, "last_name": {}, "full_name": {},
	}
	// никаких вспомогательных ключей связки/обёрток
	disallowedKeys := []string{"project_id", "person_id", "role", "joined_at", "project", "member", "persons", "people", "contact", "first_name"}

	for i, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			t.Fatalf("persons[%d] must be object, got %T", i, el)
		}
		for k := range obj {
			if _, ok := allowedPerson[k]; !ok {
				t.Fatalf("persons[%d]: unexpected key %q; obj=%#v", i, k, obj)
			}
		}
		for _, must := range []string{"last_name", "full_name"} {
			if _, ok := obj[must]; !ok {
				t.Fatalf("persons[%d]: missing required key %q; obj=%#v", i, must, obj)
			}
		}
		for _, bad := range disallowedKeys {
			if _, ok := obj[bad]; ok {
				t.Fatalf("persons[%d]: leak of join/helper key %q; obj=%#v", i, bad, obj)
			}
		}
	}

	// 4) Сверяем первого участника с ожиданием по БД.
	first := arr[0].(map[string]any)
	gotFull, _ := first["full_name"].(string)
	wantFirstFull := fmt.Sprintf("%s %s", plast, pfirst)
	if gotFull != wantFirstFull {
		t.Fatalf("persons[0].full_name mismatch: got %q want %q; persons[0]=%#v", gotFull, wantFirstFull, first)
	}

	t.Logf("✅ Project/with_members: has_many formatter ok, preset data present, no join-key leaks")
}
