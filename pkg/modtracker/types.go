package modtracker

// ChangeType represents the classification of a change detected in a mod or config.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"    // Custom mod/config added by user
	ChangeRemoved  ChangeType = "removed"  // Server pack mod removed by user
	ChangeModified ChangeType = "modified" // Existing mod/config tampered with or modified locally
)

// UserDecision represents how the user wants to treat a detected change.
type UserDecision string

const (
	DecisionKeep   UserDecision = "keep"   // Keep custom mod / keep removed / keep local edits
	DecisionRevert UserDecision = "revert" // Revert to server pack (delete custom / download missing / overwrite)
)

// ModChange details a single detected difference between the baseline and current filesystem.
type ModChange struct {
	Path        string     `json:"path"`         // Normalized relative path (e.g. "mods/custom-mod.jar")
	Type        ChangeType `json:"type"`         // "added", "removed", or "modified"
	CurrentHash string     `json:"current_hash"` // Local SHA-256 (empty if removed)
	ServerHash  string     `json:"server_hash"`  // Baseline/server SHA-256 (empty if added)
	Size        int64      `json:"size"`         // File size in bytes
	Category    string     `json:"category"`     // "mod", "config", or "pack"
}

// UserRules holds user decisions that persist across launches.
type UserRules struct {
	// KeepAdded contains relative paths of custom user-added files that should not be deleted.
	KeepAdded map[string]bool `json:"keep_added"`

	// KeepRemoved contains relative paths of server pack files that the user intentionally removed.
	KeepRemoved map[string]bool `json:"keep_removed"`

	// KeepModified maps relative paths to approved local SHA-256 hashes that should not be overwritten.
	KeepModified map[string]string `json:"keep_modified"`
}

// NewUserRules initializes an empty UserRules instance with instantiated maps.
func NewUserRules() UserRules {
	return UserRules{
		KeepAdded:    make(map[string]bool),
		KeepRemoved:  make(map[string]bool),
		KeepModified: make(map[string]string),
	}
}

// IsKeepAdded checks if a path is marked to be kept added.
func (r *UserRules) IsKeepAdded(path string) bool {
	if r == nil || r.KeepAdded == nil {
		return false
	}
	return r.KeepAdded[path]
}

// IsKeepRemoved checks if a path is marked to be kept removed.
func (r *UserRules) IsKeepRemoved(path string) bool {
	if r == nil || r.KeepRemoved == nil {
		return false
	}
	return r.KeepRemoved[path]
}

// IsKeepModified checks if a path is approved to be kept in its modified state.
func (r *UserRules) IsKeepModified(path, currentHash string) bool {
	if r == nil || r.KeepModified == nil {
		return false
	}
	approvedHash, exists := r.KeepModified[path]
	if !exists {
		return false
	}
	return approvedHash == "" || approvedHash == currentHash
}

// FileState stores the hash and size of a tracked file in the baseline.
type FileState struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// State represents the snapshot of the modpack saved in .lunaris_state.json.
type State struct {
	Version       int                  `json:"version"`
	LastSyncTime  string               `json:"last_sync_time"`
	BaselineFiles map[string]FileState `json:"baseline_files"`
	UserRules     UserRules            `json:"user_rules"`
}
