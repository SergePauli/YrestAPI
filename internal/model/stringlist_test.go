package model

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStringListUnmarshal_CommaSeparatedScalar(t *testing.T) {
	var cfg struct {
		Include StringList `yaml:"include"`
	}
	data := []byte("include: personable, simple_item")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Include) != 2 || cfg.Include[0] != "personable" || cfg.Include[1] != "simple_item" {
		t.Fatalf("include parsed wrong: %#v", cfg.Include)
	}
}
