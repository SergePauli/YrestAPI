package config

import (
	"testing"
	"time"
)

func TestLoadLocalDatetime(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("test-local", 8*60*60)
	t.Cleanup(func() { time.Local = old })
	for _, tc := range []struct {
		value   string
		enabled bool
	}{
		{"", false}, {"false", false}, {"true", true}, {"invalid", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("LOCAL_DATETIME", tc.value)
			cfg := LoadConfig()
			if cfg.LocalDatetime != tc.enabled {
				t.Fatalf("enabled=%v", cfg.LocalDatetime)
			}
			if tc.enabled && cfg.DatetimeLocation != time.Local {
				t.Fatal("local timezone not captured")
			}
			if !tc.enabled && cfg.DatetimeLocation != nil {
				t.Fatal("disabled conversion must not set a zone")
			}
		})
	}
}
