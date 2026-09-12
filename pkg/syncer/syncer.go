package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/manifest"
)

var ErrOfflineProceed = errors.New("remote server unreachable, launching offline")

// ProgressCallback is invoked during file operations.
type ProgressCallback func(currentFile string, completedFiles, totalFiles int, bytesDone, totalBytes int64)

// SyncOptions controls synchronization behavior.
type SyncOptions struct {
	Config      *config.Config
	GameDir     string
	WorkerCount int
	OnProgress  ProgressCallback
	Logger      func(format string, args ...interface{})
}

// Syncer handles comparing and synchronizing local game directories against a remote manifest.
type Syncer struct {
	opts       SyncOptions
	httpClient *http.Client
}

// New creates a new Syncer instance.
func New(opts SyncOptions) *Syncer {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = 4
	}
	if opts.Logger == nil {
		opts.Logger = func(format string, args ...interface{}) {
			fmt.Printf(format+"\n", args...)
		}
	}

	timeout := time.Duration(opts.Config.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Syncer{
		opts: opts,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// FetchRemoteManifest downloads and parses the manifest.json from the remote server.
func (s *Syncer) FetchRemoteManifest(ctx context.Context) (*manifest.Manifest, error) {
	baseURL := strings.TrimRight(s.opts.Config.ServerURL, "/")
	manifestURL := baseURL + "/manifest.json"

	s.opts.Logger("[Lunaris] Checking sync server: %s", manifestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "LunarisInjector/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return manifest.LoadFromBytes(body)
}

// Plan represents the actions needed to bring the local directory into sync.
type Plan struct {
	Downloads   []manifest.FileEntry
	Deletions   []string
	Unchanged   int
	TotalBytes  int64
}

// CalculatePlan compares local files against the remote manifest to determine diffs.
func (s *Syncer) CalculatePlan(remote *manifest.Manifest) (*Plan, error) {
	// Verify game versions if specified
	if remote.GameVersion != "" && s.opts.Config.GameVersion != "" {
		if !strings.EqualFold(remote.GameVersion, s.opts.Config.GameVersion) {
			return nil, fmt.Errorf("minecraft version mismatch: remote server is for version %s, but local config expects %s", remote.GameVersion, s.opts.Config.GameVersion)
		}
	}

	localManifest, err := manifest.ScanDirectory(s.opts.GameDir, s.opts.Config.SyncDirs, s.opts.Config.IgnoreFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to scan local directory: %w", err)
	}

	localMap := localManifest.FileMap()

	var features []string
	if s.opts.Config.EnableVR {
		features = append(features, "vr")
	}
	remoteMap := remote.FilteredMap(features)

	plan := &Plan{
		Downloads: []manifest.FileEntry{},
		Deletions: []string{},
	}

	// Determine files to download (new or changed)
	for path, remoteEntry := range remoteMap {
		localEntry, exists := localMap[path]
		if !exists || !strings.EqualFold(localEntry.SHA256, remoteEntry.SHA256) {
			plan.Downloads = append(plan.Downloads, remoteEntry)
			plan.TotalBytes += remoteEntry.Size
		} else {
			plan.Unchanged++
		}
	}

	// Determine files to delete (present locally but missing in remote)
	if s.opts.Config.DeleteExtra {
		for path := range localMap {
			if manifest.ShouldIgnore(path, s.opts.Config.IgnoreFiles) {
				continue
			}
			if _, exists := remoteMap[path]; !exists {
				plan.Deletions = append(plan.Deletions, path)
			}
		}
	}

	return plan, nil
}

// Sync performs the synchronization process.
func (s *Syncer) Sync(ctx context.Context) error {
	remoteManifest, err := s.FetchRemoteManifest(ctx)
	if err != nil {
		if s.opts.Config.OfflineLaunch {
			s.opts.Logger("[Lunaris] Warning: Remote sync server is offline or unreachable (%v).", err)
			s.opts.Logger("[Lunaris] Offline launch enabled. Starting game with existing local files.")
			return ErrOfflineProceed
		}
		return fmt.Errorf("failed to connect to sync server: %w", err)
	}

	plan, err := s.CalculatePlan(remoteManifest)
	if err != nil {
		return fmt.Errorf("failed to calculate sync diff: %w", err)
	}

	s.opts.Logger("[Lunaris] Sync status: %d unchanged, %d to download (%.2f MB), %d to delete",
		plan.Unchanged,
		len(plan.Downloads),
		float64(plan.TotalBytes)/(1024*1024),
		len(plan.Deletions),
	)

	// Step 1: Process deletions
	for _, relPath := range plan.Deletions {
		fullPath := filepath.Join(s.opts.GameDir, filepath.FromSlash(relPath))
		s.opts.Logger("[Lunaris] Deleting obsolete file: %s", relPath)
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			s.opts.Logger("[Lunaris] Warning: Failed to remove %s: %v", fullPath, err)
		}
	}

	// Step 2: Process downloads if needed
	if len(plan.Downloads) == 0 {
		s.opts.Logger("[Lunaris] Everything is up to date!")
		return nil
	}

	return s.downloadFiles(ctx, plan.Downloads, plan.TotalBytes)
}

func (s *Syncer) downloadFiles(ctx context.Context, downloads []manifest.FileEntry, totalBytes int64) error {
	var completedCount int64
	var completedBytes int64
	totalCount := int64(len(downloads))

	jobs := make(chan manifest.FileEntry, len(downloads))
	for _, d := range downloads {
		jobs <- d
	}
	close(jobs)

	var wg sync.WaitGroup
	errCh := make(chan error, len(downloads))

	baseURL := strings.TrimRight(s.opts.Config.ServerURL, "/")

	for i := 0; i < s.opts.WorkerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				downloadURL := buildFileURL(baseURL, entry.Path)
				clientPath := entry.ClientPath()
				targetPath := filepath.Join(s.opts.GameDir, filepath.FromSlash(clientPath))

				s.opts.Logger("[Lunaris] Downloading: %s (%.2f MB)...", clientPath, float64(entry.Size)/(1024*1024))

				err := s.downloadSingleFile(ctx, downloadURL, targetPath, entry.SHA256)
				if err != nil {
					errCh <- fmt.Errorf("failed to download %s: %w", clientPath, err)
					return
				}

				done := atomic.AddInt64(&completedCount, 1)
				curBytes := atomic.AddInt64(&completedBytes, entry.Size)

				if s.opts.OnProgress != nil {
					s.opts.OnProgress(clientPath, int(done), int(totalCount), curBytes, totalBytes)
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return <-errCh
	}

	s.opts.Logger("[Lunaris] All %d updates downloaded and verified successfully!", totalCount)
	return nil
}

func (s *Syncer) downloadSingleFile(ctx context.Context, fileURL, targetPath, expectedHash string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "LunarisInjector/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status: %s", resp.Status)
	}

	// Download to temporary file first
	tmpPath := targetPath + ".lunaris.tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	_, err = io.Copy(writer, resp.Body)
	tmpFile.Close()

	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download stream error: %w", err)
	}

	// Verify SHA-256
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("hash verification failed for %s: expected %s, got %s", targetPath, expectedHash, actualHash)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to move file into place: %w", err)
	}

	return nil
}

func buildFileURL(baseURL, relPath string) string {
	parts := strings.Split(relPath, "/")
	escapedParts := make([]string, len(parts))
	for i, p := range parts {
		escapedParts[i] = url.PathEscape(p)
	}
	return baseURL + "/" + strings.Join(escapedParts, "/")
}
