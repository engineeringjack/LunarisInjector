package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slide/LunarisInjector/pkg/manifest"
)

func TestServerManifestAndFileServing(t *testing.T) {
	tmpDir := t.TempDir()
	modsDir := filepath.Join(tmpDir, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}

	testModContent := []byte("mod jar binary content")
	testModFile := filepath.Join(modsDir, "server-test.jar")
	if err := os.WriteFile(testModFile, testModContent, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	srv := New(ServerOptions{
		RootDir:     tmpDir,
		SyncDirs:    []string{"mods"},
		Port:        8080,
		AutoRefresh: 1 * time.Second,
	})

	handler := srv.Handler()

	// Test 1: GET /manifest.json
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var m manifest.Manifest
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}

	if len(m.Files) != 1 {
		t.Fatalf("expected 1 file in manifest, got %d", len(m.Files))
	}
	if m.Files[0].Path != "mods/server-test.jar" {
		t.Errorf("expected path mods/server-test.jar, got %s", m.Files[0].Path)
	}

	// Test 2: GET /mods/server-test.jar
	req2 := httptest.NewRequest(http.MethodGet, "/mods/server-test.jar", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected status 200 for file download, got %d", rr2.Code)
	}
	if rr2.Body.String() != string(testModContent) {
		t.Errorf("expected file body %q, got %q", string(testModContent), rr2.Body.String())
	}

	// Test 3: GET / (Dashboard HTML)
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusOK {
		t.Fatalf("expected status 200 for index, got %d", rr3.Code)
	}
}
