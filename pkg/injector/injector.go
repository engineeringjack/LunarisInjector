package injector

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ExtractGameDir inspects command-line arguments to find Minecraft's gameDir.
func ExtractGameDir(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Format: --gameDir <path>
		if arg == "--gameDir" && i+1 < len(args) {
			return filepath.Clean(args[i+1])
		}

		// Format: --gameDir=<path>
		if strings.HasPrefix(arg, "--gameDir=") {
			return filepath.Clean(strings.TrimPrefix(arg, "--gameDir="))
		}

		// Format: -Dminecraft.applet.TargetDirectory=<path>
		if strings.HasPrefix(arg, "-Dminecraft.applet.TargetDirectory=") {
			return filepath.Clean(strings.TrimPrefix(arg, "-Dminecraft.applet.TargetDirectory="))
		}
	}

	// Fallback: check current working directory
	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "mods")); err == nil {
			return cwd
		}
	}

	return ""
}

// ExtractGameVersion attempts to determine the Minecraft version from launch args or instance files.
func ExtractGameVersion(args []string, gameDir string) string {
	// 1. Check CLI arguments for --fml.mcVersion
	for i := 0; i < len(args); i++ {
		if args[i] == "--fml.mcVersion" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if strings.HasPrefix(args[i], "--fml.mcVersion=") {
			return strings.TrimPrefix(args[i], "--fml.mcVersion=")
		}
	}

	// 2. Check minecraftinstance.json in gameDir
	if gameDir != "" {
		mcJSON := filepath.Join(gameDir, "minecraftinstance.json")
		if data, err := os.ReadFile(mcJSON); err == nil {
			var meta struct {
				GameVersion string `json:"gameVersion"`
				BaseModLoader struct {
					MinecraftVersion string `json:"minecraftVersion"`
				} `json:"baseModLoader"`
			}
			if json.Unmarshal(data, &meta) == nil {
				if meta.GameVersion != "" {
					return meta.GameVersion
				}
				if meta.BaseModLoader.MinecraftVersion != "" {
					return meta.BaseModLoader.MinecraftVersion
				}
			}
		}
	}

	// 3. Check CLI arguments for --version
	for i := 0; i < len(args); i++ {
		if args[i] == "--version" && i+1 < len(args) {
			v := args[i+1]
			// e.g. "1.20.1", "1.20.1-forge-47.4.10", "forge-47.4.10"
			if strings.Contains(v, "1.20.1") {
				return "1.20.1"
			}
		}
	}

	return ""
}

// GetSiblingRealJava checks whether a sibling real Java binary exists next to the current executable.
// This indicates the current binary was installed as a Java runtime wrapper.
func GetSiblingRealJava() string {
	selfPath, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(selfPath)
	base := filepath.Base(selfPath)

	var candidates []string
	if runtime.GOOS == "windows" {
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		lower := strings.ToLower(base)
		if strings.HasPrefix(lower, "javaw") {
			candidates = append(candidates,
				filepath.Join(dir, "javaw.real.exe"),
				filepath.Join(dir, noExt+".real.exe"),
				filepath.Join(dir, "java.real.exe"),
				filepath.Join(dir, "javaw.real"),
			)
		} else {
			candidates = append(candidates,
				filepath.Join(dir, "java.real.exe"),
				filepath.Join(dir, noExt+".real.exe"),
				filepath.Join(dir, "javaw.real.exe"),
				filepath.Join(dir, "java.real"),
			)
		}
	} else {
		candidates = append(candidates,
			filepath.Join(dir, base+".real"),
			filepath.Join(dir, "java.real"),
		)
	}

	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
	}
	return ""
}

// ResolveRealBinary checks if a binary path has a corresponding '.real' sibling.
// If so (e.g. java -> java.real), it returns the real binary path.
func ResolveRealBinary(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	dir := filepath.Dir(clean)
	base := filepath.Base(clean)

	if runtime.GOOS == "windows" {
		noExt := strings.TrimSuffix(base, filepath.Ext(base))
		cand := filepath.Join(dir, noExt+".real.exe")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
		cand = filepath.Join(dir, noExt+".real")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
	} else {
		cand := filepath.Join(dir, base+".real")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand
		}
	}

	return clean
}

// HasSiblingRealJava checks whether a candidate path has a sibling '.real' file (meaning candidate is a wrapper).
func HasSiblingRealJava(candidatePath string) bool {
	realPath := ResolveRealBinary(candidatePath)
	return realPath != filepath.Clean(candidatePath)
}

// IsSelfExecutable checks if the given path points to the currently running LunarisInjector binary,
// or if the path is a wrapper that has a sibling real Java binary.
func IsSelfExecutable(candidatePath string) bool {
	if HasSiblingRealJava(candidatePath) {
		return true
	}

	selfPath, err := os.Executable()
	if err != nil {
		return false
	}

	selfReal, err1 := filepath.EvalSymlinks(selfPath)
	candReal, err2 := filepath.EvalSymlinks(candidatePath)
	if err1 == nil && err2 == nil {
		return strings.EqualFold(selfReal, candReal)
	}

	return strings.EqualFold(filepath.Clean(selfPath), filepath.Clean(candidatePath))
}

