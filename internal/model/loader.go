package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

func LoadModelsFromDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return err
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var model Model
		if err := yaml.Unmarshal(data, &model); err != nil {
			return fmt.Errorf("unmarshal error in %s: %w", path, err)
		}

		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		Registry[name] = &model
		fmt.Printf("✅ Модель %s загружена с %d связями\n", name, len(model.Relations))
	}
	return nil
}
