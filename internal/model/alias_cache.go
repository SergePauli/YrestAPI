package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"YrestAPI/internal/logger"
)

const (
	aliasCacheTTL       = 7 * 24 * time.Hour
	aliasCacheSweepFreq = time.Hour
)

type aliasCacheEntry struct {
	aliasMap  *AliasMap
	lastUsed  time.Time
	createdAt time.Time
}

type aliasMapCache struct {
	mu        sync.Mutex
	items     map[string]*aliasCacheEntry
	lastSweep time.Time
	totalBytes int64
	maxBytes   int64
}

var globalAliasCache = &aliasMapCache{
	items: make(map[string]*aliasCacheEntry),
}

func SetAliasCacheMaxBytes(maxBytes int64) {
	globalAliasCache.mu.Lock()
	defer globalAliasCache.mu.Unlock()
	globalAliasCache.maxBytes = maxBytes
}

func (c *aliasMapCache) get(key string, now time.Time) (*AliasMap, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maybeSweepLocked(now)
	entry, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if now.Sub(entry.lastUsed) > aliasCacheTTL {
		delete(c.items, key)
		return nil, false
	}
	entry.lastUsed = now
	return entry.aliasMap, true
}

func (c *aliasMapCache) set(key string, value *AliasMap, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("alias_cache_store_failed", map[string]any{
				"error": fmt.Sprintf("%v", r),
			})
			logAliasCacheOOMHint()
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maybeSweepLocked(now)

	sizeBytes := estimateAliasMapBytes(value)
	if c.maxBytes > 0 && sizeBytes > c.maxBytes {
		logger.Warn("alias_cache_item_too_large", map[string]any{
			"item_bytes": sizeBytes,
			"max_bytes":  c.maxBytes,
		})
		return
	}

	if c.maxBytes > 0 && c.totalBytes+sizeBytes > c.maxBytes {
		logger.Warn("alias_cache_memory_limit_exceeded", map[string]any{
			"item_bytes":  sizeBytes,
			"total_bytes": c.totalBytes,
			"max_bytes":   c.maxBytes,
		})
		logAliasCacheOOMHint()
		return
	}

	if existing, ok := c.items[key]; ok {
		c.totalBytes -= estimateAliasMapBytes(existing.aliasMap)
	}
	c.items[key] = &aliasCacheEntry{
		aliasMap:  value,
		lastUsed:  now,
		createdAt: now,
	}
	c.totalBytes += sizeBytes
}

func (c *aliasMapCache) maybeSweepLocked(now time.Time) {
	if !c.lastSweep.IsZero() && now.Sub(c.lastSweep) < aliasCacheSweepFreq {
		return
	}
	for key, entry := range c.items {
		if now.Sub(entry.lastUsed) > aliasCacheTTL {
			delete(c.items, key)
			c.totalBytes -= estimateAliasMapBytes(entry.aliasMap)
		}
	}
	c.lastSweep = now
}

func aliasCacheKey(modelName string, preset *DataPreset, filters map[string]any, sorts []string) (string, error) {
	presetName := "custom"
	if preset != nil && strings.TrimSpace(preset.Name) != "" {
		presetName = preset.Name
	} else if preset == nil {
		presetName = "none"
	}

	payload := map[string]any{
		"model":  modelName,
		"preset": presetName,
		"filters": filters,
		"sorts":  sorts,
	}

	data, err := canonicalJSON(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "aliasmap:" + hex.EncodeToString(sum[:]), nil
}

func canonicalJSON(value any) ([]byte, error) {
	var b strings.Builder
	if err := encodeCanonical(&b, value); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func encodeCanonical(b *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		enc, _ := json.Marshal(v)
		b.Write(enc)
	case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		enc, _ := json.Marshal(v)
		b.Write(enc)
	case json.Number:
		b.WriteString(v.String())
	case []string:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			enc, _ := json.Marshal(item)
			b.Write(enc)
		}
		b.WriteByte(']')
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encodeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			encKey, _ := json.Marshal(k)
			b.Write(encKey)
			b.WriteByte(':')
			if err := encodeCanonical(b, v[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		switch typed := value.(type) {
		case []interface{}:
			return encodeCanonical(b, []any(typed))
		case map[string]interface{}:
			return encodeCanonical(b, map[string]any(typed))
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(enc)
	}
	return nil
}

func logAliasCacheOOMHint() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	logger.Error("alias_cache_memory_pressure", map[string]any{
		"alloc_bytes": stats.Alloc,
		"heap_inuse":  stats.HeapInuse,
	})
}

func estimateAliasMapBytes(am *AliasMap) int64 {
	if am == nil {
		return 0
	}
	var size int64
	for k, v := range am.PathToAlias {
		size += int64(len(k) + len(v))
	}
	for k, v := range am.AliasToPath {
		size += int64(len(k) + len(v))
	}
	return size
}
