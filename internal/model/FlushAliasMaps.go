package model

import (
	"YrestAPI/internal/db"
	"context"
	"fmt"
)

// FlushAliasMaps удаляет все aliasMap из Redis
func FlushAliasMaps(ctx context.Context) error {
	conn := db.RDB

	// Найдём все ключи aliasmap:*
	iter := conn.Scan(ctx, 0, "aliasmap:*", 1000).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		err := conn.Del(ctx, key).Err()
		if err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan error: %w", err)
	}

	return nil
}

