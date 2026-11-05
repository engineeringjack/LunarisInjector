package server

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ModSide specifies where a mod should run.
type ModSide string

const (
	SideBoth       ModSide = "both"
	SideClientOnly ModSide = "client_only"
	SideServerOnly ModSide = "server_only"
)

const (
	clientOnlyFileName = "client-only-mods.txt"
	serverOnlyFileName = "server-only-mods.txt"
)

// ModInfo represents metadata about an installed mod file.
type ModInfo struct {
	Filename   string  `json:"filename"`    // Standardized base name (e.g. "appleskin.jar")
	ActualName string  `json:"actual_name"` // Actual filename on disk (e.g. "appleskin.jar.disabled")
	Side       ModSide `json:"side"`        // "both", "client_only", "server_only"
	Folder     string  `json:"folder"`      // Relative folder ("mods", "client-only", "server-only")
	Path       string  `json:"path"`        // Full relative path ("mods/appleskin.jar")
	Size       int64   `json:"size"`        // Size in bytes
	ModTime    int64   `json:"mod_time"`    // Last modified unix timestamp
	Disabled   bool    `json:"disabled"`    // Whether the file is disabled with .disabled extension
}

// ModManager manages mod files, classifications, and side assignment text files.
type ModManager struct {
	mu         sync.RWMutex
	rootDir    string
	modsDir    string
	clientDir  string
	serverDir  string
	clientTxt  string
	serverTxt  string
}

// NewModManager initializes a ModManager for the given root directory.
func NewModManager(rootDir string) (*ModManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	modsDir := filepath.Join(absRoot, "mods")
	clientDir := filepath.Join(absRoot, "client-only")
	serverDir := filepath.Join(absRoot, "server-only")

	_ = os.MkdirAll(modsDir, 0755)

	return &ModManager{
		rootDir:   absRoot,
		modsDir:   modsDir,
		clientDir: clientDir,
		serverDir: serverDir,
		clientTxt: filepath.Join(absRoot, clientOnlyFileName),
		serverTxt: filepath.Join(absRoot, serverOnlyFileName),
	}, nil
}

// CleanModName strips directory path and `.disabled` suffix to yield the canonical `.jar` name.
func CleanModName(name string) string {
	base := filepath.Base(name)
	if strings.HasSuffix(strings.ToLower(base), ".disabled") {
		base = base[:len(base)-len(".disabled")]
	}
	return base
}

// ReadListFile reads a text file containing one mod name per line (ignoring comments and empty lines).
func ReadListFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	var result []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		// Strip any hash or annotations if user accidentally pasted them (take first whitespace token or clean name)
		parts := strings.Fields(line)
		if len(parts) > 0 {
			name := CleanModName(parts[0])
			if name != "" && !seen[strings.ToLower(name)] {
				seen[strings.ToLower(name)] = true
				result = append(result, name)
			}
		}
	}
	return result, scanner.Err()
}

