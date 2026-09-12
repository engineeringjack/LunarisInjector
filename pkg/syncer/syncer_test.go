package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/manifest"
)

func TestSyncerDiffAndSync(t *testing.T) {
	// Create simulated remote server
	remoteModContent := []byte("mod from server v2")
	hasher := sha256.New()
	hasher.Write(remoteModContent)
	remoteModHash := hex.EncodeToString(hasher.Sum(nil))

	remoteManifest := &manifest.Manifest{
		Version:   1,
		Timestamp: 1234567,
		Files: []manifest.FileEntry{
			{
				Path:   "mods/test-mod.jar",
				SHA256: remoteModHash,
				Size:   int64(len(remoteModContent)),
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(remoteManifest)
		case "/mods/test-mod.jar":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(remoteModContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Local environment setup
	tmpGameDir := t.TempDir()
	localModsDir := filepath.Join(tmpGameDir, "mods")
	if err := os.MkdirAll(localModsDir, 0755); err != nil {
		t.Fatalf("failed to create local mods dir: %v", err)
	}

	// Create an obsolete file that should be deleted
	obsoleteFile := filepath.Join(localModsDir, "old-mod.jar")
	if err := os.WriteFile(obsoleteFile, []byte("old obsolete mod"), 0644); err != nil {
		t.Fatalf("failed to write obsolete file: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.ServerURL = ts.URL
	cfg.SyncDirs = []string{"mods"}
	cfg.DeleteExtra = true

	s := New(SyncOptions{
		Config:      cfg,
		GameDir:     tmpGameDir,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
	})

	err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify old-mod was deleted
	if _, err := os.Stat(obsoleteFile); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", obsoleteFile)
	}

	// Verify test-mod was downloaded
	downloadedFile := filepath.Join(localModsDir, "test-mod.jar")
	content, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(content) != string(remoteModContent) {
		t.Errorf("content mismatch: got %q, want %q", string(content), string(remoteModContent))
	}
}

func TestSyncerOfflineFallback(t *testing.T) {
	tmpGameDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.ServerURL = "http://127.0.0.1:59999" // Unreachable port
	cfg.OfflineLaunch = true

	s := New(SyncOptions{
		Config:  cfg,
		GameDir: tmpGameDir,
		Logger:  func(format string, args ...interface{}) {},
	})

	err := s.Sync(context.Background())
	if err != ErrOfflineProceed {
		t.Errorf("expected ErrOfflineProceed, got %v", err)
	}
}
