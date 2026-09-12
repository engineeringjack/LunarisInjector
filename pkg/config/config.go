package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const ConfigFileName = "lunaris.json"
const DefaultServerURL = "https://lunaris.csfrederick.com"

// Config contains the configuration settings for LunarisInjector.
type Config struct {
	// ServerURL is the HTTP/HTTPS endpoint of the sync server (e.g. "https://lunaris.csfrederick.com").
	ServerURL string `json:"server_url"`

	// GameVersion is the required Minecraft version (e.g. "1.20.1").
	GameVersion string `json:"game_version"`

	// RealJavaPath is the absolute path to the actual Java executable (java or javaw.exe).
	RealJavaPath string `json:"real_java_path"`

	// SyncDirs are relative directories to synchronize (e.g. ["mods", "config", "global_packs"]).
	SyncDirs []string `json:"sync_dirs"`

	// DeleteExtra determines whether local files not present in the remote manifest are deleted.
	DeleteExtra bool `json:"delete_extra"`

	// IgnoreFiles contains glob patterns for files that should not be touched or deleted.
	IgnoreFiles []string `json:"ignore_files"`

	// OfflineLaunch allows Minecraft to boot even if the sync server is unreachable.
	OfflineLaunch bool `json:"offline_launch"`

	// TimeoutSec is the HTTP request timeout in seconds.
	TimeoutSec int `json:"timeout_sec"`
}

// DefaultSyncDirs defines the default directories synchronized between server and client.
var DefaultSyncDirs = []string{"mods", "config", "global_packs"}

// DefaultConfig returns the standard default configuration.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:     DefaultServerURL,
		GameVersion:   "1.20.1",
		RealJavaPath:  "",
		SyncDirs:      []string{"mods", "config", "global_packs"},
		DeleteExtra:   true,
		IgnoreFiles:   []string{},
		OfflineLaunch: true,
		TimeoutSec:    10,
	}
}

// Load reads and parses a JSON config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 10
	}
	if len(cfg.SyncDirs) == 0 {
		cfg.SyncDirs = []string{"mods", "config", "global_packs"}
	}

	return cfg, nil
}

// Save writes the configuration to disk.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// FindInstanceConfig searches for lunaris.json in:
// 1. gameDir/lunaris.json
// 2. The directory of the current executable
// 3. Current working directory
func FindInstanceConfig(gameDir string) (*Config, string, error) {
	var candidatePaths []string

	if gameDir != "" {
		candidatePaths = append(candidatePaths, filepath.Join(gameDir, ConfigFileName))
	}

	if execPath, err := os.Executable(); err == nil {
		candidatePaths = append(candidatePaths, filepath.Join(filepath.Dir(execPath), ConfigFileName))
	}

	if cwd, err := os.Getwd(); err == nil {
		candidatePaths = append(candidatePaths, filepath.Join(cwd, ConfigFileName))
	}

	for _, p := range candidatePaths {
		if _, err := os.Stat(p); err == nil {
			cfg, err := Load(p)
			if err == nil {
				return cfg, p, nil
			}
		}
	}

	return nil, "", errors.New("lunaris.json configuration file not found")
}
