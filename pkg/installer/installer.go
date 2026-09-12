package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/injector"
)

const TargetMinecraftVersion = "1.20.1"

// InstanceInfo represents a detected Minecraft instance from CurseForge, Vanilla, Prism, etc.
type InstanceInfo struct {
	Name             string `json:"name"`
	Launcher         string `json:"launcher"` // "CurseForge", "Vanilla", "Prism", "Modrinth", "Custom"
	Path             string `json:"path"`     // Path to the instance directory (containing mods, configs)
	GameVersion      string `json:"game_version"` // Detected Minecraft version (e.g. "1.20.1")
	IsCompatible     bool   `json:"is_compatible"`// Whether game version matches TargetMinecraftVersion
	ProfileFile      string `json:"profile_file"`
	ProfileID        string `json:"profile_id"`
	CurrentJava      string `json:"current_java"`
	IsInjected       bool   `json:"is_injected"`
	ConfiguredServer string `json:"configured_server,omitempty"`
}

// DetectInstances scans known paths for installed Minecraft modpacks and instances.
func DetectInstances() []InstanceInfo {
	var instances []InstanceInfo
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return instances
	}

	seenPaths := make(map[string]bool)

	// 1. CurseForge Instances
	curseForgeDirs := getCurseForgeDirectories(homeDir)
	for _, cfDir := range curseForgeDirs {
		entries, err := os.ReadDir(cfDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					instPath := filepath.Join(cfDir, entry.Name())
					cleanPath := filepath.Clean(instPath)
					if seenPaths[cleanPath] {
						continue
					}
					info := inspectCurseForgeInstance(cleanPath, entry.Name())
					if info != nil {
						seenPaths[cleanPath] = true
						instances = append(instances, *info)
					}
				}
			}
		}
	}

	// 2. Vanilla Minecraft Launcher profiles (~/.minecraft/launcher_profiles.json)
	vanillaProfiles := getVanillaProfilesPaths(homeDir)
	for _, pPath := range vanillaProfiles {
		vInstances := inspectVanillaLauncherProfiles(pPath)
		for _, vi := range vInstances {
			cleanPath := filepath.Clean(vi.Path)
			if !seenPaths[cleanPath] {
				seenPaths[cleanPath] = true
				instances = append(instances, vi)
			}
		}
	}

	// 3. Prism Launcher instances
	prismDirs := getPrismDirectories(homeDir)
	for _, pDir := range prismDirs {
		entries, err := os.ReadDir(pDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					instPath := filepath.Join(pDir, entry.Name())
					cleanPath := filepath.Clean(instPath)
					if seenPaths[cleanPath] {
						continue
					}
					info := inspectPrismInstance(cleanPath, entry.Name())
					if info != nil {
						seenPaths[cleanPath] = true
						instances = append(instances, *info)
					}
				}
			}
		}
	}

	return instances
}

