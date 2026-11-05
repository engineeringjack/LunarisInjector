package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminAuthEncryptedCredentials(t *testing.T) {
	tmpDir := t.TempDir()

	authMgr, err := NewAuthManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to create AuthManager: %v", err)
	}

	if authMgr.IsConfigured() {
		t.Fatalf("expected IsConfigured to be false initially")
	}

	// 1. Set credentials
	err = authMgr.SetCredentials("ServerAdmin", "SuperSecretPass123!")
	if err != nil {
		t.Fatalf("SetCredentials failed: %v", err)
	}

	if !authMgr.IsConfigured() {
		t.Fatalf("expected IsConfigured to be true after setting credentials")
	}

	// 2. Verify encrypted on disk: plaintext username & password must NOT appear in server-auth.json!
	authFile := filepath.Join(tmpDir, "server-auth.json")
	data, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("failed to read %s: %v", authFile, err)
	}
	content := string(data)

	if strings.Contains(content, "ServerAdmin") {
		t.Errorf("plaintext username 'ServerAdmin' found in %s! Must be encrypted.", authFile)
	}
	if strings.Contains(content, "SuperSecretPass123!") {
		t.Errorf("plaintext password found in %s! Must be hashed with salt.", authFile)
	}

	// Verify decrypted username retrieves original casing
	if u := authMgr.GetUsername(); u != "ServerAdmin" {
		t.Errorf("expected decrypted username 'ServerAdmin', got %q", u)
	}

	// 3. Test Authentication
	if !authMgr.Authenticate("ServerAdmin", "SuperSecretPass123!") {
		t.Errorf("expected authentication to succeed with correct credentials")
	}
	if !authMgr.Authenticate("serveradmin", "SuperSecretPass123!") {
		t.Errorf("expected case-insensitive username authentication to succeed")
	}
	if authMgr.Authenticate("ServerAdmin", "WrongPassword") {
		t.Errorf("authentication should fail with wrong password")
	}
	if authMgr.Authenticate("WrongUser", "SuperSecretPass123!") {
		t.Errorf("authentication should fail with wrong username")
	}

	// 4. Test Session Management
	token, err := authMgr.CreateSession("ServerAdmin")
	if err != nil || token == "" {
		t.Fatalf("failed to create session: %v", err)
	}

	u, valid := authMgr.ValidateToken(token)
	if !valid || u != "ServerAdmin" {
		t.Errorf("expected token to be valid for ServerAdmin, got valid=%v user=%s", valid, u)
	}

	// Test revocation (logout)
	authMgr.RevokeToken(token)
	_, validAfterRevoke := authMgr.ValidateToken(token)
	if validAfterRevoke {
		t.Errorf("expected token to be invalid after revocation")
	}
}

