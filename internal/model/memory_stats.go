package model

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

func readAllocBytes() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func countPresetsAndComputable() (int, int) {
	presets := 0
	computable := 0
	for _, m := range Registry {
		presets += len(m.Presets)
		computable += len(m.Computable)
	}
	return presets, computable
}

// detectMemoryLimit best-effort detection of memory limit (cgroup or MemTotal). Returns bytes and source label.
func detectMemoryLimit() (uint64, string) {
	// cgroup v2: memory.max
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		if v, ok := parseLimitValue(string(data)); ok {
			return v, "cgroup v2 memory.max"
		}
	}
	// cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if v, ok := parseLimitValue(string(data)); ok {
			return v, "cgroup v1 memory.limit_in_bytes"
		}
	}
	// /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, ln := range lines {
			if strings.HasPrefix(ln, "MemTotal:") {
				fields := strings.Fields(ln)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						return kb * 1024, "proc meminfo MemTotal"
					}
				}
			}
		}
	}
	// fallback to 0 (unknown)
	return 0, "unknown"
}

func parseLimitValue(raw string) (uint64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "max" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func formatBytes(v uint64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case v >= gb:
		return strconv.FormatFloat(float64(v)/float64(gb), 'f', 2, 64) + " GB"
	case v >= mb:
		return strconv.FormatFloat(float64(v)/float64(mb), 'f', 2, 64) + " MB"
	case v >= kb:
		return strconv.FormatFloat(float64(v)/float64(kb), 'f', 2, 64) + " KB"
	default:
		return strconv.FormatUint(v, 10) + " B"
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