func getCurseForgeDirectories(homeDir string) []string {
	var candidateDirs []string

	// 1. Inspect CurseForge's own storage.json configuration
	var storagePaths []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			storagePaths = append(storagePaths, filepath.Join(appData, "CurseForge", "storage.json"))
		}
		userProfile := os.Getenv("USERPROFILE")
		if userProfile != "" {
			storagePaths = append(storagePaths, filepath.Join(userProfile, "AppData", "Roaming", "CurseForge", "storage.json"))
		}
	} else if runtime.GOOS == "darwin" {
		storagePaths = append(storagePaths, filepath.Join(homeDir, "Library", "Application Support", "CurseForge", "storage.json"))
	} else {
		// Linux
		storagePaths = append(storagePaths,
			filepath.Join(homeDir, ".config", "CurseForge", "storage.json"),
			filepath.Join(homeDir, ".var", "app", "com.curseforge.CurseForge", "config", "CurseForge", "storage.json"),
		)
	}

	for _, sPath := range storagePaths {
		if data, err := os.ReadFile(sPath); err == nil {
			var root map[string]interface{}
			if json.Unmarshal(data, &root) == nil {
				if mcSettingsRaw, ok := root["minecraft-settings"].(string); ok {
					var mcSettings struct {
						MinecraftRoot string `json:"minecraftRoot"`
					}
					if json.Unmarshal([]byte(mcSettingsRaw), &mcSettings) == nil && mcSettings.MinecraftRoot != "" {
						candidateDirs = append(candidateDirs, filepath.Join(mcSettings.MinecraftRoot, "Instances"))
					}
				}
			}
		}
	}

	// 2. Standard filesystem paths across operating systems
	candidateDirs = append(candidateDirs,
		filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Instances"),
		filepath.Join(homeDir, "curseforge", "minecraft", "Instances"),
		filepath.Join(homeDir, ".curseforge", "minecraft", "Instances"),
		filepath.Join(homeDir, ".var", "app", "com.curseforge.CurseForge", "data", "curseforge", "minecraft", "Instances"),
	)

	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		if userProfile != "" {
			candidateDirs = append(candidateDirs,
				filepath.Join(userProfile, "Documents", "curseforge", "minecraft", "Instances"),
				filepath.Join(userProfile, "curseforge", "minecraft", "Instances"),
			)
		}
		candidateDirs = append(candidateDirs, `C:\curseforge\minecraft\Instances`)
	}

	// Filter and deduplicate
	var result []string
	seen := make(map[string]bool)
	for _, dir := range candidateDirs {
		clean := filepath.Clean(dir)
		if !seen[clean] {
			seen[clean] = true
			if info, err := os.Stat(clean); err == nil && info.IsDir() {
				result = append(result, clean)
			}
		}
	}

	return result
}

func inspectCurseForgeInstance(instPath, fallbackName string) *InstanceInfo {
	mcInstJSON := filepath.Join(instPath, "minecraftinstance.json")
	name := fallbackName
	gameVersion := ""

	if data, err := os.ReadFile(mcInstJSON); err == nil {
		var meta struct {
			Name        string `json:"name"`
			GameVersion string `json:"gameVersion"`
			BaseModLoader struct {
				MinecraftVersion string `json:"minecraftVersion"`
			} `json:"baseModLoader"`
			Manifest struct {
				Minecraft struct {
					Version string `json:"version"`
				} `json:"minecraft"`
			} `json:"manifest"`
		}
		if json.Unmarshal(data, &meta) == nil {
			if meta.Name != "" {
				name = meta.Name
			}
			if meta.GameVersion != "" {
				gameVersion = meta.GameVersion
			} else if meta.BaseModLoader.MinecraftVersion != "" {
				gameVersion = meta.BaseModLoader.MinecraftVersion
			} else if meta.Manifest.Minecraft.Version != "" {
				gameVersion = meta.Manifest.Minecraft.Version
			}
		}
	} else {
		// Fallback: check if mods directory exists
		if _, err := os.Stat(filepath.Join(instPath, "mods")); err != nil {
			return nil
		}
	}

	info := &InstanceInfo{
		Name:         name,
		Launcher:     "CurseForge",
		Path:         instPath,
		GameVersion:  gameVersion,
		IsCompatible: (gameVersion == TargetMinecraftVersion),
	}

	// Check if lunaris.json exists
	cfgPath := filepath.Join(instPath, config.ConfigFileName)
	if cfg, err := config.Load(cfgPath); err == nil {
		info.IsInjected = true
		info.ConfiguredServer = cfg.ServerURL
		info.CurrentJava = cfg.RealJavaPath
	}

	// Check if instance has its own launcher_profiles.json
	instProfiles := filepath.Join(instPath, "launcher_profiles.json")
	if _, err := os.Stat(instProfiles); err == nil {
		info.ProfileFile = instProfiles
	}

	return info
}

