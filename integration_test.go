package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/config"
	"github.com/engineeringjack/LunarisInjector/pkg/installer"
	"github.com/engineeringjack/LunarisInjector/pkg/logger"
	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
	"github.com/engineeringjack/LunarisInjector/pkg/modtracker"
	"github.com/engineeringjack/LunarisInjector/pkg/server"
	"github.com/engineeringjack/LunarisInjector/pkg/syncer"
)

func TestEndToEndSync(t *testing.T) {
	// 1. Setup Server Directory with mods
	serverDir := t.TempDir()
	serverMods := filepath.Join(serverDir, "mods")
	if err := os.MkdirAll(serverMods, 0755); err != nil {
		t.Fatalf("failed to create server mods dir: %v", err)
	}

	mod1Content := []byte("Lunaris Mod v1.0")
	mod2Content := []byte("Performance Mod v2.5")
	if err := os.WriteFile(filepath.Join(serverMods, "lunaris-core.jar"), mod1Content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverMods, "performance.jar"), mod2Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Start server on an open port
	srv := server.New(server.ServerOptions{
		RootDir:  serverDir,
		SyncDirs: []string{"mods"},
		Port:     18088,
		Host:     "127.0.0.1",
	})

	httpServer := &http.Server{
		Addr:    "127.0.0.1:18088",
		Handler: srv.Handler(),
	}

	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer httpServer.Close()

	// Wait briefly for server to bind
	time.Sleep(50 * time.Millisecond)

	// 2. Setup Client Directory with outdated & obsolete files
	clientDir := t.TempDir()
	clientMods := filepath.Join(clientDir, "mods")
	if err := os.MkdirAll(clientMods, 0755); err != nil {
		t.Fatalf("failed to create client mods dir: %v", err)
	}

	// Obsolete file: should be deleted
	obsoletePath := filepath.Join(clientMods, "deleted-mod.jar")
	_ = os.WriteFile(obsoletePath, []byte("obsolete"), 0644)

	// Outdated file: should be replaced
	outdatedPath := filepath.Join(clientMods, "lunaris-core.jar")
	_ = os.WriteFile(outdatedPath, []byte("old version v0.1"), 0644)

	// 3. Test Installer
	err := installer.Install(installer.InstallConfig{
		InstanceDir: clientDir,
		ServerURL:   "http://127.0.0.1:18088",
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	cfg, _, err := config.FindInstanceConfig(clientDir)
	if err != nil {
		t.Fatalf("FindInstanceConfig failed: %v", err)
	}

	// 4. Run Syncer
	s := syncer.New(syncer.SyncOptions{
		Config:      cfg,
		GameDir:     clientDir,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
	})

	err = s.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// 5. Assertions
	// Check obsolete file is deleted
	if _, err := os.Stat(obsoletePath); !os.IsNotExist(err) {
		t.Errorf("expected obsolete file %s to be deleted", obsoletePath)
	}

	// Check lunaris-core was updated
	coreData, err := os.ReadFile(outdatedPath)
	if err != nil {
		t.Fatalf("failed to read lunaris-core: %v", err)
	}
	if string(coreData) != string(mod1Content) {
		t.Errorf("lunaris-core content mismatch: got %q, want %q", string(coreData), string(mod1Content))
	}

	// Check performance.jar was added
	perfPath := filepath.Join(clientMods, "performance.jar")
	perfData, err := os.ReadFile(perfPath)
	if err != nil {
		t.Fatalf("failed to read performance.jar: %v", err)
	}
	if string(perfData) != string(mod2Content) {
		t.Errorf("performance.jar content mismatch: got %q, want %q", string(perfData), string(mod2Content))
	}

	// Check SHA256 of downloaded files
	hash, _, err := manifest.ComputeSHA256(perfPath)
	if err != nil {
		t.Fatal(err)
	}
	expectedHash, _, _ := manifest.ComputeSHA256(filepath.Join(serverMods, "performance.jar"))
	if hash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, hash)
	}
}

