package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultGitHubRepo = "engineeringjack/LunarisInjector"
	UserAgent         = "LunarisInjector-Updater"
	CheckCooldown     = 4 * time.Hour // Do not hammer GitHub API on rapid restarts
)

// GitHubRelease represents the minimal release object from GitHub's Releases API.
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt string        `json:"published_at"`
	Assets      []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a binary asset attached to a GitHub Release.
type GitHubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ReleaseInfo holds the discovered update metadata.
type ReleaseInfo struct {
	Version   string
	AssetURL  string
	AssetName string
	AssetSize int64
}

// CheckUpdate checks GitHub Releases for a newer version of LunarisInjector.
func CheckUpdate(ctx context.Context, repo, currentVersion string) (*ReleaseInfo, bool, error) {
	if repo == "" {
		repo = DefaultGitHubRepo
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		// No release published yet on this repository
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, false, fmt.Errorf("failed to parse release info: %w", err)
	}

	if rel.Draft {
		return nil, false, nil
	}

	isNewer := IsNewerVersion(currentVersion, rel.TagName)
	if !isNewer {
		return nil, false, nil
	}

	asset := MatchAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return nil, false, fmt.Errorf("release %s has no matching binary for %s/%s", rel.TagName, runtime.GOOS, runtime.GOARCH)
	}

	return &ReleaseInfo{
		Version:   strings.TrimPrefix(rel.TagName, "v"),
		AssetURL:  asset.BrowserDownloadURL,
		AssetName: asset.Name,
		AssetSize: asset.Size,
	}, true, nil
}

// MatchAsset finds the appropriate asset for the current OS and architecture.
func MatchAsset(assets []GitHubAsset, goos, goarch string) *GitHubAsset {
	expectedSuffix := ""
	if goos == "windows" {
		expectedSuffix = ".exe"
	}

	for _, a := range assets {
		name := strings.ToLower(a.Name)

		// Skip archives (.zip, .tar.gz, .dmg, .deb) since self-updater replaces the raw binary
		if strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".dmg") || strings.HasSuffix(name, ".deb") {
			continue
		}

		// Filter by OS
		switch goos {
		case "windows":
			if !strings.Contains(name, "windows") && !strings.Contains(name, "win") {
				continue
			}
		case "darwin":
			if !strings.Contains(name, "darwin") && !strings.Contains(name, "macos") && !strings.Contains(name, "mac") {
				continue
			}
		case "linux":
			if !strings.Contains(name, "linux") {
				continue
			}
		}

		// Filter by Architecture
		switch goarch {
		case "amd64":
			if !strings.Contains(name, "amd64") && !strings.Contains(name, "x86_64") && !strings.Contains(name, "x64") {
				continue
			}
		case "arm64":
			if !strings.Contains(name, "arm64") && !strings.Contains(name, "aarch64") {
				continue
			}
		}

		if expectedSuffix != "" && !strings.HasSuffix(name, expectedSuffix) {
			continue
		}

		return &a
	}
	return nil
}

// IsNewerVersion returns true if remoteVer is strictly newer than currentVer according to SemVer.
func IsNewerVersion(currentVer, remoteVer string) bool {
	curParts := parseVersion(currentVer)
	remParts := parseVersion(remoteVer)

	for i := 0; i < len(curParts) && i < len(remParts); i++ {
		if remParts[i] > curParts[i] {
			return true
		}
		if remParts[i] < curParts[i] {
			return false
		}
	}
	return len(remParts) > len(curParts)
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	v = strings.Split(v, "-")[0] // Strip pre-release tags like -beta
	parts := strings.Split(v, ".")
	var nums []int
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil {
			nums = append(nums, n)
		} else {
			nums = append(nums, 0)
		}
	}
	return nums
}

// SelfUpdate downloads a new binary and replaces the currently running executable.
func SelfUpdate(ctx context.Context, assetURL string, logger func(string, ...interface{})) error {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate current executable: %w", err)
	}
	selfPath, err = filepath.EvalSymlinks(selfPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink for executable: %w", err)
	}

	selfDir := filepath.Dir(selfPath)
	binName := filepath.Base(selfPath)

	logger("[Updater] Downloading new binary from %s...", assetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	// Write to temporary file in the same directory (to ensure same filesystem mount for atomic rename)
	tmpPath := filepath.Join(selfDir, fmt.Sprintf(".%s.new.%d", binName, time.Now().UnixNano()))
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp binary: %w", err)
	}

	_, copyErr := io.Copy(tmpFile, resp.Body)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed writing downloaded binary: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed closing temp binary: %w", closeErr)
	}

	_ = os.Chmod(tmpPath, 0755)

	// Replace active executable
	if runtime.GOOS == "windows" {
		// Windows forbids overwriting a running executable, but permits renaming it!
		oldPath := filepath.Join(selfDir, fmt.Sprintf(".%s.old", binName))
		_ = os.Remove(oldPath) // Remove any previous old backup

		if err := os.Rename(selfPath, oldPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to rename existing Windows binary: %w", err)
		}

		if err := os.Rename(tmpPath, selfPath); err != nil {
			// Rollback if possible
			_ = os.Rename(oldPath, selfPath)
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to install new Windows binary: %w", err)
		}
	} else {
		// Unix (macOS / Linux) allows atomic rename over executing inode
		if err := os.Rename(tmpPath, selfPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed replacing Unix binary: %w", err)
		}
	}

	logger("[Updater] Binary successfully updated at %s", selfPath)
	return nil
}

// CleanupOld removes leftover .old binaries from previous updates (mainly on Windows).
func CleanupOld() {
	selfPath, err := os.Executable()
	if err != nil {
		return
	}
	selfDir := filepath.Dir(selfPath)
	binName := filepath.Base(selfPath)

	oldPath := filepath.Join(selfDir, fmt.Sprintf(".%s.old", binName))
	_ = os.Remove(oldPath)

	// Also clean up any lingering temporary download files older than 1 hour
	entries, err := os.ReadDir(selfDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "."+binName+".new.") {
				info, err := e.Info()
				if err == nil && time.Since(info.ModTime()) > time.Hour {
					_ = os.Remove(filepath.Join(selfDir, e.Name()))
				}
			}
		}
	}
}

// AutoCheckAndUpdate checks for updates if cooldown has elapsed, and applies them.
// It returns true if an update was downloaded and installed.
func AutoCheckAndUpdate(ctx context.Context, repo, currentVersion, cacheDir string, force bool, logger func(string, ...interface{})) (bool, error) {
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}

	CleanupOld()

	// Rate limiting via cooldown timestamp file
	if cacheDir != "" && !force {
		stampFile := filepath.Join(cacheDir, ".lunaris_update_check")
		if info, err := os.Stat(stampFile); err == nil {
			if time.Since(info.ModTime()) < CheckCooldown {
				// Checked recently, skip check
				return false, nil
			}
		}
		// Update cooldown stamp
		_ = os.WriteFile(stampFile, []byte(time.Now().Format(time.RFC3339)), 0644)
	}

	info, hasUpdate, err := CheckUpdate(ctx, repo, currentVersion)
	if err != nil {
		return false, err
	}
	if !hasUpdate || info == nil {
		return false, nil
	}

	logger("[Lunaris] ✦ An update is available: v%s (current: v%s)", info.Version, currentVersion)
	logger("[Lunaris] ✦ Downloading and applying update...")

	if err := SelfUpdate(ctx, info.AssetURL, logger); err != nil {
		return false, err
	}

	logger("[Lunaris] ✔ LunarisInjector updated to v%s!", info.Version)
	return true, nil
}