func getVanillaProfilesPaths(homeDir string) []string {
	var paths []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			paths = append(paths, filepath.Join(appData, ".minecraft", "launcher_profiles.json"))
		}
	} else if runtime.GOOS == "darwin" {
		paths = append(paths, filepath.Join(homeDir, "Library", "Application Support", "minecraft", "launcher_profiles.json"))
	} else {
		paths = append(paths, filepath.Join(homeDir, ".minecraft", "launcher_profiles.json"))
	}
	return paths
}

func inspectVanillaLauncherProfiles(profilePath string) []InstanceInfo {
	var list []InstanceInfo
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return list
	}

	var root struct {
		Profiles map[string]struct {
			Name          string `json:"name"`
			GameDir       string `json:"gameDir"`
			JavaDir       string `json:"javaDir"`
			LastVersionID string `json:"lastVersionId"`
			Type          string `json:"type"`
		} `json:"profiles"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return list
	}

	for id, p := range root.Profiles {
		if p.Name == "" {
			p.Name = id
		}
		gDir := p.GameDir
		if gDir == "" {
			gDir = filepath.Dir(profilePath)
		}

		gameVersion := parseVersionFromID(p.LastVersionID)

		info := InstanceInfo{
			Name:         p.Name,
			Launcher:     "Vanilla Launcher",
			Path:         gDir,
			GameVersion:  gameVersion,
			IsCompatible: (gameVersion == TargetMinecraftVersion),
			ProfileFile:  profilePath,
			ProfileID:    id,
			CurrentJava:  p.JavaDir,
		}

		cfgPath := filepath.Join(gDir, config.ConfigFileName)
		if cfg, err := config.Load(cfgPath); err == nil {
			info.IsInjected = true
			info.ConfiguredServer = cfg.ServerURL
		}

		list = append(list, info)
	}

	return list
}

func parseVersionFromID(versionID string) string {
	if strings.Contains(versionID, "1.20.1") {
		return "1.20.1"
	}
	if strings.Contains(versionID, "26.2") {
		return "1.26.2"
	}
	if versionID == "latest-release" || versionID == "latest-snapshot" {
		return versionID
	}
	return versionID
}

func getPrismDirectories(homeDir string) []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			dirs = append(dirs, filepath.Join(appData, "PrismLauncher", "instances"))
		}
	} else if runtime.GOOS == "darwin" {
		dirs = append(dirs, filepath.Join(homeDir, "Library", "Application Support", "PrismLauncher", "instances"))
	} else {
		dirs = append(dirs, filepath.Join(homeDir, ".local", "share", "PrismLauncher", "instances"))
	}
	return dirs
}

func inspectPrismInstance(instPath, fallbackName string) *InstanceInfo {
	cfgFile := filepath.Join(instPath, "instance.cfg")
	if _, err := os.Stat(cfgFile); err != nil {
		return nil
	}

	name := fallbackName
	gamePath := filepath.Join(instPath, ".minecraft")
	if _, err := os.Stat(gamePath); err != nil {
		gamePath = instPath
	}

	gameVersion := DetectInstanceVersion(instPath)

	info := &InstanceInfo{
		Name:         name,
		Launcher:     "Prism",
		Path:         gamePath,
		GameVersion:  gameVersion,
		IsCompatible: (gameVersion == TargetMinecraftVersion),
	}

	cfgPath := filepath.Join(gamePath, config.ConfigFileName)
	if cfg, err := config.Load(cfgPath); err == nil {
		info.IsInjected = true
		info.ConfiguredServer = cfg.ServerURL
	}

	return info
}

// DetectInstanceVersion inspects a directory and returns its Minecraft version if discoverable.
func DetectInstanceVersion(instanceDir string) string {
	// 1. CurseForge minecraftinstance.json
	mcJSON := filepath.Join(instanceDir, "minecraftinstance.json")
	if data, err := os.ReadFile(mcJSON); err == nil {
		var meta struct {
			GameVersion string `json:"gameVersion"`
			BaseModLoader struct {
				MinecraftVersion string `json:"minecraftVersion"`
			} `json:"baseModLoader"`
			Manifest struct {
				Minecraft struct {
					Version string `json:"version"`
				} `json:"minecraft"`
			} `json:"manifest"`
		}
		if json.Unmarshal(data, &meta) == nil {
			if meta.GameVersion != "" {
				return meta.GameVersion
			}
			if meta.BaseModLoader.MinecraftVersion != "" {
				return meta.BaseModLoader.MinecraftVersion
			}
			if meta.Manifest.Minecraft.Version != "" {
				return meta.Manifest.Minecraft.Version
			}
		}
	}

	// 2. Prism instance.cfg
	cfgFile := filepath.Join(instanceDir, "instance.cfg")
	if data, err := os.ReadFile(cfgFile); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "IntendedVersion=") {
				return strings.TrimPrefix(l, "IntendedVersion=")
			}
		}
	}

	// 3. Vanilla launcher_profiles.json
	lpFile := filepath.Join(instanceDir, "launcher_profiles.json")
	if data, err := os.ReadFile(lpFile); err == nil {
		var root struct {
			Profiles map[string]struct {
				LastVersionID string `json:"lastVersionId"`
			} `json:"profiles"`
		}
		if json.Unmarshal(data, &root) == nil && len(root.Profiles) > 0 {
			hasMatching := false
			var firstVer string
			for _, p := range root.Profiles {
				v := parseVersionFromID(p.LastVersionID)
				if v == TargetMinecraftVersion {
					hasMatching = true
					break
				}
				if firstVer == "" && v != "" {
					firstVer = v
				}
			}
			if hasMatching {
				return TargetMinecraftVersion
			}
			if firstVer != "" {
				return firstVer
			}
		}
	}

	return ""
}

// InstallConfig contains options for setting up Lunaris on an instance.
type InstallConfig struct {
	InstanceDir       string // Directory of the instance (where mods/ folder is)
	ServerURL         string // Sync server URL
	RealJavaPath      string // Path to real Java (auto-detected if empty)
	LunarisBinary     string // Path to lunaris binary (auto-detected if empty)
	ProfileFile       string // Optional launcher_profiles.json to patch
	ProfileID         string // Optional profile ID to patch
	RequiredGameVersion string // Required Minecraft version (defaults to "1.20.1")
}

// Install sets up LunarisInjector for a given instance.
func Install(opts InstallConfig) error {
	if opts.InstanceDir == "" {
		return errors.New("instance directory is required")
	}

	info, err := os.Stat(opts.InstanceDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("instance directory does not exist: %s", opts.InstanceDir)
	}

	reqVersion := opts.RequiredGameVersion
	if reqVersion == "" {
		reqVersion = TargetMinecraftVersion
	}

	// Version Verification: do not allow install if version does not match
	detectedVer := DetectInstanceVersion(opts.InstanceDir)
	if detectedVer != "" && detectedVer != reqVersion {
		return fmt.Errorf("version mismatch: this modpack requires Minecraft %s, but selected instance '%s' is on Minecraft %s. Installation is not allowed.",
			reqVersion, filepath.Base(opts.InstanceDir), detectedVer)
	}

	// 1. Locate real Java runtime
	realJava := opts.RealJavaPath
	if realJava == "" {
		detectedJava, err := injector.FindRealJava("")
		if err != nil {
			return fmt.Errorf("failed to auto-detect real Java: %w", err)
		}
		realJava = detectedJava
	}

	// 2. Locate Lunaris executable
	lunarisBin := opts.LunarisBinary
	if lunarisBin == "" {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to determine executable path: %w", err)
		}
		lunarisBin = self
	}
	lunarisBin, _ = filepath.Abs(lunarisBin)

	// Ensure mods/ folder exists in instance
	_ = os.MkdirAll(filepath.Join(opts.InstanceDir, "mods"), 0755)

	// 3. Write lunaris.json in instance directory
	cfg := config.DefaultConfig()
	cfg.ServerURL = opts.ServerURL
	cfg.GameVersion = reqVersion
	cfg.RealJavaPath = realJava
	cfg.SyncDirs = []string{"mods", "config", "global_packs"}
	cfg.DeleteExtra = true
	cfg.OfflineLaunch = true

	cfgPath := filepath.Join(opts.InstanceDir, config.ConfigFileName)
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save %s: %w", cfgPath, err)
	}

	// 4. If a launcher_profiles.json is provided, patch the profile's javaDir
	if opts.ProfileFile != "" && opts.ProfileID != "" {
		if err := patchProfileJavaDir(opts.ProfileFile, opts.ProfileID, lunarisBin); err != nil {
			return fmt.Errorf("failed to patch profile in %s: %w", opts.ProfileFile, err)
		}
	} else {
		localProfiles := filepath.Join(opts.InstanceDir, "launcher_profiles.json")
		if _, err := os.Stat(localProfiles); err == nil {
			_ = patchAllProfilesInFile(localProfiles, lunarisBin)
		}
	}

	return nil
}

// Uninstall removes LunarisInjector configuration from an instance.
func Uninstall(instanceDir string, profileFile, profileID string) error {
	cfgPath := filepath.Join(instanceDir, config.ConfigFileName)
	if _, err := os.Stat(cfgPath); err == nil {
		_ = os.Remove(cfgPath)
	}

	if profileFile != "" {
		_ = unpatchProfileJavaDir(profileFile, profileID)
	}

	localProfiles := filepath.Join(instanceDir, "launcher_profiles.json")
	if _, err := os.Stat(localProfiles); err == nil {
		_ = unpatchAllProfilesInFile(localProfiles)
	}

	return nil
}

func patchProfileJavaDir(profilePath, profileID, newJavaDir string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	profilesRaw, ok := root["profiles"].(map[string]interface{})
	if !ok {
		return errors.New("no profiles object in JSON")
	}

	profileRaw, ok := profilesRaw[profileID].(map[string]interface{})
	if !ok {
		return fmt.Errorf("profile %s not found in JSON", profileID)
	}

	bakPath := profilePath + ".lunaris.bak"
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		_ = os.WriteFile(bakPath, data, 0644)
	}

	profileRaw["javaDir"] = newJavaDir
	profilesRaw[profileID] = profileRaw
	root["profiles"] = profilesRaw

	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(profilePath, updated, 0644)
}

func patchAllProfilesInFile(profilePath, newJavaDir string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	profilesRaw, ok := root["profiles"].(map[string]interface{})
	if !ok {
		return nil
	}

	bakPath := profilePath + ".lunaris.bak"
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		_ = os.WriteFile(bakPath, data, 0644)
	}

	for id, val := range profilesRaw {
		if pMap, ok := val.(map[string]interface{}); ok {
			pMap["javaDir"] = newJavaDir
			profilesRaw[id] = pMap
		}
	}
	root["profiles"] = profilesRaw

	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(profilePath, updated, 0644)
}

func unpatchProfileJavaDir(profilePath, profileID string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	profilesRaw, ok := root["profiles"].(map[string]interface{})
	if !ok {
		return nil
	}

	profileRaw, ok := profilesRaw[profileID].(map[string]interface{})
	if !ok {
		return nil
	}

	delete(profileRaw, "javaDir")
	profilesRaw[profileID] = profileRaw
	root["profiles"] = profilesRaw

	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(profilePath, updated, 0644)
}

func unpatchAllProfilesInFile(profilePath string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	profilesRaw, ok := root["profiles"].(map[string]interface{})
	if !ok {
		return nil
	}

	for id, val := range profilesRaw {
		if pMap, ok := val.(map[string]interface{}); ok {
			delete(pMap, "javaDir")
			profilesRaw[id] = pMap
		}
	}
	root["profiles"] = profilesRaw

	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(profilePath, updated, 0644)
}
