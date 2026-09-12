package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/injector"
)

// InstanceInfo represents a detected Minecraft instance from CurseForge, Vanilla, Prism, etc.
type InstanceInfo struct {
	Name            string `json:"name"`
	Launcher        string `json:"launcher"` // "CurseForge", "Vanilla", "Prism", "Modrinth", "Custom"
	Path            string `json:"path"`     // Path to the instance directory (containing mods, configs)
	ProfileFile     string `json:"profile_file"`
	ProfileID       string `json:"profile_id"`
	CurrentJava     string `json:"current_java"`
	IsInjected      bool   `json:"is_injected"`
	ConfiguredServer string `json:"configured_server,omitempty"`
}

// DetectInstances scans known paths for installed Minecraft modpacks and instances.
func DetectInstances() []InstanceInfo {
	var instances []InstanceInfo
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return instances
	}

	// 1. CurseForge Instances
	curseForgeDirs := getCurseForgeDirectories(homeDir)
	for _, cfDir := range curseForgeDirs {
		entries, err := os.ReadDir(cfDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					instPath := filepath.Join(cfDir, entry.Name())
					info := inspectCurseForgeInstance(instPath, entry.Name())
					if info != nil {
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
		instances = append(instances, vInstances...)
	}

	// 3. Prism Launcher instances
	prismDirs := getPrismDirectories(homeDir)
	for _, pDir := range prismDirs {
		entries, err := os.ReadDir(pDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					instPath := filepath.Join(pDir, entry.Name())
					info := inspectPrismInstance(instPath, entry.Name())
					if info != nil {
						instances = append(instances, *info)
					}
				}
			}
		}
	}

	return instances
}

func getCurseForgeDirectories(homeDir string) []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		dirs = append(dirs,
			filepath.Join(homeDir, "curseforge", "minecraft", "Instances"),
			filepath.Join(os.Getenv("USERPROFILE"), "curseforge", "minecraft", "Instances"),
			`C:\curseforge\minecraft\Instances`,
		)
	} else if runtime.GOOS == "darwin" {
		dirs = append(dirs,
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Instances"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Instances"),
		)
	} else {
		// Linux
		dirs = append(dirs,
			filepath.Join(homeDir, "curseforge", "minecraft", "Instances"),
			filepath.Join(homeDir, ".var", "app", "com.curseforge.CurseForge", "data", "curseforge", "minecraft", "Instances"),
		)
	}
	return dirs
}

func inspectCurseForgeInstance(instPath, fallbackName string) *InstanceInfo {
	// CurseForge instances usually have minecraftinstance.json or mods folder
	mcInstJSON := filepath.Join(instPath, "minecraftinstance.json")
	name := fallbackName

	if data, err := os.ReadFile(mcInstJSON); err == nil {
		var meta struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &meta) == nil && meta.Name != "" {
			name = meta.Name
		}
	} else {
		// Check if it has a mods directory
		if _, err := os.Stat(filepath.Join(instPath, "mods")); err != nil {
			return nil
		}
	}

	info := &InstanceInfo{
		Name:     name,
		Launcher: "CurseForge",
		Path:     instPath,
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
			Name     string `json:"name"`
			GameDir  string `json:"gameDir"`
			JavaDir  string `json:"javaDir"`
			Type     string `json:"type"`
		} `json:"profiles"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return list
	}

	for id, p := range root.Profiles {
		// Only consider profiles that have a custom gameDir or modded type
		if p.Name == "" {
			p.Name = id
		}
		gDir := p.GameDir
		if gDir == "" {
			gDir = filepath.Dir(profilePath)
		}

		info := InstanceInfo{
			Name:        p.Name,
			Launcher:    "Vanilla Launcher",
			Path:        gDir,
			ProfileFile: profilePath,
			ProfileID:   id,
			CurrentJava: p.JavaDir,
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
	// Check .minecraft directory inside instance
	gamePath := filepath.Join(instPath, ".minecraft")
	if _, err := os.Stat(gamePath); err != nil {
		gamePath = instPath
	}

	info := &InstanceInfo{
		Name:     name,
		Launcher: "Prism",
		Path:     gamePath,
	}

	cfgPath := filepath.Join(gamePath, config.ConfigFileName)
	if cfg, err := config.Load(cfgPath); err == nil {
		info.IsInjected = true
		info.ConfiguredServer = cfg.ServerURL
	}

	return info
}

// InstallConfig contains options for setting up Lunaris on an instance.
type InstallConfig struct {
	InstanceDir   string // Directory of the instance (where mods/ folder is)
	ServerURL     string // Sync server URL
	RealJavaPath  string // Path to real Java (auto-detected if empty)
	LunarisBinary string // Path to lunaris binary (auto-detected if empty)
	ProfileFile   string // Optional launcher_profiles.json to patch
	ProfileID     string // Optional profile ID to patch
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
	cfg.RealJavaPath = realJava
	cfg.SyncDirs = []string{"mods"}
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
		// Check if instance folder itself has a launcher_profiles.json (common in CurseForge workDirs)
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

	// Backup original file once
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