// FindRealJava attempts to locate the real Java runtime on the system.
func FindRealJava(configuredPath string) (string, error) {
	// 0. Sibling real Java (when running as hooked wrapper)
	if sibling := GetSiblingRealJava(); sibling != "" {
		return sibling, nil
	}

	// 1. If configured in lunaris.json and valid
	if configuredPath != "" {
		resolved := ResolveRealBinary(configuredPath)
		if fi, err := os.Stat(resolved); err == nil && !fi.IsDir() {
			if !IsSelfExecutable(resolved) {
				return resolved, nil
			}
		}
	}

	// 2. Search common Minecraft Launcher runtime directories
	homeDir, _ := os.UserHomeDir()
	var searchPatterns []string

	if runtime.GOOS == "windows" {
		searchPatterns = []string{
			filepath.Join(homeDir, "AppData", "Local", "Packages", "*", "LocalCache", "Local", "runtime", "java-runtime-gamma", "*", "bin", "javaw.exe"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "javaw.exe"),
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "javaw.exe"),
			`C:\curseforge\minecraft\Install\java\java-runtime-gamma\bin\javaw.exe`,
			filepath.Join(homeDir, "AppData", "Local", "Packages", "*", "LocalCache", "Local", "runtime", "*", "*", "bin", "javaw.exe"),
			filepath.Join(homeDir, "AppData", "Local", "Packages", "*", "LocalCache", "Local", "runtime", "*", "*", "bin", "java.exe"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "*", "bin", "javaw.exe"),
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "*", "bin", "javaw.exe"),
			`C:\Program Files (x86)\Minecraft Launcher\runtime\*\*\bin\javaw.exe`,
			`C:\Program Files (x86)\Minecraft Launcher\runtime\*\*\bin\java.exe`,
			`C:\Program Files\Java\*\bin\javaw.exe`,
			`C:\Program Files\Java\*\bin\java.exe`,
			`C:\Program Files\Eclipse Adoptium\*\bin\javaw.exe`,
			`C:\Program Files\BellSoft\*\bin\javaw.exe`,
		}
	} else if runtime.GOOS == "darwin" {
		searchPatterns = []string{
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "java"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "java"),
			filepath.Join(homeDir, "Library", "Application Support", "minecraft", "runtime", "*", "*", "bin", "java"),
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "*", "bin", "java"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "*", "bin", "java"),
			"/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java",
		}
	} else {
		// Linux
		searchPatterns = []string{
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "java"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "java"),
			filepath.Join(homeDir, ".var", "app", "com.curseforge.CurseForge", "data", "curseforge", "minecraft", "Install", "java", "java-runtime-gamma", "bin", "java"),
			filepath.Join(homeDir, "Documents", "curseforge", "minecraft", "Install", "java", "*", "bin", "java"),
			filepath.Join(homeDir, "curseforge", "minecraft", "Install", "java", "*", "bin", "java"),
			filepath.Join(homeDir, ".var", "app", "com.curseforge.CurseForge", "data", "curseforge", "minecraft", "Install", "java", "*", "bin", "java"),
			filepath.Join(homeDir, ".minecraft", "runtime", "java-runtime-gamma", "*", "bin", "java"),
			filepath.Join(homeDir, ".minecraft", "runtime", "*", "*", "bin", "java"),
			filepath.Join(homeDir, ".minecraft", "runtime", "*", "*", "*", "bin", "java"),
			"/usr/lib/jvm/java-17-*/bin/java",
			"/usr/lib/jvm/*/bin/java",
			"/usr/bin/java",
		}
	}

	for _, pattern := range searchPatterns {
		matches, err := filepath.Glob(pattern)
		if err == nil {
			for _, match := range matches {
				resolved := ResolveRealBinary(match)
				if _, err := os.Stat(resolved); err == nil && !IsSelfExecutable(resolved) {
					return resolved, nil
				}
			}
		}
	}

	// 3. Check JAVA_HOME
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		binName := "java"
		if runtime.GOOS == "windows" {
			binName = "javaw.exe"
		}
		candidate := ResolveRealBinary(filepath.Join(javaHome, "bin", binName))
		if _, err := os.Stat(candidate); err == nil && !IsSelfExecutable(candidate) {
			return candidate, nil
		}
	}

	// 4. Check system PATH
	binaries := []string{"javaw", "java"}
	if runtime.GOOS == "windows" {
		binaries = []string{"javaw.exe", "java.exe"}
	}

	for _, bin := range binaries {
		if path, err := exec.LookPath(bin); err == nil {
			resolved := ResolveRealBinary(path)
			if !IsSelfExecutable(resolved) {
				return resolved, nil
			}
		}
	}

	return "", errors.New("unable to locate real Java runtime. Please specify 'real_java_path' in lunaris.json")
}

// RunJava launches the real Java runtime passing through all original arguments.
func RunJava(javaPath string, args []string) (int, error) {
	cmd := exec.Command(javaPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("failed to start Java process: %w", err)
	}

	err := cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}

	return 0, nil
}
