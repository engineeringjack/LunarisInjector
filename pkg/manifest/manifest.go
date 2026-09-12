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
	Path     string `json:"path"`                // Normalized relative path on server (e.g. "mods/foo.jar" or "optional/vr/mods/vivecraft.jar")
	SHA256   string `json:"sha256"`              // Hex-encoded SHA-256 hash
	Size     int64  `json:"size"`                // File size in bytes
	Feature  string `json:"feature,omitempty"`   // Optional feature tag (e.g. "vr")
	DestPath string `json:"dest_path,omitempty"` // Destination relative path on client (e.g. "mods/vivecraft.jar")
}

// ClientPath returns the relative destination path on the client machine.
func (f FileEntry) ClientPath() string {
	if f.DestPath != "" {
		return NormalizePath(f.DestPath)
	}
	return NormalizePath(f.Path)
}

// Manifest represents the complete list of files and their expected hashes.
type Manifest struct {
	Version         int         `json:"version"`                    // Manifest format version
	GameVersion     string      `json:"game_version,omitempty"`     // Target Minecraft version (e.g. "1.20.1")
	Timestamp       int64       `json:"timestamp"`                  // Unix timestamp when generated
	Files           []FileEntry `json:"files"`                      // List of tracked files
}

// FileMap returns a lookup map of client relative path -> FileEntry for quick matching.
func (m *Manifest) FileMap() map[string]FileEntry {
	res := make(map[string]FileEntry, len(m.Files))
	for _, f := range m.Files {
		res[f.ClientPath()] = f
	}
	return res
}

// FilteredMap returns a lookup map of client relative path -> FileEntry,
// filtering out any optional features that are not explicitly enabled.
func (m *Manifest) FilteredMap(enabledFeatures []string) map[string]FileEntry {
	featMap := make(map[string]bool, len(enabledFeatures))
	for _, feat := range enabledFeatures {
		featMap[strings.ToLower(feat)] = true
	}

	res := make(map[string]FileEntry, len(m.Files))
	for _, f := range m.Files {
		if f.Feature != "" && !featMap[strings.ToLower(f.Feature)] {
			continue
		}
		res[f.ClientPath()] = f
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

// DefaultSyncDirs defines the default directories tracked for synchronization.
var DefaultSyncDirs = []string{"mods", "config", "global_packs"}

// DefaultExcludes returns file patterns that should not be tracked or synced.
func DefaultExcludes() []string {
	return []string{
		".DS_Store",
		"thumbs.db",
		"*.tmp",
		"*.crdownload",
		"lunaris.json",
		"manifest.json",
		"embeddium-fingerprint.json",
		"player-volumes.properties",
		"category-volumes.properties",
		"username-cache.json",
	}
}

// ShouldIgnore checks whether a relative file path matches any exclusion pattern.
func ShouldIgnore(relPath string, patterns []string) bool {
	fileName := filepath.Base(relPath)
	allPatterns := append(DefaultExcludes(), patterns...)

	lowerFile := strings.ToLower(fileName)
	lowerRel := strings.ToLower(relPath)

	for _, pat := range allPatterns {
		pat = strings.ToLower(pat)
		if matched, _ := filepath.Match(pat, lowerFile); matched {
			return true
		}
		if matched, _ := filepath.Match(pat, lowerRel); matched {
			return true
		}
		if strings.HasSuffix(pat, "/") && strings.HasPrefix(lowerRel, pat) {
			return true
		}
	}
	return false
}

// ScanDirectory scans the specified baseDir and subdirectories (e.g. "mods", "config", "global_packs")
// and builds a Manifest with SHA-256 hashes and file sizes.
func ScanDirectory(baseDir string, subDirs []string, ignorePatterns []string) (*Manifest, error) {
	if len(subDirs) == 0 {
		subDirs = DefaultSyncDirs
	}

	gameVersion := "1.20.1"
	// Try to detect gameVersion from minecraftinstance.json
	mcJSON := filepath.Join(baseDir, "minecraftinstance.json")
	if data, err := os.ReadFile(mcJSON); err == nil {
		var meta struct {
			GameVersion string `json:"gameVersion"`
		}
		if json.Unmarshal(data, &meta) == nil && meta.GameVersion != "" {
			gameVersion = meta.GameVersion
		}
	}

	manifest := &Manifest{
		Version:     1,
		GameVersion: gameVersion,
		Timestamp:   time.Now().Unix(),
		Files:       []FileEntry{},
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

			var feature string
			var destPath string
			if strings.HasPrefix(normRel, "optional/") {
				parts := strings.Split(normRel, "/")
				if len(parts) >= 3 {
					feature = parts[1]
					destPath = strings.Join(parts[2:], "/")
				}
			}

			manifest.Files = append(manifest.Files, FileEntry{
				Path:     normRel,
				SHA256:   hash,
				Size:     size,
				Feature:  feature,
				DestPath: destPath,
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