// WriteListFile writes a sorted list of mod names to a text file with a descriptive header.
func WriteListFile(path, header string, mods []string) error {
	seen := make(map[string]bool)
	var cleanList []string
	for _, m := range mods {
		cleaned := CleanModName(m)
		if cleaned != "" && !seen[strings.ToLower(cleaned)] {
			seen[strings.ToLower(cleaned)] = true
			cleanList = append(cleanList, cleaned)
		}
	}
	sort.Strings(cleanList)

	var sb strings.Builder
	if header != "" {
		sb.WriteString(header + "\n")
	}
	for _, m := range cleanList {
		sb.WriteString(m + "\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// GetClientOnlyList returns all mod names listed in client-only-mods.txt (or alias client-only.txt).
func (m *ModManager) GetClientOnlyList() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list, err := ReadListFile(m.clientTxt)
	if (list == nil || len(list) == 0) && err == nil {
		altPath := filepath.Join(m.rootDir, "client-only.txt")
		if altList, altErr := ReadListFile(altPath); altErr == nil && len(altList) > 0 {
			return altList, nil
		}
	}
	return list, err
}

// GetServerOnlyList returns all mod names listed in server-only-mods.txt (or alias server-only.txt).
func (m *ModManager) GetServerOnlyList() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list, err := ReadListFile(m.serverTxt)
	if (list == nil || len(list) == 0) && err == nil {
		altPath := filepath.Join(m.rootDir, "server-only.txt")
		if altList, altErr := ReadListFile(altPath); altErr == nil && len(altList) > 0 {
			return altList, nil
		}
	}
	return list, err
}

// SetClientOnlyList replaces the contents of client-only-mods.txt.
func (m *ModManager) SetClientOnlyList(mods []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	header := "# Lunaris - Client-Only Mods (one mod filename per line, e.g. appleskin.jar)\n# Any mod listed here will stay in the client-only folder on the server."
	return WriteListFile(m.clientTxt, header, mods)
}

// SetServerOnlyList replaces the contents of server-only-mods.txt.
func (m *ModManager) SetServerOnlyList(mods []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	header := "# Lunaris - Server-Only Mods (one mod filename per line, e.g. chunky.jar)\n# Any mod listed here will not be sent to clients in manifest.json."
	return WriteListFile(m.serverTxt, header, mods)
}

// ClassifyMod returns the side of a mod file based on text files or location.
func (m *ModManager) ClassifyMod(filename string) ModSide {
	clean := CleanModName(filename)
	lower := strings.ToLower(clean)

	clientMods, _ := m.GetClientOnlyList()
	for _, c := range clientMods {
		if strings.ToLower(CleanModName(c)) == lower {
			return SideClientOnly
		}
	}

	serverMods, _ := m.GetServerOnlyList()
	for _, s := range serverMods {
		if strings.ToLower(CleanModName(s)) == lower {
			return SideServerOnly
		}
	}

	return SideBoth
}

// ListAllMods scans mods/, client-only/, and server-only/ and returns a structured list.
func (m *ModManager) ListAllMods() ([]ModInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clientList, _ := ReadListFile(m.clientTxt)
	clientMap := make(map[string]bool)
	for _, c := range clientList {
		clientMap[strings.ToLower(CleanModName(c))] = true
	}

	serverList, _ := ReadListFile(m.serverTxt)
	serverMap := make(map[string]bool)
	for _, s := range serverList {
		serverMap[strings.ToLower(CleanModName(s))] = true
	}

	resultMap := make(map[string]ModInfo)

	scanDir := func(folderName string, dirPath string) {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(name)
			if !strings.HasSuffix(lower, ".jar") && !strings.HasSuffix(lower, ".jar.disabled") && !strings.HasSuffix(lower, ".zip") {
				continue
			}

			clean := CleanModName(name)
			cleanLower := strings.ToLower(clean)
			disabled := strings.HasSuffix(lower, ".disabled")

			info, err := e.Info()
			var size int64
			var modTime int64
			if err == nil {
				size = info.Size()
				modTime = info.ModTime().Unix()
			}

			side := SideBoth
			if folderName == "client-only" || clientMap[cleanLower] || disabled {
				side = SideClientOnly
			} else if folderName == "server-only" || serverMap[cleanLower] {
				side = SideServerOnly
			}

			relPath := filepath.ToSlash(filepath.Join(folderName, name))

			resultMap[cleanLower] = ModInfo{
				Filename:   clean,
				ActualName: name,
				Side:       side,
				Folder:     folderName,
				Path:       relPath,
				Size:       size,
				ModTime:    modTime,
				Disabled:   disabled,
			}
		}
	}

	scanDir("mods", m.modsDir)
	scanDir("client-only", m.clientDir)
	scanDir("server-only", m.serverDir)

	var list []ModInfo
	for _, info := range resultMap {
		list = append(list, info)
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Filename) < strings.ToLower(list[j].Filename)
	})

	return list, nil
}

// AddOrUpdateMod saves a mod file, placing it in the correct directory and updating text lists.
func (m *ModManager) AddOrUpdateMod(filename string, content io.Reader, side ModSide) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := CleanModName(filename)
	if clean == "" {
		return fmt.Errorf("invalid mod filename")
	}

	// 1. Remove existing instances of this mod from any of the three folders
	m.removeFileEverywhereLocked(clean)

	// 2. Determine target path based on side
	var targetPath string
	switch side {
	case SideClientOnly:
		_ = os.MkdirAll(m.clientDir, 0755)
		targetPath = filepath.Join(m.clientDir, clean)
		m.updateListFilesLocked(clean, SideClientOnly)
	case SideServerOnly:
		_ = os.MkdirAll(m.serverDir, 0755)
		// For dedicated server to execute it, it can live in server-only or mods;
		// placing it in mods/ with listing in server-only-mods.txt allows Minecraft server
		// to load it while excluding it from client sync manifest.
		targetPath = filepath.Join(m.modsDir, clean)
		m.updateListFilesLocked(clean, SideServerOnly)
	default: // SideBoth
		_ = os.MkdirAll(m.modsDir, 0755)
		targetPath = filepath.Join(m.modsDir, clean)
		m.updateListFilesLocked(clean, SideBoth)
	}

	// 3. Write file
	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, content); err != nil {
		return fmt.Errorf("failed to stream content: %w", err)
	}

	return nil
}

