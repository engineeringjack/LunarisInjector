package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeSHA256(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world lunaris injector")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	hash, size, err := ComputeSHA256(testFile)
	if err != nil {
		t.Fatalf("ComputeSHA256 failed: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	// echo -n "hello world lunaris injector" | sha256sum
	// 557db945be11b8162d9fb4c759535790be4bb04c553d1fb527ec56c70146059b
	expectedHash := "7ec1f9f1682f6d29568fe908498809c4421a8074150088b2ea24465c077abd27"
	if hash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, hash)
	}
}

func TestScanDirectoryAndSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	modsDir := filepath.Join(tmpDir, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir: %v", err)
	}

	mod1 := filepath.Join(modsDir, "example1.jar")
	mod2 := filepath.Join(modsDir, "example2.jar")
	_ = os.WriteFile(mod1, []byte("mod 1 content"), 0644)
	_ = os.WriteFile(mod2, []byte("mod 2 content"), 0644)

	m, err := ScanDirectory(tmpDir, []string{"mods"}, nil)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	if len(m.Files) != 2 {
		t.Fatalf("expected 2 files in manifest, got %d", len(m.Files))
	}

	manifestFile := filepath.Join(tmpDir, "manifest.json")
	if err := m.SaveToFile(manifestFile); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	loaded, err := LoadFromFile(manifestFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if len(loaded.Files) != 2 {
		t.Fatalf("expected 2 loaded files, got %d", len(loaded.Files))
	}

	fmap := loaded.FileMap()
	if _, ok := fmap["mods/example1.jar"]; !ok {
		t.Errorf("missing mods/example1.jar in filemap")
	}
}
