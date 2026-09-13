package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/config"
	"github.com/engineeringjack/LunarisInjector/pkg/installer"
	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
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