func TestAdminModSidesAndTextFiles(t *testing.T) {
	tmpDir := t.TempDir()

	srv := New(ServerOptions{
		RootDir:     tmpDir,
		SyncDirs:    []string{"mods"},
		Port:        8080,
		AdminUser:   "admin",
		AdminPass:   "pass123",
		AutoRefresh: 100 * time.Millisecond,
	})

	modMgr := srv.modMgr

	// 1. Add "Both" mod (default shared mod)
	coreModContent := []byte("shared mod binary")
	err := modMgr.AddOrUpdateMod("lunaris-core.jar", bytes.NewReader(coreModContent), SideBoth)
	if err != nil {
		t.Fatalf("failed to add both mod: %v", err)
	}

	// Verify file is in mods/
	if _, err := os.Stat(filepath.Join(tmpDir, "mods", "lunaris-core.jar")); err != nil {
		t.Errorf("expected lunaris-core.jar in mods/: %v", err)
	}

	// 2. Add "Client Only" mod (e.g. rubidium.jar)
	clientModContent := []byte("client graphics boost mod")
	err = modMgr.AddOrUpdateMod("rubidium-mc1.20.1.jar", bytes.NewReader(clientModContent), SideClientOnly)
	if err != nil {
		t.Fatalf("failed to add client-only mod: %v", err)
	}

	// Verify file is in client-only/
	if _, err := os.Stat(filepath.Join(tmpDir, "client-only", "rubidium-mc1.20.1.jar")); err != nil {
		t.Errorf("expected rubidium-mc1.20.1.jar in client-only/: %v", err)
	}

	// Verify listed in client-only-mods.txt (plain name, not hash)
	clientList, err := modMgr.GetClientOnlyList()
	if err != nil || len(clientList) != 1 || clientList[0] != "rubidium-mc1.20.1.jar" {
		t.Errorf("expected client-only-mods.txt to contain rubidium-mc1.20.1.jar, got %v (err: %v)", clientList, err)
	}

	// 3. Add "Server Only" mod (e.g. chunky.jar)
	serverModContent := []byte("server chunk pregenerator mod")
	err = modMgr.AddOrUpdateMod("chunky-1.3.92.jar", bytes.NewReader(serverModContent), SideServerOnly)
	if err != nil {
		t.Fatalf("failed to add server-only mod: %v", err)
	}

	// Verify file is in mods/ (so server can run it)
	if _, err := os.Stat(filepath.Join(tmpDir, "mods", "chunky-1.3.92.jar")); err != nil {
		t.Errorf("expected chunky-1.3.92.jar in mods/: %v", err)
	}

	// Verify listed in server-only-mods.txt (plain name, not hash)
	serverList, err := modMgr.GetServerOnlyList()
	if err != nil || len(serverList) != 1 || serverList[0] != "chunky-1.3.92.jar" {
		t.Errorf("expected server-only-mods.txt to contain chunky-1.3.92.jar, got %v (err: %v)", serverList, err)
	}

	// 4. Test Manifest Generation:
	// - Both mod (lunaris-core.jar) MUST be present with DestPath="mods/lunaris-core.jar"
	// - Client-only mod (rubidium-mc1.20.1.jar) MUST be present with DestPath="mods/rubidium-mc1.20.1.jar"
	// - Server-only mod (chunky-1.3.92.jar) MUST be EXCLUDED so clients never download it!
	m, err := srv.RefreshManifest()
	if err != nil {
		t.Fatalf("RefreshManifest failed: %v", err)
	}

	fMap := m.FileMap()

	if _, ok := fMap["mods/lunaris-core.jar"]; !ok {
		t.Errorf("expected lunaris-core.jar in manifest")
	}

	clientEntry, ok := fMap["mods/rubidium-mc1.20.1.jar"]
	if !ok {
		t.Fatalf("expected rubidium-mc1.20.1.jar in manifest for client sync")
	}
	if clientEntry.DestPath != "mods/rubidium-mc1.20.1.jar" {
		t.Errorf("expected DestPath mods/rubidium-mc1.20.1.jar, got %s", clientEntry.DestPath)
	}

	if _, ok := fMap["mods/chunky-1.3.92.jar"]; ok {
		t.Errorf("server-only mod chunky-1.3.92.jar MUST NOT be included in client manifest!")
	}

	// 5. Test File Downloads via HTTP
	handler := srv.Handler()

	// Download client-only mod via client path /mods/rubidium-mc1.20.1.jar
	reqClientMod := httptest.NewRequest(http.MethodGet, "/mods/rubidium-mc1.20.1.jar", nil)
	rrClientMod := httptest.NewRecorder()
	handler.ServeHTTP(rrClientMod, reqClientMod)
	if rrClientMod.Code != http.StatusOK {
		t.Fatalf("expected 200 for client mod download, got %d", rrClientMod.Code)
	}
	if rrClientMod.Body.String() != string(clientModContent) {
		t.Errorf("expected content %q, got %q", string(clientModContent), rrClientMod.Body.String())
	}

	// 6. Test Mod Side Switch (move client-only mod to Both)
	err = modMgr.SetModSide("rubidium-mc1.20.1.jar", SideBoth)
	if err != nil {
		t.Fatalf("failed to set mod side to Both: %v", err)
	}

	// File should now be in mods/ and removed from client-only-mods.txt
	if _, err := os.Stat(filepath.Join(tmpDir, "mods", "rubidium-mc1.20.1.jar")); err != nil {
		t.Errorf("expected rubidium-mc1.20.1.jar moved to mods/: %v", err)
	}
	newClientList, _ := modMgr.GetClientOnlyList()
	if len(newClientList) != 0 {
		t.Errorf("expected client-only-mods.txt to be empty after moving to Both, got %v", newClientList)
	}

	// 7. Test Mod Delete
	err = modMgr.DeleteMod("lunaris-core.jar")
	if err != nil {
		t.Fatalf("DeleteMod failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "mods", "lunaris-core.jar")); !os.IsNotExist(err) {
		t.Errorf("expected lunaris-core.jar to be deleted from disk")
	}
}

func TestFolderDropSyncAndAdd(t *testing.T) {
	tmpDir := t.TempDir()

	srv := New(ServerOptions{
		RootDir:   tmpDir,
		SyncDirs:  []string{"mods"},
		Port:      8080,
		AdminUser: "admin",
		AdminPass: "secret",
	})

	handler := srv.Handler()

	// Log in to get session token
	loginBody := `{"username":"admin","password":"secret"}`
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/admin/auth/login", strings.NewReader(loginBody))
	rrLogin := httptest.NewRecorder()
	handler.ServeHTTP(rrLogin, reqLogin)

	var loginResp struct {
		Success bool   `json:"success"`
		Token   string `json:"token"`
	}
	_ = json.Unmarshal(rrLogin.Body.Bytes(), &loginResp)
	if !loginResp.Success || loginResp.Token == "" {
		t.Fatalf("login failed: %s", rrLogin.Body.String())
	}

	token := loginResp.Token

	// Initial setup: Server has old-mod.jar and keep-mod.jar
	_ = srv.modMgr.AddOrUpdateMod("old-mod.jar", strings.NewReader("old mod"), SideBoth)
	_ = srv.modMgr.AddOrUpdateMod("keep-mod.jar", strings.NewReader("keep mod"), SideBoth)

	// 1. Simulate dropping a mod folder with "remove_missing = true" (Full Sync)
	// Dropped folder contains: new-mod.jar and keep-mod.jar (old-mod.jar is missing!)
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("remove_missing", "true")
	_ = w.WriteField("sides", `{"new-mod.jar":"client_only","keep-mod.jar":"both"}`)

	part1, _ := w.CreateFormFile("files", "new-mod.jar")
	_, _ = part1.Write([]byte("new mod content"))
	part2, _ := w.CreateFormFile("files", "keep-mod.jar")
	_, _ = part2.Write([]byte("updated keep mod content"))
	_ = w.Close()

	reqUpload := httptest.NewRequest(http.MethodPost, "/api/admin/mods/upload", &b)
	reqUpload.Header.Set("Content-Type", w.FormDataContentType())
	reqUpload.Header.Set("Authorization", "Bearer "+token)
	rrUpload := httptest.NewRecorder()
	handler.ServeHTTP(rrUpload, reqUpload)

	if rrUpload.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", rrUpload.Code, rrUpload.Body.String())
	}

	// Verify old-mod.jar was deleted because remove_missing was true
	mods, _ := srv.modMgr.ListAllMods()
	modNames := make(map[string]ModSide)
	for _, m := range mods {
		modNames[m.Filename] = m.Side
	}

	if _, ok := modNames["old-mod.jar"]; ok {
		t.Errorf("expected old-mod.jar to be removed during folder sync with remove_missing=true")
	}
	if _, ok := modNames["keep-mod.jar"]; !ok {
		t.Errorf("expected keep-mod.jar to be present")
	}
	if side, ok := modNames["new-mod.jar"]; !ok || side != SideClientOnly {
		t.Errorf("expected new-mod.jar with side client_only, got %v", side)
	}

	// Verify client-only-mods.txt contains new-mod.jar
	clientTxt, _ := srv.modMgr.GetClientOnlyList()
	hasNewMod := false
	for _, c := range clientTxt {
		if c == "new-mod.jar" {
			hasNewMod = true
			break
		}
	}
	if !hasNewMod {
		t.Errorf("expected client-only-mods.txt to contain new-mod.jar")
	}

	// 2. Simulate dropping with "remove_missing = false" (Just Add)
	var b2 bytes.Buffer
	w2 := multipart.NewWriter(&b2)
	_ = w2.WriteField("remove_missing", "false")
	part3, _ := w2.CreateFormFile("files", "another-mod.jar")
	_, _ = part3.Write([]byte("another mod content"))
	_ = w2.Close()

	reqUpload2 := httptest.NewRequest(http.MethodPost, "/api/admin/mods/upload", &b2)
	reqUpload2.Header.Set("Content-Type", w2.FormDataContentType())
	reqUpload2.Header.Set("Authorization", "Bearer "+token)
	rrUpload2 := httptest.NewRecorder()
	handler.ServeHTTP(rrUpload2, reqUpload2)

	// Verify existing mods (keep-mod, new-mod) are still preserved
	mods2, _ := srv.modMgr.ListAllMods()
	if len(mods2) != 3 {
		t.Errorf("expected 3 mods after additive upload, got %d", len(mods2))
	}
}

