package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/slide/LunarisInjector/pkg/config"
)

func TestInstallAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	instanceDir := filepath.Join(tmpDir, "instance1")
	if err := os.MkdirAll(filepath.Join(instanceDir, "mods"), 0755); err != nil {
		t.Fatalf("failed to create instance dir: %v", err)
	}

	// Create dummy launcher_profiles.json
	profilesFile := filepath.Join(instanceDir, "launcher_profiles.json")
	initialProfile := map[string]interface{}{
		"profiles": map[string]interface{}{
			"test-profile": map[string]interface{}{
				"name":    "My Test Profile",
				"gameDir": instanceDir,
			},
		},
	}
	data, _ := json.Marshal(initialProfile)
	if err := os.WriteFile(profilesFile, data, 0644); err != nil {
		t.Fatalf("failed to write dummy profile: %v", err)
	}

	// Run Install
	err := Install(InstallConfig{
		InstanceDir:   instanceDir,
		ServerURL:     "http://myserver:8080",
		RealJavaPath:  "/usr/bin/java",
		LunarisBinary: "/usr/local/bin/lunaris",
		ProfileFile:   profilesFile,
		ProfileID:     "test-profile",
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify lunaris.json was written
	cfgPath := filepath.Join(instanceDir, config.ConfigFileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load generated lunaris.json: %v", err)
	}
	if cfg.ServerURL != "http://myserver:8080" {
		t.Errorf("expected ServerURL http://myserver:8080, got %s", cfg.ServerURL)
	}
	if cfg.RealJavaPath != "/usr/bin/java" {
		t.Errorf("expected RealJavaPath /usr/bin/java, got %s", cfg.RealJavaPath)
	}

	// Verify launcher_profiles.json was patched
	patchedData, _ := os.ReadFile(profilesFile)
	var patchedRoot map[string]interface{}
	_ = json.Unmarshal(patchedData, &patchedRoot)
	profiles := patchedRoot["profiles"].(map[string]interface{})
	p := profiles["test-profile"].(map[string]interface{})
	if p["javaDir"] != "/usr/local/bin/lunaris" {
		t.Errorf("expected javaDir /usr/local/bin/lunaris, got %v", p["javaDir"])
	}

	// Run Uninstall
	err = Uninstall(instanceDir, profilesFile, "test-profile")
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify lunaris.json was deleted
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("expected lunaris.json to be deleted after uninstall")
	}

	// Verify javaDir was removed
	unpatchedData, _ := os.ReadFile(profilesFile)
	var unpatchedRoot map[string]interface{}
	_ = json.Unmarshal(unpatchedData, &unpatchedRoot)
	unpProfiles := unpatchedRoot["profiles"].(map[string]interface{})
	unp := unpProfiles["test-profile"].(map[string]interface{})
	if _, ok := unp["javaDir"]; ok {
		t.Errorf("expected javaDir to be removed after uninstall")
	}
}
