package modtracker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
)

func TestInitialBaseline(t *testing.T) {
	tempDir := t.TempDir()

	// Initial check before state exists
	if HasBaseline(tempDir) {
		t.Errorf("Expected HasBaseline to be false for new directory")
	}

	state, err := LoadState(tempDir)
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got %v", err)
	}

	// First run: detect changes with nil state
	changes, err := DetectChanges(tempDir, state, nil, []string{"mods"}, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("Expected 0 changes on first run, got %d", len(changes))
	}

	// Create baseline
	files := map[string]manifest.FileEntry{
		"mods/modA.jar": {Path: "mods/modA.jar", SHA256: "hashA", Size: 100},
		"mods/modB.jar": {Path: "mods/modB.jar", SHA256: "hashB", Size: 200},
	}

	newState, err := CreateInitialBaseline(tempDir, files)
	if err != nil {
		t.Fatalf("CreateInitialBaseline failed: %v", err)
	}
	if !HasBaseline(tempDir) {
		t.Errorf("Expected HasBaseline to be true after creation")
	}
	if len(newState.BaselineFiles) != 2 {
		t.Errorf("Expected 2 baseline files, got %d", len(newState.BaselineFiles))
	}
}

func TestDetectAddedAndRemovedAndModified(t *testing.T) {
	tempDir := t.TempDir()
	modsDir := filepath.Join(tempDir, "mods")
	configDir := filepath.Join(tempDir, "config")
	_ = os.MkdirAll(modsDir, 0755)
	_ = os.MkdirAll(configDir, 0755)

	// Server pack files
	modAPath := filepath.Join(modsDir, "modA.jar")
	modBPath := filepath.Join(modsDir, "modB.jar")
	cfgPath := filepath.Join(configDir, "settings.json")

	_ = os.WriteFile(modAPath, []byte("mod A content"), 0644)
	_ = os.WriteFile(modBPath, []byte("mod B content"), 0644)
	_ = os.WriteFile(cfgPath, []byte("config content original"), 0644)

	hashA, _, _ := manifest.ComputeSHA256(modAPath)
	hashB, _, _ := manifest.ComputeSHA256(modBPath)
	hashCfg, _, _ := manifest.ComputeSHA256(cfgPath)

	remoteMap := map[string]manifest.FileEntry{
		"mods/modA.jar":       {Path: "mods/modA.jar", SHA256: hashA, Size: 13},
		"mods/modB.jar":       {Path: "mods/modB.jar", SHA256: hashB, Size: 13},
		"config/settings.json": {Path: "config/settings.json", SHA256: hashCfg, Size: 23},
	}

	// 1. Establish initial baseline
	state, err := CreateInitialBaseline(tempDir, remoteMap)
	if err != nil {
		t.Fatalf("Failed to create baseline: %v", err)
	}

	// 2. User makes changes between launches:
	// A: add custom mod "mods/my-minimap.jar"
	customModPath := filepath.Join(modsDir, "my-minimap.jar")
	_ = os.WriteFile(customModPath, []byte("custom minimap content"), 0644)
	customHash, _, _ := manifest.ComputeSHA256(customModPath)

	// B: remove "mods/modB.jar"
	_ = os.Remove(modBPath)

	// C: modify "config/settings.json"
	_ = os.WriteFile(cfgPath, []byte("config modified by user"), 0644)
	modifiedCfgHash, _, _ := manifest.ComputeSHA256(cfgPath)

	// 3. Detect changes on second run
	changes, err := DetectChanges(tempDir, state, remoteMap, []string{"mods", "config"}, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("Expected 3 detected changes, got %d: %+v", len(changes), changes)
	}

	changeMap := make(map[string]ModChange)
	for _, c := range changes {
		changeMap[c.Path] = c
	}

	// Check added
	addedCh, ok := changeMap["mods/my-minimap.jar"]
	if !ok || addedCh.Type != ChangeAdded || addedCh.CurrentHash != customHash {
		t.Errorf("Expected added custom mod, got: %+v", addedCh)
	}

	// Check removed
	remCh, ok := changeMap["mods/modB.jar"]
	if !ok || remCh.Type != ChangeRemoved || remCh.ServerHash != hashB {
		t.Errorf("Expected removed modB, got: %+v", remCh)
	}

	// Check modified
	modCh, ok := changeMap["config/settings.json"]
	if !ok || modCh.Type != ChangeModified || modCh.CurrentHash != modifiedCfgHash {
		t.Errorf("Expected modified config, got: %+v", modCh)
	}

	// 4. User decides to KEEP custom mod, KEEP modB removed, and KEEP modified config
	decisions := map[string]UserDecision{
		"mods/my-minimap.jar":  DecisionKeep,
		"mods/modB.jar":        DecisionKeep,
		"config/settings.json": DecisionKeep,
	}

	err = ApplyDecisions(tempDir, state, decisions, changes)
	if err != nil {
		t.Fatalf("ApplyDecisions failed: %v", err)
	}

	// Verify decisions were saved in state
	if !state.UserRules.IsKeepAdded("mods/my-minimap.jar") {
		t.Errorf("Expected my-minimap.jar in KeepAdded")
	}
	if !state.UserRules.IsKeepRemoved("mods/modB.jar") {
		t.Errorf("Expected modB.jar in KeepRemoved")
	}
	if !state.UserRules.IsKeepModified("config/settings.json", modifiedCfgHash) {
		t.Errorf("Expected settings.json in KeepModified")
	}

	// 5. Next launch: User did NOT make any new changes!
	// DetectChanges MUST return 0 changes (user should not get prompted again!)
	reloadedState, _ := LoadState(tempDir)
	changesNextLaunch, err := DetectChanges(tempDir, reloadedState, remoteMap, []string{"mods", "config"}, nil)
	if err != nil {
		t.Fatalf("DetectChanges on next launch failed: %v", err)
	}
	if len(changesNextLaunch) != 0 {
		t.Errorf("Expected 0 unapproved changes on next launch, got %d: %+v", len(changesNextLaunch), changesNextLaunch)
	}

	// 6. User introduces ONE brand new change:
	extraModPath := filepath.Join(modsDir, "brand-new.jar")
	_ = os.WriteFile(extraModPath, []byte("brand new mod"), 0644)

	changesWithNew, _ := DetectChanges(tempDir, reloadedState, remoteMap, []string{"mods", "config"}, nil)
	if len(changesWithNew) != 1 {
		t.Fatalf("Expected only 1 new change, got %d: %+v", len(changesWithNew), changesWithNew)
	}
	if changesWithNew[0].Path != "mods/brand-new.jar" {
		t.Errorf("Expected brand-new.jar to be the single detected change, got: %s", changesWithNew[0].Path)
	}

	// 7. Reset to pack
	err = ResetToPack(tempDir)
	if err != nil {
		t.Fatalf("ResetToPack failed: %v", err)
	}
	resetState, _ := LoadState(tempDir)
	if resetState.UserRules.IsKeepAdded("mods/my-minimap.jar") {
		t.Errorf("Expected KeepAdded to be cleared after ResetToPack")
	}
	if resetState.UserRules.IsKeepRemoved("mods/modB.jar") {
		t.Errorf("Expected KeepRemoved to be cleared after ResetToPack")
	}
}