func TestAdminProcessAndBootFailurePrompt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create simulated failure log in logs/latest.log
	logsDir := filepath.Join(tmpDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)
	crashLog := `[18:00:00] [Server thread/INFO] [minecraft/Main]: Loading Minecraft 1.20.1
[18:00:02] [Server thread/ERROR] [minecraft/MinecraftServer]: Encountered an unexpected exception
java.lang.NullPointerException: Cannot invoke mod initializer
	at net.minecraft.server.Main.main(Main.java:123)
[18:00:02] [Server thread/ERROR] [minecraft/MinecraftServer]: This crash report has been saved to: crash-reports/crash-2026-11-04_18.00.02-server.txt`
	_ = os.WriteFile(filepath.Join(logsDir, "latest.log"), []byte(crashLog), 0644)

	// Configure server with a start command that exits with code 1 (simulating boot failure)
	srv := New(ServerOptions{
		RootDir:      tmpDir,
		Port:         8080,
		AdminUser:    "admin",
		AdminPass:    "pass",
		StartCommand: "exit 1",
	})

	pm := srv.processMgr

	// Test 1: Start process that immediately fails
	err := pm.Start()
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	// Wait briefly for process to exit
	time.Sleep(300 * time.Millisecond)

	status, _, _, failedBoot, reason, _ := pm.GetStatus()
	if status != StatusFailedToBoot && !failedBoot {
		t.Errorf("expected status failed_to_boot or failedBoot=true, got status=%s failedBoot=%v reason=%s", status, failedBoot, reason)
	}

	// Test 2: Fetch latest log for debugging
	content, source, err := pm.GetLatestLog()
	if err != nil {
		t.Fatalf("GetLatestLog failed: %v", err)
	}
	if !strings.Contains(content, "Encountered an unexpected exception") {
		t.Errorf("expected crash log content in GetLatestLog, got: %s", content)
	}
	if source != "logs/latest.log" {
		t.Errorf("expected source logs/latest.log, got %s", source)
	}

	// Test 3: HTTP API endpoint for latest log
	handler := srv.Handler()
	token, _ := srv.authMgr.CreateSession("admin")

	reqLog := httptest.NewRequest(http.MethodGet, "/api/admin/server/logs/latest", nil)
	reqLog.Header.Set("Authorization", "Bearer "+token)
	rrLog := httptest.NewRecorder()
	handler.ServeHTTP(rrLog, reqLog)

	if rrLog.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/admin/server/logs/latest, got %d", rrLog.Code)
	}

	var logResp struct {
		Success bool   `json:"success"`
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	_ = json.Unmarshal(rrLog.Body.Bytes(), &logResp)
	if !logResp.Success || !strings.Contains(logResp.Content, "NullPointerException") {
		t.Errorf("expected latest log in API response, got: %v", logResp)
	}
}