func TestMultiDirSyncWithConfigsAndPacks(t *testing.T) {
	// 1. Setup Server Directory with mods, config, global_packs
	serverDir := t.TempDir()
	serverMods := filepath.Join(serverDir, "mods")
	serverConfig := filepath.Join(serverDir, "config")
	serverPacks := filepath.Join(serverDir, "global_packs", "required_resources")
	_ = os.MkdirAll(serverMods, 0755)
	_ = os.MkdirAll(serverConfig, 0755)
	_ = os.MkdirAll(serverPacks, 0755)

	_ = os.WriteFile(filepath.Join(serverMods, "test-mod.jar"), []byte("mod content"), 0644)
	_ = os.WriteFile(filepath.Join(serverConfig, "gameplay.toml"), []byte("difficulty = hard\n"), 0644)
	_ = os.WriteFile(filepath.Join(serverPacks, "custom-pack.zip"), []byte("pack zip binary data"), 0644)

	srv := server.New(server.ServerOptions{
		RootDir:  serverDir,
		SyncDirs: []string{"mods", "config", "global_packs"},
		Port:     18089,
		Host:     "127.0.0.1",
	})

	httpServer := &http.Server{
		Addr:    "127.0.0.1:18089",
		Handler: srv.Handler(),
	}

	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer httpServer.Close()

	time.Sleep(50 * time.Millisecond)

	// 2. Setup Client Directory
	clientDir := t.TempDir()
	clientMods := filepath.Join(clientDir, "mods")
	clientConfig := filepath.Join(clientDir, "config")
	clientVoice := filepath.Join(clientConfig, "voicechat")
	_ = os.MkdirAll(clientMods, 0755)
	_ = os.MkdirAll(clientConfig, 0755)
	_ = os.MkdirAll(clientVoice, 0755)

	// Existing outdated config
	outdatedConfig := filepath.Join(clientConfig, "gameplay.toml")
	_ = os.WriteFile(outdatedConfig, []byte("difficulty = easy\n"), 0644)

	// Excluded client-only personal settings that MUST NOT be deleted
	playerVols := filepath.Join(clientVoice, "player-volumes.properties")
	_ = os.WriteFile(playerVols, []byte("player1=1.5\n"), 0644)
	embedFingerprint := filepath.Join(clientConfig, "embeddium-fingerprint.json")
	_ = os.WriteFile(embedFingerprint, []byte("{\"fingerprint\": \"abc\"}"), 0644)

	// 3. Install
	err := installer.Install(installer.InstallConfig{
		InstanceDir: clientDir,
		ServerURL:   "http://127.0.0.1:18089",
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	cfg, _, err := config.FindInstanceConfig(clientDir)
	if err != nil {
		t.Fatalf("FindInstanceConfig failed: %v", err)
	}

	// 4. Sync
	s := syncer.New(syncer.SyncOptions{
		Config:      cfg,
		GameDir:     clientDir,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
	})

	err = s.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// 5. Verify
	// Check mod downloaded
	if _, err := os.Stat(filepath.Join(clientMods, "test-mod.jar")); err != nil {
		t.Errorf("expected mod test-mod.jar to exist: %v", err)
	}

	// Check config updated
	cfgData, err := os.ReadFile(outdatedConfig)
	if err != nil {
		t.Fatalf("failed to read gameplay.toml: %v", err)
	}
	if string(cfgData) != "difficulty = hard\n" {
		t.Errorf("gameplay.toml not updated: %s", string(cfgData))
	}

	// Check global pack downloaded
	packPath := filepath.Join(clientDir, "global_packs", "required_resources", "custom-pack.zip")
	if _, err := os.Stat(packPath); err != nil {
		t.Errorf("expected custom-pack.zip to exist: %v", err)
	}

	// Check personal/excluded files are PRESERVED
	if _, err := os.Stat(playerVols); err != nil {
		t.Errorf("expected player-volumes.properties to be preserved: %v", err)
	}
	if _, err := os.Stat(embedFingerprint); err != nil {
		t.Errorf("expected embeddium-fingerprint.json to be preserved: %v", err)
	}
}

func TestLoggingEndToEndAndOverwrite(t *testing.T) {
	// 1. Setup server
	serverDir := t.TempDir()
	serverMods := filepath.Join(serverDir, "mods")
	_ = os.MkdirAll(serverMods, 0755)
	_ = os.WriteFile(filepath.Join(serverMods, "mod-alpha.jar"), []byte("alpha content"), 0644)

	srv := server.New(server.ServerOptions{
		RootDir:  serverDir,
		SyncDirs: []string{"mods"},
		Port:     18090,
		Host:     "127.0.0.1",
	})
	httpServer := &http.Server{
		Addr:    "127.0.0.1:18090",
		Handler: srv.Handler(),
	}
	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer httpServer.Close()
	time.Sleep(50 * time.Millisecond)

	// 2. Setup client instance
	clientDir := t.TempDir()
	logFilePrimary := filepath.Join(clientDir, logger.DefaultLogFileName)
	logFileSecondary := filepath.Join(clientDir, logger.LogsDirName, logger.DefaultLogFileName)

	// RUN 1: Installation and Initial Sync
	logFiles, err := logger.Init(clientDir, logger.Options{
		QuietConsole: true,
		Version:      "1.0.1",
		Args:         []string{"install", "--instance", clientDir},
	})
	if err != nil {
		t.Fatalf("logger.Init failed: %v", err)
	}
	if len(logFiles) != 2 {
		t.Errorf("expected 2 log files (root and logs/), got: %v", logFiles)
	}

	err = installer.Install(installer.InstallConfig{
		InstanceDir: clientDir,
		ServerURL:   "http://127.0.0.1:18090",
		Logger:      logger.AsFunc(),
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	cfg, _, err := config.FindInstanceConfig(clientDir)
	if err != nil {
		t.Fatalf("FindInstanceConfig failed: %v", err)
	}

	s := syncer.New(syncer.SyncOptions{
		Config:      cfg,
		GameDir:     clientDir,
		WorkerCount: 2,
		Logger:      logger.AsFunc(),
	})

	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	_ = logger.Close()

	// Verify Run 1 log contents
	data1, err := os.ReadFile(logFilePrimary)
	if err != nil {
		t.Fatalf("failed to read primary log: %v", err)
	}
	logStr1 := string(data1)
	if !strings.Contains(logStr1, "[Installer] Starting installation for instance:") {
		t.Errorf("Log missing installer start: %s", logStr1)
	}
	if !strings.Contains(logStr1, "[Installer] Saved configuration") {
		t.Errorf("Log missing config save: %s", logStr1)
	}
	if !strings.Contains(logStr1, "[Lunaris] Checking sync server:") {
		t.Errorf("Log missing sync server check: %s", logStr1)
	}
	if !strings.Contains(logStr1, "[Lunaris] Verified and installed: mods/mod-alpha.jar") {
		t.Errorf("Log missing download verification: %s", logStr1)
	}

	// Verify secondary log file in logs/
	data1Sec, err := os.ReadFile(logFileSecondary)
	if err != nil {
		t.Fatalf("failed to read secondary log: %v", err)
	}
	if !strings.Contains(string(data1Sec), "[Lunaris] Verified and installed: mods/mod-alpha.jar") {
		t.Errorf("Secondary log missing action: %s", string(data1Sec))
	}

	// RUN 2: Next run MUST overwrite or remove the last run with the current one!
	_, err = logger.Init(clientDir, logger.Options{
		QuietConsole: true,
		Version:      "1.0.1",
		Args:         []string{"--gameDir", clientDir},
	})
	if err != nil {
		t.Fatalf("logger.Init (Run 2) failed: %v", err)
	}

	logger.Infof("[Lunaris] Run 2 pre-launch sync starting")
	// Second sync (already up-to-date)
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync run 2 failed: %v", err)
	}
	logger.Infof("[Lunaris] Run 2 completed successfully")
	_ = logger.Close()

	// Verify Run 2 log contents
	data2, err := os.ReadFile(logFilePrimary)
	if err != nil {
		t.Fatalf("failed to read primary log after Run 2: %v", err)
	}
	logStr2 := string(data2)

	// Must contain Run 2 actions
	if !strings.Contains(logStr2, "[Lunaris] Run 2 pre-launch sync starting") {
		t.Errorf("Run 2 log missing Run 2 start: %s", logStr2)
	}
	if !strings.Contains(logStr2, "[Lunaris] Everything is up to date!") {
		t.Errorf("Run 2 log missing up-to-date message: %s", logStr2)
	}
	if !strings.Contains(logStr2, "[Lunaris] Run 2 completed successfully") {
		t.Errorf("Run 2 log missing Run 2 completion: %s", logStr2)
	}

	// Must NOT contain Run 1 actions (they were overwritten/removed)
	if strings.Contains(logStr2, "[Installer] Starting installation for instance:") {
		t.Errorf("Log was not overwritten! Still contains Run 1 install action: %s", logStr2)
	}
	if strings.Contains(logStr2, "mods/mod-alpha.jar (1/1)") {
		t.Errorf("Log was not overwritten! Still contains Run 1 download action: %s", logStr2)
	}
}

func TestModTrackerAndResyncEndToEnd(t *testing.T) {
	// 1. Setup Server Directory with official server pack
	serverDir := t.TempDir()
	serverMods := filepath.Join(serverDir, "mods")
	serverConfig := filepath.Join(serverDir, "config")
	_ = os.MkdirAll(serverMods, 0755)
	_ = os.MkdirAll(serverConfig, 0755)

	serverCoreContent := []byte("official-server-core-mod-v1.0")
	troubleModContent := []byte("troublesome-mod-content")
	origConfigContent := []byte(`{"fov": 70, "difficulty": "hard"}`)

	_ = os.WriteFile(filepath.Join(serverMods, "server-core.jar"), serverCoreContent, 0644)
	_ = os.WriteFile(filepath.Join(serverMods, "troublesome-mod.jar"), troubleModContent, 0644)
	_ = os.WriteFile(filepath.Join(serverConfig, "server-options.json"), origConfigContent, 0644)

	srv := server.New(server.ServerOptions{
		RootDir:  serverDir,
		SyncDirs: []string{"mods", "config"},
		Port:     18091,
		Host:     "127.0.0.1",
	})

	httpServer := &http.Server{
		Addr:    "127.0.0.1:18091",
		Handler: srv.Handler(),
	}
	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer httpServer.Close()

	time.Sleep(50 * time.Millisecond)

	// 2. Setup Client Directory
	clientDir := t.TempDir()
	err := installer.Install(installer.InstallConfig{
		InstanceDir: clientDir,
		ServerURL:   "http://127.0.0.1:18091",
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	cfg, _, err := config.FindInstanceConfig(clientDir)
	if err != nil {
		t.Fatalf("FindInstanceConfig failed: %v", err)
	}

	// 3. FIRST RUN: Establish initial baseline without prompting
	// Baseline state file should NOT exist yet
	if modtracker.HasBaseline(clientDir) {
		t.Fatalf("Baseline file should not exist before first run")
	}

	s := syncer.New(syncer.SyncOptions{
		Config:      cfg,
		GameDir:     clientDir,
		WorkerCount: 2,
		Logger:      func(string, ...interface{}) {},
	})

	ctx := context.Background()
	remoteM, err := s.FetchRemoteManifest(ctx)
	if err != nil {
		t.Fatalf("FetchRemoteManifest failed: %v", err)
	}

	if err := s.SyncWithManifest(ctx, remoteM); err != nil {
		t.Fatalf("Initial sync failed: %v", err)
	}

	remoteMap := remoteM.FilteredMap(nil)
	_, err = modtracker.CreateInitialBaseline(clientDir, remoteMap)
	if err != nil {
		t.Fatalf("CreateInitialBaseline failed: %v", err)
	}

	if !modtracker.HasBaseline(clientDir) {
		t.Fatalf("Baseline state file was not created")
	}

	// 4. USER MODIFICATIONS between launches:
	// A: User installs custom client-side mod
	customModPath := filepath.Join(clientDir, "mods", "client-minimap.jar")
	customModContent := []byte("client-side-minimap-mod")
	if err := os.WriteFile(customModPath, customModContent, 0644); err != nil {
		t.Fatal(err)
	}

	// B: User removes troublesome server mod for themselves
	troubleModPath := filepath.Join(clientDir, "mods", "troublesome-mod.jar")
	if err := os.Remove(troubleModPath); err != nil {
		t.Fatal(err)
	}

	// C: User modifies config file
	clientCfgPath := filepath.Join(clientDir, "config", "server-options.json")
	userModifiedConfig := []byte(`{"fov": 95, "difficulty": "hard"}`)
	if err := os.WriteFile(clientCfgPath, userModifiedConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// 5. SECOND RUN: Detect changes and prompt user
	loadedState, err := modtracker.LoadState(clientDir)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	changes, err := modtracker.DetectChanges(clientDir, loadedState, remoteMap, cfg.SyncDirs, cfg.IgnoreFiles)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("Expected 3 detected changes, got %d: %+v", len(changes), changes)
	}

	// User decides to KEEP all modifications on top of pack
	decisions := map[string]modtracker.UserDecision{
		"mods/client-minimap.jar":    modtracker.DecisionKeep,
		"mods/troublesome-mod.jar":   modtracker.DecisionKeep,
		"config/server-options.json": modtracker.DecisionKeep,
	}

	if err := modtracker.ApplyDecisions(clientDir, loadedState, decisions, changes); err != nil {
		t.Fatalf("ApplyDecisions failed: %v", err)
	}

	// Apply decisions in syncer and execute sync
	s.SetUserRules(&loadedState.UserRules)
	if err := s.SyncWithManifest(ctx, remoteM); err != nil {
		t.Fatalf("Second sync failed: %v", err)
	}

	// Assertions after second sync:
	// - Custom minimap must still exist (not deleted as extra)
	if _, err := os.Stat(customModPath); os.IsNotExist(err) {
		t.Errorf("Expected custom mod client-minimap.jar to be kept, but was deleted")
	}
	// - Troublesome mod must NOT be re-downloaded
	if _, err := os.Stat(troubleModPath); !os.IsNotExist(err) {
		t.Errorf("Expected troublesome-mod.jar to remain removed, but was re-downloaded")
	}
	// - Modified config must preserve user's changes
	cfgData, _ := os.ReadFile(clientCfgPath)
	if string(cfgData) != string(userModifiedConfig) {
		t.Errorf("Expected user config edits to be kept, got: %s", string(cfgData))
	}

	// 6. THIRD RUN: No new changes made! User must NOT be prompted again
	reloadedState, _ := modtracker.LoadState(clientDir)
	changesRun3, err := modtracker.DetectChanges(clientDir, reloadedState, remoteMap, cfg.SyncDirs, cfg.IgnoreFiles)
	if err != nil {
		t.Fatalf("DetectChanges Run 3 failed: %v", err)
	}
	if len(changesRun3) != 0 {
		t.Fatalf("Expected 0 unapproved changes on Run 3 (no prompt!), got %d: %+v", len(changesRun3), changesRun3)
	}

	// 7. USER CHOOSES RESYNC (Forget all modifications and match server pack 100%)
	if err := syncer.Resync(context.Background(), syncer.ResyncOptions{
		InstanceDir: clientDir,
		WorkerCount: 2,
	}); err != nil {
		t.Fatalf("syncer.Resync failed: %v", err)
	}

	// Assertions after Resync:
	// - Custom minimap MUST be deleted
	if _, err := os.Stat(customModPath); !os.IsNotExist(err) {
		t.Errorf("Expected custom mod client-minimap.jar to be deleted after resync")
	}
	// - Troublesome mod MUST be restored from server
	if _, err := os.Stat(troubleModPath); os.IsNotExist(err) {
		t.Errorf("Expected troublesome-mod.jar to be restored after resync")
	}
	// - Config MUST be restored to original server pack content
	resyncedCfgData, _ := os.ReadFile(clientCfgPath)
	if string(resyncedCfgData) != string(origConfigContent) {
		t.Errorf("Expected config to be restored to server content, got: %s", string(resyncedCfgData))
	}

	// - User rules in state must be empty
	finalState, _ := modtracker.LoadState(clientDir)
	if len(finalState.UserRules.KeepAdded) != 0 || len(finalState.UserRules.KeepRemoved) != 0 || len(finalState.UserRules.KeepModified) != 0 {
		t.Errorf("Expected all user rules to be wiped after resync, got: %+v", finalState.UserRules)
	}
}


