package modtracker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
)

const (
	// StateFileName is the hidden state file stored inside each instance directory.
	StateFileName = ".lunaris_state.json"

	// CurrentStateVersion is the format version for the state file.
	CurrentStateVersion = 1
)

var ErrNoState = errors.New("no baseline state file found (first run)")

// StateFilePath returns the absolute or relative path to the instance state file.
func StateFilePath(instanceDir string) string {
	return filepath.Join(instanceDir, StateFileName)
}

// HasBaseline checks whether a baseline state file already exists for the instance.
func HasBaseline(instanceDir string) bool {
	p := StateFilePath(instanceDir)
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// LoadState loads the baseline and user rules from .lunaris_state.json.
func LoadState(instanceDir string) (*State, error) {
	p := StateFilePath(instanceDir)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoState
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.BaselineFiles == nil {
		state.BaselineFiles = make(map[string]FileState)
	}
	if state.UserRules.KeepAdded == nil {
		state.UserRules.KeepAdded = make(map[string]bool)
	}
	if state.UserRules.KeepRemoved == nil {
		state.UserRules.KeepRemoved = make(map[string]bool)
	}
	if state.UserRules.KeepModified == nil {
		state.UserRules.KeepModified = make(map[string]string)
	}

	return &state, nil
}

// SaveState writes the state snapshot to .lunaris_state.json.
func SaveState(instanceDir string, state *State) error {
	p := StateFilePath(instanceDir)
	if state.UserRules.KeepAdded == nil {
		state.UserRules.KeepAdded = make(map[string]bool)
	}
	if state.UserRules.KeepRemoved == nil {
		state.UserRules.KeepRemoved = make(map[string]bool)
	}
	if state.UserRules.KeepModified == nil {
		state.UserRules.KeepModified = make(map[string]string)
	}
	if state.BaselineFiles == nil {
		state.BaselineFiles = make(map[string]FileState)
	}
	state.Version = CurrentStateVersion
	state.LastSyncTime = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	return os.WriteFile(p, data, 0644)
}

// CreateInitialBaseline creates the initial baseline snapshot after the first sync.
func CreateInitialBaseline(instanceDir string, files map[string]manifest.FileEntry) (*State, error) {
	state := &State{
		Version:       CurrentStateVersion,
		LastSyncTime:  time.Now().UTC().Format(time.RFC3339),
		BaselineFiles: make(map[string]FileState, len(files)),
		UserRules:     NewUserRules(),
	}

	for p, f := range files {
		normPath := manifest.NormalizePath(p)
		state.BaselineFiles[normPath] = FileState{
			SHA256: f.SHA256,
			Size:   f.Size,
		}
	}

	if err := SaveState(instanceDir, state); err != nil {
		return nil, err
	}
	return state, nil
}

// DetectChanges compares the current local filesystem against the baseline and remote manifest,
// filtering out any items that have already been approved in UserRules.
func DetectChanges(
	instanceDir string,
	state *State,
	remoteMap map[string]manifest.FileEntry,
	syncDirs []string,
	ignoreFiles []string,
) ([]ModChange, error) {
	// If no state exists, this is the first run; no changes to prompt for.
	if state == nil {
		return nil, nil
	}

	// Scan local filesystem
	localManifest, err := manifest.ScanDirectory(instanceDir, syncDirs, ignoreFiles)
	if err != nil {
		return nil, err
	}
	localMap := localManifest.FileMap()

	var changes []ModChange

	// 1. Detect ADDED and MODIFIED files (present locally)
	for localPath, localEntry := range localMap {
		normPath := manifest.NormalizePath(localPath)

		// Exclude internal files or ignored patterns
		if manifest.ShouldIgnore(normPath, ignoreFiles) {
			continue
		}

		baselineEntry, inBaseline := state.BaselineFiles[normPath]
		remoteEntry, inRemote := remoteMap[normPath]

		if !inBaseline && !inRemote {
			// File is local, but was never in the baseline and is not in the server pack -> ADDED
			if !state.UserRules.IsKeepAdded(normPath) {
				changes = append(changes, ModChange{
					Path:        normPath,
					Type:        ChangeAdded,
					CurrentHash: localEntry.SHA256,
					Size:        localEntry.Size,
					Category:    detectCategory(normPath),
				})
			}
		} else {
			// File is in baseline or remote: check if tampered/modified locally
			expectedHash := baselineEntry.SHA256
			if expectedHash == "" && inRemote {
				expectedHash = remoteEntry.SHA256
			}

			if expectedHash != "" && !strings.EqualFold(localEntry.SHA256, expectedHash) {
				// Local hash does not match expected hash -> MODIFIED
				if !state.UserRules.IsKeepModified(normPath, localEntry.SHA256) {
					changes = append(changes, ModChange{
						Path:        normPath,
						Type:        ChangeModified,
						CurrentHash: localEntry.SHA256,
						ServerHash:  expectedHash,
						Size:        localEntry.Size,
						Category:    detectCategory(normPath),
					})
				}
			}
		}
	}

	// 2. Detect REMOVED files (were in baseline AND remote, but now missing locally)
	for path, baseEntry := range state.BaselineFiles {
		normPath := manifest.NormalizePath(path)
		if manifest.ShouldIgnore(normPath, ignoreFiles) {
			continue
		}

		_, inRemote := remoteMap[normPath]
		_, inLocal := localMap[normPath]

		// Only flag as removed if it was a server pack file that the user deleted
		if inRemote && !inLocal {
			if !state.UserRules.IsKeepRemoved(normPath) {
				changes = append(changes, ModChange{
					Path:       normPath,
					Type:       ChangeRemoved,
					ServerHash: baseEntry.SHA256,
					Size:       baseEntry.Size,
					Category:   detectCategory(normPath),
				})
			}
		}
	}

	// Sort changes alphabetically by path for consistent ordering
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	return changes, nil
}

// ApplyDecisions updates the UserRules and disk files based on the decisions made by the user.
func ApplyDecisions(
	instanceDir string,
	state *State,
	decisions map[string]UserDecision,
	changes []ModChange,
) error {
	if state == nil {
		return errors.New("state cannot be nil")
	}

	for _, ch := range changes {
		normPath := manifest.NormalizePath(ch.Path)
		decision, ok := decisions[normPath]
		if !ok {
			// Default to keep if unspecified
			decision = DecisionKeep
		}

		switch ch.Type {
		case ChangeAdded:
			if decision == DecisionKeep {
				// User wants to keep custom mod on top of the pack:
				state.UserRules.KeepAdded[normPath] = true
			} else {
				// User chose to discard/remove custom mod:
				fullPath := filepath.Join(instanceDir, filepath.FromSlash(normPath))
				_ = os.Remove(fullPath)
				delete(state.UserRules.KeepAdded, normPath)
			}

		case ChangeRemoved:
			if decision == DecisionKeep {
				// User wants to keep this pack mod removed:
				state.UserRules.KeepRemoved[normPath] = true
			} else {
				// User chose to restore mod from server:
				delete(state.UserRules.KeepRemoved, normPath)
			}

		case ChangeModified:
			if decision == DecisionKeep {
				// User wants to keep their modified local version:
				state.UserRules.KeepModified[normPath] = ch.CurrentHash
			} else {
				// User chose to revert file to server version:
				delete(state.UserRules.KeepModified, normPath)
			}
		}
	}

	return SaveState(instanceDir, state)
}

// UpdateBaseline refreshes the baseline snapshot with the current local files after a sync.
func UpdateBaseline(instanceDir string, state *State, localFiles map[string]manifest.FileEntry) error {
	if state == nil {
		state = &State{
			Version:       CurrentStateVersion,
			UserRules:     NewUserRules(),
			BaselineFiles: make(map[string]FileState),
		}
	}

	state.BaselineFiles = make(map[string]FileState, len(localFiles))
	for p, f := range localFiles {
		normPath := manifest.NormalizePath(p)
		state.BaselineFiles[normPath] = FileState{
			SHA256: f.SHA256,
			Size:   f.Size,
		}
	}

	return SaveState(instanceDir, state)
}

// ResetToPack clears all user rules and overrides for the instance,
// allowing the next sync to cleanly restore 100% server match.
func ResetToPack(instanceDir string) error {
	state, err := LoadState(instanceDir)
	if err != nil && err != ErrNoState {
		return err
	}

	if state != nil {
		state.UserRules = NewUserRules()
		return SaveState(instanceDir, state)
	}

	// Remove state file if present
	_ = os.Remove(StateFilePath(instanceDir))
	return nil
}

func detectCategory(path string) string {
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "mods/") || strings.HasSuffix(lower, ".jar") {
		return "mod"
	}
	if strings.HasPrefix(lower, "config/") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml") || strings.HasSuffix(lower, ".properties") {
		return "config"
	}
	if strings.HasPrefix(lower, "global_packs/") || strings.HasSuffix(lower, ".zip") {
		return "pack"
	}
	return "file"
}
