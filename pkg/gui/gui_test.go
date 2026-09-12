package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGUIEndpoints(t *testing.T) {
	srv := New(GUIOptions{
		ServerURL: "https://lunaris.csfrederick.com",
	})
	handler := srv.Handler()

	// 1. Test GET / (HTML Page)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for index, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Lunaris Injector Setup") {
		t.Errorf("expected HTML to contain title")
	}
	if !strings.Contains(body, "JACK FREDERICK") {
		t.Errorf("expected HTML to contain JACK FREDERICK branding")
	}
	if !strings.Contains(body, "--card-bg: #0d0d12") {
		t.Errorf("expected HTML to contain portal color palette")
	}

	// 2. Test GET /api/instances
	reqInst := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rrInst := httptest.NewRecorder()
	handler.ServeHTTP(rrInst, reqInst)

	if rrInst.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/instances, got %d", rrInst.Code)
	}
	var instances []map[string]interface{}
	if err := json.Unmarshal(rrInst.Body.Bytes(), &instances); err != nil {
		t.Fatalf("failed to decode instances JSON: %v", err)
	}

	// 3. Test GET /api/ping
	reqPing := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	rrPing := httptest.NewRecorder()
	handler.ServeHTTP(rrPing, reqPing)

	if rrPing.Code != http.StatusOK {
		t.Fatalf("expected 200 for ping, got %d", rrPing.Code)
	}

	// 4. Test POST /api/install
	tmpGameDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpGameDir, "mods"), 0755)

	installPayload := map[string]interface{}{
		"instance_path": tmpGameDir,
		"server_url":    "https://lunaris.csfrederick.com",
		"enable_vr":     true,
	}
	payloadBytes, _ := json.Marshal(installPayload)

	reqInstall := httptest.NewRequest(http.MethodPost, "/api/install", bytes.NewReader(payloadBytes))
	reqInstall.Header.Set("Content-Type", "application/json")
	rrInstall := httptest.NewRecorder()
	handler.ServeHTTP(rrInstall, reqInstall)

	if rrInstall.Code != http.StatusOK {
		t.Fatalf("expected 200 for install, got %d: %s", rrInstall.Code, rrInstall.Body.String())
	}

	var installResp map[string]interface{}
	if err := json.Unmarshal(rrInstall.Body.Bytes(), &installResp); err != nil {
		t.Fatalf("failed to decode install resp: %v", err)
	}
	if installResp["success"] != true {
		t.Errorf("expected success true, got %v", installResp["success"])
	}
	if installResp["enable_vr"] != true {
		t.Errorf("expected enable_vr true, got %v", installResp["enable_vr"])
	}

	// Verify lunaris.json was created
	cfgFile := filepath.Join(tmpGameDir, "lunaris.json")
	if _, err := os.Stat(cfgFile); err != nil {
		t.Errorf("expected lunaris.json to exist at %s", cfgFile)
	}
}
