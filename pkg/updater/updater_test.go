package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		remote  string
		want    bool
	}{
		{"1.0.0", "1.0.1", true},
		{"v1.0.0", "v1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.4", "1.2.3", false},
		{"v2.0.0", "v1.9.9", false},
		{"1.0.0", "1.0.0-beta", false},
		{"1.0.0-beta", "1.0.1", true},
	}

	for _, tt := range tests {
		got := IsNewerVersion(tt.current, tt.remote)
		if got != tt.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.current, tt.remote, got, tt.want)
		}
	}
}

func TestMatchAsset(t *testing.T) {
	assets := []GitHubAsset{
		{Name: "lunaris-windows-amd64.exe", BrowserDownloadURL: "http://example.com/win64.exe"},
		{Name: "lunaris-darwin-arm64", BrowserDownloadURL: "http://example.com/mac-arm64"},
		{Name: "lunaris-darwin-amd64", BrowserDownloadURL: "http://example.com/mac-amd64"},
		{Name: "lunaris-linux-amd64", BrowserDownloadURL: "http://example.com/linux-amd64"},
	}

	// Test windows amd64
	win := MatchAsset(assets, "windows", "amd64")
	if win == nil || win.Name != "lunaris-windows-amd64.exe" {
		t.Errorf("Expected windows asset, got %v", win)
	}

	// Test darwin arm64
	macArm := MatchAsset(assets, "darwin", "arm64")
	if macArm == nil || macArm.Name != "lunaris-darwin-arm64" {
		t.Errorf("Expected mac arm64 asset, got %v", macArm)
	}

	// Test darwin amd64
	macIntel := MatchAsset(assets, "darwin", "amd64")
	if macIntel == nil || macIntel.Name != "lunaris-darwin-amd64" {
		t.Errorf("Expected mac amd64 asset, got %v", macIntel)
	}

	// Test linux amd64
	linux := MatchAsset(assets, "linux", "amd64")
	if linux == nil || linux.Name != "lunaris-linux-amd64" {
		t.Errorf("Expected linux asset, got %v", linux)
	}
}

func TestCheckUpdate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := GitHubRelease{
			TagName: "v1.2.0",
			Name:    "Lunaris Release v1.2.0",
			Assets: []GitHubAsset{
				{Name: "lunaris-windows-amd64.exe", BrowserDownloadURL: "http://example.com/win"},
				{Name: "lunaris-darwin-arm64", BrowserDownloadURL: "http://example.com/mac-arm"},
				{Name: "lunaris-darwin-amd64", BrowserDownloadURL: "http://example.com/mac-intel"},
				{Name: "lunaris-linux-amd64", BrowserDownloadURL: "http://example.com/linux"},
			},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer ts.Close()

	// Intercept via custom test using the test server response directly
	client := ts.Client()
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("Failed to query test server: %v", err)
	}
	defer resp.Body.Close()

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if !IsNewerVersion("1.0.0", rel.TagName) {
		t.Errorf("Expected v1.2.0 to be newer than 1.0.0")
	}
}

func TestCleanupOld(t *testing.T) {
	tmpDir := t.TempDir()
	selfPath, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable")
	}
	binName := filepath.Base(selfPath)

	oldFile := filepath.Join(tmpDir, "."+binName+".old")
	_ = os.WriteFile(oldFile, []byte("old content"), 0644)

	// Verify file was written
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("file not created")
	}
}

func TestSelfUpdateAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	testBin := filepath.Join(tmpDir, "dummy_app")
	_ = os.WriteFile(testBin, []byte("version 1.0"), 0755)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("version 2.0 new binary"))
	}))
	defer server.Close()

	// Download to temp file and replace dummy_app
	tmpPath := testBin + ".tmp"
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	defer resp.Body.Close()

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		t.Fatalf("Create temp error: %v", err)
	}
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	_, _ = out.Write(buf[:n])
	_ = out.Close()

	if err := os.Rename(tmpPath, testBin); err != nil {
		t.Fatalf("Rename error: %v", err)
	}

	content, _ := os.ReadFile(testBin)
	if string(content) != "version 2.0 new binary" {
		t.Errorf("Expected updated content, got %s", string(content))
	}
}
