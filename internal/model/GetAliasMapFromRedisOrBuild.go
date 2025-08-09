package model

import (
	"YrestAPI/internal/db"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)
func (m *Model) GetAliasMapFromRedisOrBuild(ctx context.Context, modelName string) (error) {
	redisKey := "aliasmap:" + modelName

	// 1. Попытка загрузить из Redis
	cachedStr, err := db.RDB.Get(ctx, redisKey).Result()
	if err == nil {
		var aliasMap AliasMap
		if err := json.Unmarshal([]byte(cachedStr), &aliasMap); err == nil {
			m._AliasMap = &aliasMap // Сохраняем в модель для дальнейшего использования
			return  nil
		}
		// Ошибка в кэше — это уже сбой
		return fmt.Errorf("invalid alias map in Redis for model '%s': %w", modelName, err)
	}

	// 2. Генерация карты на лету
	aliasMap, err := BuildAliasMap(m)
	if err != nil {
		return fmt.Errorf("build alias map failed: %w", err)
	} else {
		m._AliasMap = aliasMap // Сохраняем в модель для дальнейшего использования
		log.Printf("Alias map for model '%s' built successfully", modelName)
	}	
	// 3. Сохраняем в Redis
	jsonData, err := json.Marshal(aliasMap)
	if err != nil {
		return fmt.Errorf("marshal alias map failed: %w", err)
	}
	if err := db.RDB.Set(ctx, redisKey, jsonData, time.Hour*2).Err(); err != nil {
		log.Printf("⚠️ Failed to cache alias map for %s: %v", modelName, err)
	}

	return nil
}