// SetModSide moves an existing mod file to the appropriate folder and updates the text files.
func (m *ModManager) SetModSide(filename string, targetSide ModSide) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := CleanModName(filename)
	existingPath, err := m.findFileLocked(clean)
	if err != nil {
		return err
	}

	var newPath string
	switch targetSide {
	case SideClientOnly:
		_ = os.MkdirAll(m.clientDir, 0755)
		newPath = filepath.Join(m.clientDir, clean)
	case SideServerOnly:
		_ = os.MkdirAll(m.modsDir, 0755)
		newPath = filepath.Join(m.modsDir, clean)
	default: // SideBoth
		_ = os.MkdirAll(m.modsDir, 0755)
		newPath = filepath.Join(m.modsDir, clean)
	}

	if existingPath != newPath {
		// If moving, copy/rename
		if err := os.Rename(existingPath, newPath); err != nil {
			// Cross-device fallback
			if cpErr := copyFile(existingPath, newPath); cpErr != nil {
				return cpErr
			}
			_ = os.Remove(existingPath)
		}
	}

	m.updateListFilesLocked(clean, targetSide)
	return nil
}

// ToggleDisabled toggles the .disabled extension on a mod file.
func (m *ModManager) ToggleDisabled(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := CleanModName(filename)
	existingPath, err := m.findFileLocked(clean)
	if err != nil {
		return err
	}

	dir := filepath.Dir(existingPath)
	base := filepath.Base(existingPath)

	var newPath string
	if strings.HasSuffix(strings.ToLower(base), ".disabled") {
		newPath = filepath.Join(dir, base[:len(base)-len(".disabled")])
	} else {
		newPath = filepath.Join(dir, base+".disabled")
	}

	return os.Rename(existingPath, newPath)
}

// DeleteMod removes a mod from disk and removes its entry from both text files.
func (m *ModManager) DeleteMod(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clean := CleanModName(filename)
	m.removeFileEverywhereLocked(clean)
	m.updateListFilesLocked(clean, "")
	return nil
}

func (m *ModManager) findFileLocked(clean string) (string, error) {
	candidates := []string{
		filepath.Join(m.modsDir, clean),
		filepath.Join(m.modsDir, clean+".disabled"),
		filepath.Join(m.clientDir, clean),
		filepath.Join(m.clientDir, clean+".disabled"),
		filepath.Join(m.serverDir, clean),
		filepath.Join(m.serverDir, clean+".disabled"),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("mod file not found: %s", clean)
}

func (m *ModManager) removeFileEverywhereLocked(clean string) {
	candidates := []string{
		filepath.Join(m.modsDir, clean),
		filepath.Join(m.modsDir, clean+".disabled"),
		filepath.Join(m.clientDir, clean),
		filepath.Join(m.clientDir, clean+".disabled"),
		filepath.Join(m.serverDir, clean),
		filepath.Join(m.serverDir, clean+".disabled"),
	}

	for _, c := range candidates {
		_ = os.Remove(c)
	}
}

func (m *ModManager) updateListFilesLocked(modName string, side ModSide) {
	clean := CleanModName(modName)
	lower := strings.ToLower(clean)

	clientMods, _ := ReadListFile(m.clientTxt)
	serverMods, _ := ReadListFile(m.serverTxt)

	var newClient []string
	for _, c := range clientMods {
		if strings.ToLower(CleanModName(c)) != lower {
			newClient = append(newClient, CleanModName(c))
		}
	}

	var newServer []string
	for _, s := range serverMods {
		if strings.ToLower(CleanModName(s)) != lower {
			newServer = append(newServer, CleanModName(s))
		}
	}

	if side == SideClientOnly {
		newClient = append(newClient, clean)
	} else if side == SideServerOnly {
		newServer = append(newServer, clean)
	}

	headerClient := "# Lunaris - Client-Only Mods (one mod filename per line, e.g. appleskin.jar)\n# Any mod listed here will stay in the client-only folder on the server."
	_ = WriteListFile(m.clientTxt, headerClient, newClient)

	headerServer := "# Lunaris - Server-Only Mods (one mod filename per line, e.g. chunky.jar)\n# Any mod listed here will not be sent to clients in manifest.json."
	_ = WriteListFile(m.serverTxt, headerServer, newServer)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
