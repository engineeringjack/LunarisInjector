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

	"github.com/engineeringjack/LunarisInjector/pkg/config"
	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
	"github.com/engineeringjack/LunarisInjector/pkg/modtracker"
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

func TestSyncerOptionalVR(t *testing.T) {
	coreContent := []byte("core mod 1.20.1")
	h1 := sha256.Sum256(coreContent)
	coreHash := hex.EncodeToString(h1[:])

	vrContent := []byte("vivecraft 1.20.1 forge")
	h2 := sha256.Sum256(vrContent)
	vrHash := hex.EncodeToString(h2[:])

	remoteManifest := &manifest.Manifest{
		Version:   1,
		Timestamp: 1234567,
		Files: []manifest.FileEntry{
			{
				Path:   "mods/core.jar",
				SHA256: coreHash,
				Size:   int64(len(coreContent)),
			},
			{
				Path:     "optional/vr/mods/vivecraft.jar",
				SHA256:   vrHash,
				Size:     int64(len(vrContent)),
				Feature:  "vr",
				DestPath: "mods/vivecraft.jar",
			},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(remoteManifest)
		case "/mods/core.jar":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(coreContent)
		case "/optional/vr/mods/vivecraft.jar":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(vrContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Scenario 1: Non-VR player has vivecraft.jar present locally.
	// With EnableVR = false and DeleteExtra = true, vivecraft.jar should be deleted and core.jar downloaded.
	tmpGameDirNoVR := t.TempDir()
	localModsNoVR := filepath.Join(tmpGameDirNoVR, "mods")
	_ = os.MkdirAll(localModsNoVR, 0755)
	localVRJar := filepath.Join(localModsNoVR, "vivecraft.jar")
	_ = os.WriteFile(localVRJar, []byte("old vivecraft"), 0644)

	cfgNoVR := config.DefaultConfig()
	cfgNoVR.ServerURL = ts.URL
	cfgNoVR.SyncDirs = []string{"mods"}
	cfgNoVR.DeleteExtra = true
	cfgNoVR.EnableVR = false

	sNoVR := New(SyncOptions{
		Config:      cfgNoVR,
		GameDir:     tmpGameDirNoVR,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
	})

	if err := sNoVR.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (NoVR) failed: %v", err)
	}

	// Verify vivecraft.jar was removed for non-VR user
	if _, err := os.Stat(localVRJar); !os.IsNotExist(err) {
		t.Errorf("expected vivecraft.jar to be removed for non-VR client")
	}
	// Verify core.jar was installed
	if _, err := os.Stat(filepath.Join(localModsNoVR, "core.jar")); err != nil {
		t.Errorf("expected core.jar to be downloaded for non-VR client")
	}

	// Scenario 2: VR player has EnableVR = true.
	// Both core.jar and vivecraft.jar should be downloaded into mods/.
	tmpGameDirVR := t.TempDir()
	localModsVR := filepath.Join(tmpGameDirVR, "mods")
	_ = os.MkdirAll(localModsVR, 0755)

	cfgVR := config.DefaultConfig()
	cfgVR.ServerURL = ts.URL
	cfgVR.SyncDirs = []string{"mods"}
	cfgVR.DeleteExtra = true
	cfgVR.EnableVR = true

	sVR := New(SyncOptions{
		Config:      cfgVR,
		GameDir:     tmpGameDirVR,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
	})

	if err := sVR.Sync(context.Background()); err != nil {
		t.Fatalf("Sync (VR) failed: %v", err)
	}

	// Verify both mods exist
	if _, err := os.Stat(filepath.Join(localModsVR, "core.jar")); err != nil {
		t.Errorf("expected core.jar to be downloaded for VR client")
	}
	downloadedVR, err := os.ReadFile(filepath.Join(localModsVR, "vivecraft.jar"))
	if err != nil {
		t.Fatalf("expected vivecraft.jar to be downloaded for VR client: %v", err)
	}
	if string(downloadedVR) != string(vrContent) {
		t.Errorf("vivecraft content mismatch: got %q, want %q", string(downloadedVR), string(vrContent))
	}
}

func TestSyncerUserRules(t *testing.T) {
	// Remote server has:
	// - mods/server-mod.jar
	// - mods/troublesome.jar
	// - config/pack-config.json
	remoteServerMod := []byte("server mod")
	remoteTroublesome := []byte("troublesome mod")
	remoteConfig := []byte("pack config original")

	hMod := sha256.Sum256(remoteServerMod)
	hTrouble := sha256.Sum256(remoteTroublesome)
	hCfg := sha256.Sum256(remoteConfig)

	m := manifest.Manifest{
		Version: 1,
		Files: []manifest.FileEntry{
			{Path: "mods/server-mod.jar", SHA256: hex.EncodeToString(hMod[:]), Size: int64(len(remoteServerMod))},
			{Path: "mods/troublesome.jar", SHA256: hex.EncodeToString(hTrouble[:]), Size: int64(len(remoteTroublesome))},
			{Path: "config/pack-config.json", SHA256: hex.EncodeToString(hCfg[:]), Size: int64(len(remoteConfig))},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(m)
		case "/mods/server-mod.jar":
			_, _ = w.Write(remoteServerMod)
		case "/mods/troublesome.jar":
			_, _ = w.Write(remoteTroublesome)
		case "/config/pack-config.json":
			_, _ = w.Write(remoteConfig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Client has:
	// - mods/custom-client.jar (User added mod!)
	// - config/pack-config.json (User edited locally!)
	// And client intentionally DOES NOT have mods/troublesome.jar (User removed it!)
	tmpGameDir := t.TempDir()
	modsDir := filepath.Join(tmpGameDir, "mods")
	cfgDir := filepath.Join(tmpGameDir, "config")
	_ = os.MkdirAll(modsDir, 0755)
	_ = os.MkdirAll(cfgDir, 0755)

	customClientPath := filepath.Join(modsDir, "custom-client.jar")
	_ = os.WriteFile(customClientPath, []byte("my client mod"), 0644)

	localConfigPath := filepath.Join(cfgDir, "pack-config.json")
	localConfigContent := []byte("pack config edited by user")
	_ = os.WriteFile(localConfigPath, localConfigContent, 0644)
	hLocalCfg := sha256.Sum256(localConfigContent)

	cfg := config.DefaultConfig()
	cfg.ServerURL = ts.URL
	cfg.SyncDirs = []string{"mods", "config"}
	cfg.DeleteExtra = true

	userRules := &modtracker.UserRules{
		KeepAdded:    map[string]bool{"mods/custom-client.jar": true},
		KeepRemoved:  map[string]bool{"mods/troublesome.jar": true},
		KeepModified: map[string]string{"config/pack-config.json": hex.EncodeToString(hLocalCfg[:])},
	}

	s := New(SyncOptions{
		Config:      cfg,
		GameDir:     tmpGameDir,
		WorkerCount: 2,
		Logger:      func(format string, args ...interface{}) {},
		UserRules:   userRules,
	})

	// Run Sync
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// 1. Verify custom client mod was NOT deleted
	if _, err := os.Stat(customClientPath); err != nil {
		t.Errorf("expected custom-client.jar to be preserved, but got error: %v", err)
	}

	// 2. Verify troublesome mod was NOT downloaded
	if _, err := os.Stat(filepath.Join(modsDir, "troublesome.jar")); !os.IsNotExist(err) {
		t.Errorf("expected troublesome.jar to remain removed, but it was downloaded!")
	}

	// 3. Verify server-mod.jar was downloaded normally
	if _, err := os.Stat(filepath.Join(modsDir, "server-mod.jar")); err != nil {
		t.Errorf("expected server-mod.jar to be downloaded: %v", err)
	}

	// 4. Verify pack-config.json was NOT overwritten by server
	dataCfg, err := os.ReadFile(localConfigPath)
	if err != nil {
		t.Fatalf("failed to read local config: %v", err)
	}
	if string(dataCfg) != string(localConfigContent) {
		t.Errorf("pack-config.json was overwritten by server! got %q, want %q", string(dataCfg), string(localConfigContent))
	}
}


