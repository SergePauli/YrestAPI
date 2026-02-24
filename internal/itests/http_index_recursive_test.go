package itests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"YrestAPI/internal/db"
)

func Test_Index_Contract_RecursiveChain_RespectsMaxDepth(t *testing.T) {
	if testBaseURL == "" || httpSrv == nil {
		t.Fatal("bootstrap not ready: HTTP server/baseURL missing")
	}

	const maxDepth = 3 // max recursive relation hops from the root node
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var firstID, lastID int
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM contracts ORDER BY id ASC LIMIT 1`).Scan(&firstID); err != nil {
		t.Fatalf("failed to fetch first contract id: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM contracts ORDER BY id DESC LIMIT 1`).Scan(&lastID); err != nil {
		t.Fatalf("failed to fetch last contract id: %v", err)
	}

	wantPrev := expectedChainPrevNumbers(t, ctx, lastID, maxDepth)
	gotPrev := fetchChainNumbers(t, "chain_prev", lastID, "prev")
	if !reflect.DeepEqual(gotPrev, wantPrev) {
		t.Fatalf("chain_prev mismatch: got %v, want %v", gotPrev, wantPrev)
	}

	wantNext := expectedChainNextNumbers(t, ctx, firstID, maxDepth)
	gotNext := fetchChainNumbers(t, "chain_next", firstID, "next")
	if !reflect.DeepEqual(gotNext, wantNext) {
		t.Fatalf("chain_next mismatch: got %v, want %v", gotNext, wantNext)
	}
}

func expectedChainPrevNumbers(t *testing.T, ctx context.Context, startID, maxDepth int) []string {
	t.Helper()
	rows, err := db.Pool.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, number, prev_contract_id, 1 AS depth
			FROM contracts
			WHERE id = $1
			UNION ALL
			SELECT c.id, c.number, c.prev_contract_id, chain.depth + 1
			FROM contracts c
			JOIN chain ON c.id = chain.prev_contract_id
			WHERE chain.depth < $2
		)
		SELECT number
		FROM chain
		ORDER BY depth ASC
	`, startID, maxDepth+1)
	if err != nil {
		t.Fatalf("failed to build expected prev-chain: %v", err)
	}
	defer rows.Close()
	return scanStrings(t, rows)
}

func expectedChainNextNumbers(t *testing.T, ctx context.Context, startID, maxDepth int) []string {
	t.Helper()
	rows, err := db.Pool.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, number, 1 AS depth
			FROM contracts
			WHERE id = $1
			UNION ALL
			SELECT c.id, c.number, chain.depth + 1
			FROM contracts c
			JOIN chain ON c.prev_contract_id = chain.id
			WHERE chain.depth < $2
		)
		SELECT number
		FROM chain
		ORDER BY depth ASC
	`, startID, maxDepth+1)
	if err != nil {
		t.Fatalf("failed to build expected next-chain: %v", err)
	}
	defer rows.Close()
	return scanStrings(t, rows)
}

func fetchChainNumbers(t *testing.T, preset string, rootID int, relKey string) []string {
	t.Helper()
	payload := map[string]any{
		"model":  "Contract",
		"preset": preset,
		"filters": map[string]any{
			"id__eq": rootID,
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

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. body=%s", resp.StatusCode, string(respBody))
	}
  
	var raw any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		t.Fatalf("invalid JSON response: %v; body=%s", err, string(respBody))
	}
	items, err := extractItemsArray(raw)
	if err != nil {
		t.Fatalf("extract items failed: %v; body=%s", err, string(respBody))
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d; body=%s", len(items), string(respBody))
	}

	numbers := make([]string, 0, 4)
	cur := items[0]
	first := true
	for {
		if first {
			id, ok := asInt(cur["id"])
			if !ok || id <= 0 {
				t.Fatalf("root node id has unexpected type/value: %T (%v), node=%#v, body=%s", cur["id"], cur["id"], cur, string(respBody))
			}
			first = false
		} else if cur["id"] != nil {
			id, ok := asInt(cur["id"])
			if !ok || id <= 0 {
				t.Fatalf("nested node id has unexpected type/value: %T (%v), node=%#v, body=%s", cur["id"], cur["id"], cur, string(respBody))
			}
		}

		areaAny, exists := cur["area"]
		if !exists || areaAny == nil {
			t.Fatalf("node area missing: node=%#v, body=%s", cur, string(respBody))
		}
		area, ok := areaAny.(map[string]any)
		if !ok {
			t.Fatalf("node area must be object, got %T (%v), node=%#v, body=%s", areaAny, areaAny, cur, string(respBody))
		}
		areaID, ok := asInt(area["id"])
		if !ok || areaID <= 0 {
			t.Fatalf("area.id has unexpected type/value: %T (%v), area=%#v, node=%#v", area["id"], area["id"], area, cur)
		}
		areaName, ok := area["name"].(string)
		if !ok || areaName == "" {
			t.Fatalf("area.name has unexpected type/value: %T (%v), area=%#v, node=%#v", area["name"], area["name"], area, cur)
		}

		number, ok := cur["number"].(string)
		if !ok {
			t.Fatalf("node number has unexpected type: %T (%v), node=%#v, body=%s", cur["number"], cur["number"], cur, string(respBody))
		}
		numbers = append(numbers, number)

		next, exists := cur[relKey]
		if !exists || next == nil {
			break
		}
		node, ok := next.(map[string]any)
		if !ok {
			t.Fatalf("%s node must be object or null, got %T (%v)", relKey, next, next)
		}
		cur = node
	}
	return numbers
}

func scanStrings(t *testing.T, rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) []string {
	t.Helper()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan value: %v", err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return out
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
