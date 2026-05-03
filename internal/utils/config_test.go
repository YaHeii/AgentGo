package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsEachDirectoryIndependently(t *testing.T) {
	t.Parallel()

	firstDir := t.TempDir()
	secondDir := t.TempDir()

	writeConfigFile(t, firstDir, "BASE_URL=https://first.example\nAPI_KEY=first-key\nMODEL=first-model\n")
	writeConfigFile(t, secondDir, "BASE_URL=https://second.example\nAPI_KEY=second-key\nMODEL=second-model\n")

	firstCfg, err := LoadConfig(firstDir)
	if err != nil {
		t.Fatalf("load first config: %v", err)
	}
	if firstCfg.Model != "first-model" {
		t.Fatalf("expected first model, got %q", firstCfg.Model)
	}

	secondCfg, err := LoadConfig(secondDir)
	if err != nil {
		t.Fatalf("load second config: %v", err)
	}
	if secondCfg.BaseURL != "https://second.example" {
		t.Fatalf("expected second base url, got %q", secondCfg.BaseURL)
	}
	if secondCfg.APIKey != "second-key" {
		t.Fatalf("expected second api key, got %q", secondCfg.APIKey)
	}
	if secondCfg.Model != "second-model" {
		t.Fatalf("expected second model, got %q", secondCfg.Model)
	}
}

func writeConfigFile(t *testing.T, dir string, contents string) {
	t.Helper()

	path := filepath.Join(dir, "app.env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}