func TestAdminPageAndNetworkInfo(t *testing.T) {
	tmpDir := t.TempDir()

	srv := New(ServerOptions{
		RootDir:   tmpDir,
		Port:      9090,
		Host:      "0.0.0.0",
		AdminUser: "admin",
		AdminPass: "admin123",
	})

	handler := srv.Handler()

	// Test 1: GET /admin serves AdminHTML
	reqAdmin := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rrAdmin := httptest.NewRecorder()
	handler.ServeHTTP(rrAdmin, reqAdmin)

	if rrAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 for /admin, got %d", rrAdmin.Code)
	}
	if !strings.Contains(rrAdmin.Body.String(), "Lunaris Server Admin") {
		t.Errorf("expected AdminHTML content in /admin")
	}

	// Test 2: Network Info API
	token, _ := srv.authMgr.CreateSession("admin")
	reqNet := httptest.NewRequest(http.MethodGet, "/api/admin/network", nil)
	reqNet.Header.Set("Authorization", "Bearer "+token)
	rrNet := httptest.NewRecorder()
	handler.ServeHTTP(rrNet, reqNet)

	if rrNet.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/admin/network, got %d", rrNet.Code)
	}

	var netResp struct {
		Success  bool   `json:"success"`
		Port     int    `json:"port"`
		PortOpen bool   `json:"port_open"`
		Host     string `json:"host"`
	}
	_ = json.Unmarshal(rrNet.Body.Bytes(), &netResp)
	if !netResp.Success || netResp.Port != 9090 || netResp.Host != "0.0.0.0" {
		t.Errorf("unexpected network response: %v", netResp)
	}
}
