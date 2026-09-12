package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/installer"
	"github.com/slide/LunarisInjector/pkg/manifest"
	"github.com/slide/LunarisInjector/pkg/server"
	"github.com/slide/LunarisInjector/pkg/syncer"
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
