package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileEntry represents a tracked file in the manifest.
type FileEntry struct {
	Path   string `json:"path"`   // Normalized relative path (e.g. "mods/fabric-api.jar")
	SHA256 string `json:"sha256"` // Hex-encoded SHA-256 hash
	Size   int64  `json:"size"`   // File size in bytes
}

// Manifest represents the complete list of files and their expected hashes.
type Manifest struct {
	Version   int         `json:"version"`   // Manifest format version
	Timestamp int64       `json:"timestamp"` // Unix timestamp when generated
	Files     []FileEntry `json:"files"`     // List of tracked files
}

// FileMap returns a lookup map of relative path -> FileEntry for quick matching.
func (m *Manifest) FileMap() map[string]FileEntry {
	res := make(map[string]FileEntry, len(m.Files))
	for _, f := range m.Files {
		res[NormalizePath(f.Path)] = f
	}
	return res
}

// NormalizePath ensures consistent forward-slash path format across Windows and Unix.
func NormalizePath(p string) string {
	cleaned := filepath.Clean(p)
	return strings.ReplaceAll(cleaned, "\\", "/")
}

// ComputeSHA256 calculates the SHA-256 hash and file size of a given local file.
func ComputeSHA256(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return "", 0, err
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))
	return hashStr, size, nil
}

// DefaultExcludes returns file patterns that should not be tracked or synced.
func DefaultExcludes() []string {
	return []string{
		".DS_Store",
		"thumbs.db",
		"*.tmp",
		"*.crdownload",
		"lunaris.json",
		"manifest.json",
	}
}

// ShouldIgnore checks whether a relative file path matches any exclusion pattern.
func ShouldIgnore(relPath string, patterns []string) bool {
	fileName := filepath.Base(relPath)
	allPatterns := append(DefaultExcludes(), patterns...)

	for _, pat := range allPatterns {
		if matched, _ := filepath.Match(strings.ToLower(pat), strings.ToLower(fileName)); matched {
			return true
		}
		if matched, _ := filepath.Match(strings.ToLower(pat), strings.ToLower(relPath)); matched {
			return true
		}
	}
	return false
}

// ScanDirectory scans the specified baseDir and subdirectories (e.g. "mods")
// and builds a Manifest with SHA-256 hashes and file sizes.
func ScanDirectory(baseDir string, subDirs []string, ignorePatterns []string) (*Manifest, error) {
	if len(subDirs) == 0 {
		subDirs = []string{"mods"}
	}

	manifest := &Manifest{
		Version:   1,
		Timestamp: time.Now().Unix(),
		Files:     []FileEntry{},
	}

	for _, subDir := range subDirs {
		targetPath := filepath.Join(baseDir, subDir)
		info, err := os.Stat(targetPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("failed to access directory %s: %w", targetPath, err)
		}

		if !info.IsDir() {
			continue
		}

		err = filepath.Walk(targetPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(baseDir, path)
			if err != nil {
				return err
			}
			normRel := NormalizePath(rel)

			if ShouldIgnore(normRel, ignorePatterns) {
				return nil
			}

			hash, size, err := ComputeSHA256(path)
			if err != nil {
				return fmt.Errorf("failed to hash %s: %w", path, err)
			}

			manifest.Files = append(manifest.Files, FileEntry{
				Path:   normRel,
				SHA256: hash,
				Size:   size,
			})
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("error walking %s: %w", targetPath, err)
		}
	}

	sort.Slice(manifest.Files, func(i, j int) bool {
		return manifest.Files[i].Path < manifest.Files[j].Path
	})

	return manifest, nil
}

// SaveToFile serializes the manifest to formatted JSON and writes it to a file.
func (m *Manifest) SaveToFile(filePath string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadFromFile reads a manifest from a local JSON file.
func LoadFromFile(filePath string) (*Manifest, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses a manifest from JSON bytes.
func LoadFromBytes(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest json: %w", err)
	}
	return &m, nil
}
