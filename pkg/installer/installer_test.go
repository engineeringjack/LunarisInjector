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

func TestInstallVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	instanceDir := filepath.Join(tmpDir, "instance-1.26.2")
	_ = os.MkdirAll(instanceDir, 0755)

	// Create minecraftinstance.json with version 1.26.2
	mcJSON := filepath.Join(instanceDir, "minecraftinstance.json")
	_ = os.WriteFile(mcJSON, []byte(`{"name":"Wrong Version Modpack","gameVersion":"1.26.2"}`), 0644)

	err := Install(InstallConfig{
		InstanceDir:         instanceDir,
		ServerURL:           "http://myserver:8080",
		RequiredGameVersion: "1.20.1",
	})

	if err == nil {
		t.Fatalf("expected error when installing on mismatched version 1.26.2, got nil")
	}

	// Now test with matching 1.20.1
	matchingDir := filepath.Join(tmpDir, "instance-1.20.1")
	_ = os.MkdirAll(matchingDir, 0755)
	_ = os.WriteFile(filepath.Join(matchingDir, "minecraftinstance.json"), []byte(`{"name":"Correct Modpack","gameVersion":"1.20.1"}`), 0644)

	err = Install(InstallConfig{
		InstanceDir:         matchingDir,
		ServerURL:           "http://myserver:8080",
		RequiredGameVersion: "1.20.1",
		RealJavaPath:        "/usr/bin/java",
	})

	if err != nil {
		t.Fatalf("expected success for 1.20.1, got: %v", err)
	}
}

func TestHookAndUnhookCurseForgeJava(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "Install", "java", "java-runtime-gamma", "bin")
	_ = os.MkdirAll(binDir, 0755)

	targetName := "java"
	realName := "java.real"
	if os.PathSeparator == '\\' {
		targetName = "javaw.exe"
		realName = "javaw.real.exe"
	}

	origJavaContent := "original-openjdk-binary"
	targetPath := filepath.Join(binDir, targetName)
	_ = os.WriteFile(targetPath, []byte(origJavaContent), 0755)

	lunarisContent := "lunaris-injector-binary"
	lunarisBin := filepath.Join(tmpDir, "lunaris")
	_ = os.WriteFile(lunarisBin, []byte(lunarisContent), 0755)

	inst1 := filepath.Join(tmpDir, "Instances", "Lunaris V.2")

	// 1. Hook
	hooked, err := HookCurseForgeJava(binDir, lunarisBin, inst1)
	if err != nil {
		t.Fatalf("HookCurseForgeJava failed: %v", err)
	}
	if !hooked {
		t.Fatalf("expected hooked to be true")
	}

	if !IsCurseForgeJavaHooked(binDir) {
		t.Errorf("expected IsCurseForgeJavaHooked to be true")
	}

	// Verify java.real has original content
	realPath := filepath.Join(binDir, realName)
	realData, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", realPath, err)
	}
	if string(realData) != origJavaContent {
		t.Errorf("expected real content %q, got %q", origJavaContent, string(realData))
	}

	// Verify target binary has lunaris content
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", targetPath, err)
	}
	if string(targetData) != lunarisContent {
		t.Errorf("expected target content %q, got %q", lunarisContent, string(targetData))
	}

	// 2. Add second instance
	inst2 := filepath.Join(tmpDir, "Instances", "Second Instance")
	_ = os.MkdirAll(inst2, 0755)
	_ = os.WriteFile(filepath.Join(inst2, config.ConfigFileName), []byte("{}"), 0644)
	_, err = HookCurseForgeJava(binDir, lunarisBin, inst2)
	if err != nil {
		t.Fatalf("second hook failed: %v", err)
	}

	// Unhook first instance -> should preserve hook because inst2 remains
	unhooked, err := UnhookCurseForgeJava(binDir, inst1)
	if err != nil {
		t.Fatalf("UnhookCurseForgeJava inst1 failed: %v", err)
	}
	if unhooked {
		t.Errorf("expected unhooked=false because inst2 remains")
	}
	if !IsCurseForgeJavaHooked(binDir) {
		t.Errorf("expected still hooked after inst1 unhook")
	}

	// Unhook second instance -> now should fully restore
	unhooked, err = UnhookCurseForgeJava(binDir, inst2)
	if err != nil {
		t.Fatalf("UnhookCurseForgeJava inst2 failed: %v", err)
	}
	if !unhooked {
		t.Errorf("expected unhooked=true when all instances removed")
	}
	if IsCurseForgeJavaHooked(binDir) {
		t.Errorf("expected not hooked after inst2 unhook")
	}

	// Verify restored target has original content
	restoredData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read restored %s: %v", targetPath, err)
	}
	if string(restoredData) != origJavaContent {
		t.Errorf("expected restored content %q, got %q", origJavaContent, string(restoredData))
	}

	// Verify .real is deleted
	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted after unhook", realPath)
	}
}

