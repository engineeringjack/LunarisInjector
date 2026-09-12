package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "lunaris.json")

	cfg := DefaultConfig()
	cfg.ServerURL = "http://mc.myserver.com:8080"
	cfg.RealJavaPath = "/usr/lib/jvm/java-21/bin/java"
	cfg.SyncDirs = []string{"mods", "config"}
	cfg.IgnoreFiles = []string{"options.txt"}

	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.ServerURL != cfg.ServerURL {
		t.Errorf("expected ServerURL %s, got %s", cfg.ServerURL, loaded.ServerURL)
	}
	if loaded.RealJavaPath != cfg.RealJavaPath {
		t.Errorf("expected RealJavaPath %s, got %s", cfg.RealJavaPath, loaded.RealJavaPath)
	}
	if len(loaded.SyncDirs) != 2 {
		t.Errorf("expected 2 sync dirs, got %d", len(loaded.SyncDirs))
	}
	if len(loaded.IgnoreFiles) != 1 {
		t.Errorf("expected 1 ignore file, got %d", len(loaded.IgnoreFiles))
	}
}

func TestFindInstanceConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ConfigFileName)
	_ = os.WriteFile(cfgPath, []byte(`{"server_url":"http://test:8080"}`), 0644)

	cfg, foundPath, err := FindInstanceConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to find config: %v", err)
	}
	if foundPath != cfgPath {
		t.Errorf("expected foundPath %s, got %s", cfgPath, foundPath)
	}
	if cfg.ServerURL != "http://test:8080" {
		t.Errorf("expected server_url http://test:8080, got %s", cfg.ServerURL)
	}
}
