package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// AdminHandler handles all /api/admin/* endpoints.
type AdminHandler struct {
	authMgr    *AuthManager
	modMgr     *ModManager
	processMgr *ProcessManager
	server     *Server
}

// NewAdminHandler creates a new AdminHandler instance.
func NewAdminHandler(s *Server) *AdminHandler {
	return &AdminHandler{
		authMgr:    s.authMgr,
		modMgr:     s.modMgr,
		processMgr: s.processMgr,
		server:     s,
	}
}

// RegisterRoutes attaches the admin GUI and API endpoints to the router mux.
func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	// 1. Web GUI page
	mux.HandleFunc("/admin", h.handleAdminPage)
	mux.HandleFunc("/admin/", h.handleAdminPage)

	// 2. Auth APIs
	mux.HandleFunc("/api/admin/auth/status", h.handleAuthStatus)
	mux.HandleFunc("/api/admin/auth/setup", h.handleAuthSetup)
	mux.HandleFunc("/api/admin/auth/login", h.handleAuthLogin)
	mux.HandleFunc("/api/admin/auth/logout", h.handleAuthLogout)
	mux.HandleFunc("/api/admin/auth/change-credentials", h.requireAuth(h.handleChangeCredentials))

	// 3. Mod Management APIs
	mux.HandleFunc("/api/admin/mods", h.requireAuth(h.handleGetMods))
	mux.HandleFunc("/api/admin/mods/upload", h.requireAuth(h.handleUploadMods))
	mux.HandleFunc("/api/admin/mods/set-side", h.requireAuth(h.handleSetModSide))
	mux.HandleFunc("/api/admin/mods/toggle-disabled", h.requireAuth(h.handleToggleDisabled))
	mux.HandleFunc("/api/admin/mods/delete", h.requireAuth(h.handleDeleteMod))
	mux.HandleFunc("/api/admin/mods/text-files", h.requireAuth(h.handleTextFiles))

	// 4. Server Process APIs
	mux.HandleFunc("/api/admin/server/status", h.requireAuth(h.handleServerStatus))
	mux.HandleFunc("/api/admin/server/start", h.requireAuth(h.handleServerStart))
	mux.HandleFunc("/api/admin/server/stop", h.requireAuth(h.handleServerStop))
	mux.HandleFunc("/api/admin/server/restart", h.requireAuth(h.handleServerRestart))
	mux.HandleFunc("/api/admin/server/command", h.requireAuth(h.handleServerCommand))
	mux.HandleFunc("/api/admin/server/config", h.requireAuth(h.handleServerConfig))
	mux.HandleFunc("/api/admin/server/logs/console", h.requireAuth(h.handleConsoleLogs))
	mux.HandleFunc("/api/admin/server/logs/clear", h.requireAuth(h.handleClearLogs))
	mux.HandleFunc("/api/admin/server/logs/latest", h.requireAuth(h.handleLatestLog))

	// 5. Network & Diagnostics
	mux.HandleFunc("/api/admin/network", h.requireAuth(h.handleNetworkInfo))
	mux.HandleFunc("/api/admin/server/rescan", h.requireAuth(h.handleRescan))
}

func (h *AdminHandler) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(AdminHTML))
}

func (h *AdminHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Authentication required",
			})
			return
		}

		username, valid := h.authMgr.ValidateToken(token)
		if !valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Session expired or invalid",
			})
			return
		}

		// Inject verified username into header for subsequent handlers
		r.Header.Set("X-Verified-User", username)
		next(w, r)
	}
}

// -------------------------------------------------------------
// Auth Handlers
// -------------------------------------------------------------

func (h *AdminHandler) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	configured := h.authMgr.IsConfigured()

	token := ExtractToken(r)
	username, valid := h.authMgr.ValidateToken(token)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"configured":    configured,
		"authenticated": valid,
		"username":      username,
	})
}

func (h *AdminHandler) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.authMgr.IsConfigured() {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Admin credentials have already been configured. Please log in or use change-credentials.",
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON payload",
		})
		return
	}

	if err := h.authMgr.SetCredentials(req.Username, req.Password); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	token, err := h.authMgr.CreateSession(req.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Credentials set, but failed to create session",
		})
		return
	}

	SetSessionCookie(w, token)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"token":    token,
		"username": req.Username,
	})
}

func (h *AdminHandler) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !h.authMgr.IsConfigured() {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Admin credentials not configured yet. Setup is required.",
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	if !h.authMgr.Authenticate(req.Username, req.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Incorrect username or password",
		})
		return
	}

	token, err := h.authMgr.CreateSession(req.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to create session",
		})
		return
	}

	SetSessionCookie(w, token)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"token":    token,
		"username": req.Username,
	})
}

func (h *AdminHandler) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	token := ExtractToken(r)
	if token != "" {
		h.authMgr.RevokeToken(token)
	}
	ClearSessionCookie(w)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (h *AdminHandler) handleChangeCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewUsername     string `json:"new_username"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	currentUser := r.Header.Get("X-Verified-User")
	if !h.authMgr.Authenticate(currentUser, req.CurrentPassword) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Current password is incorrect",
		})
		return
	}

	newUsername := req.NewUsername
	if strings.TrimSpace(newUsername) == "" {
		newUsername = currentUser
	}

	if err := h.authMgr.SetCredentials(newUsername, req.NewPassword); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	token, _ := h.authMgr.CreateSession(newUsername)
	SetSessionCookie(w, token)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"username": newUsername,
		"token":    token,
	})
}

// -------------------------------------------------------------
// Mod Management Handlers
// -------------------------------------------------------------

func (h *AdminHandler) handleGetMods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	mods, err := h.modMgr.ListAllMods()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	clientList, _ := h.modMgr.GetClientOnlyList()
	serverList, _ := h.modMgr.GetServerOnlyList()

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"mods":        mods,
		"client_only": clientList,
		"server_only": serverList,
	})
}

func (h *AdminHandler) handleUploadMods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 500 MB max memory for upload buffer
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to parse upload: %v", err),
		})
		return
	}

	removeMissing := r.FormValue("remove_missing") == "true"
	sidesJSON := r.FormValue("sides")
	sidesMap := make(map[string]string)
	if sidesJSON != "" {
		_ = json.Unmarshal([]byte(sidesJSON), &sidesMap)
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No files uploaded",
		})
		return
	}

	// If remove_missing was chosen (when dropping a full mod folder):
	// Delete any current server mod that is not present in the dropped files!
	removedCount := 0
	if removeMissing {
		currentMods, _ := h.modMgr.ListAllMods()
		uploadedSet := make(map[string]bool)
		for _, fHeader := range files {
			uploadedSet[strings.ToLower(CleanModName(fHeader.Filename))] = true
		}

		for _, cur := range currentMods {
			if !uploadedSet[strings.ToLower(cur.Filename)] {
				if err := h.modMgr.DeleteMod(cur.Filename); err == nil {
					removedCount++
				}
			}
		}
	}

	addedCount := 0
	for _, fHeader := range files {
		f, err := fHeader.Open()
		if err != nil {
			continue
		}

		clean := CleanModName(fHeader.Filename)
		sideStr, ok := sidesMap[clean]
		if !ok {
			// Check if known in text files
			sideStr = string(h.modMgr.ClassifyMod(clean))
		}

		side := SideBoth
		if sideStr == string(SideClientOnly) {
			side = SideClientOnly
		} else if sideStr == string(SideServerOnly) {
			side = SideServerOnly
		}

		if err := h.modMgr.AddOrUpdateMod(clean, f, side); err == nil {
			addedCount++
		}
		_ = f.Close()
	}

	// Refresh manifest on the fly
	_, _ = h.server.RefreshManifest()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"added":   addedCount,
		"removed": removedCount,
	})
}

func (h *AdminHandler) handleSetModSide(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string  `json:"filename"`
		Side     ModSide `json:"side"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	if err := h.modMgr.SetModSide(req.Filename, req.Side); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_, _ = h.server.RefreshManifest()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleToggleDisabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	if err := h.modMgr.ToggleDisabled(req.Filename); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_, _ = h.server.RefreshManifest()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleDeleteMod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	if err := h.modMgr.DeleteMod(req.Filename); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_, _ = h.server.RefreshManifest()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleTextFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		clientList, _ := h.modMgr.GetClientOnlyList()
		serverList, _ := h.modMgr.GetServerOnlyList()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"client_only": clientList,
			"server_only": serverList,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ClientOnly []string `json:"client_only"`
			ServerOnly []string `json:"server_only"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
			return
		}

		_ = h.modMgr.SetClientOnlyList(req.ClientOnly)
		_ = h.modMgr.SetServerOnlyList(req.ServerOnly)
		_, _ = h.server.RefreshManifest()

		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// -------------------------------------------------------------
// Server Process Handlers
// -------------------------------------------------------------

func (h *AdminHandler) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status, pid, uptime, failedBoot, reason, startCmd := h.processMgr.GetStatus()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"status":        string(status),
		"pid":           pid,
		"uptime":        uptime,
		"failed_boot":   failedBoot,
		"failed_reason": reason,
		"start_command": startCmd,
	})
}

func (h *AdminHandler) handleServerStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := h.processMgr.Start(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Server starting"})
}

func (h *AdminHandler) handleServerStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := h.processMgr.Stop(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Server stopped"})
}

func (h *AdminHandler) handleServerRestart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Rescan files first
	_, _ = h.server.RefreshManifest()

	// 2. Restart Minecraft server process
	if err := h.processMgr.Restart(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Server restarting"})
}

func (h *AdminHandler) handleServerCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	if err := h.processMgr.SendCommand(req.Command); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleServerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		StartCommand string `json:"start_command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid JSON"})
		return
	}

	h.processMgr.SetStartCommand(req.StartCommand)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleConsoleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	limit := 300
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	lines := h.processMgr.GetConsoleLogs(limit)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"lines":   lines,
	})
}

func (h *AdminHandler) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	h.processMgr.ClearConsoleBuffer()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (h *AdminHandler) handleLatestLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	content, source, err := h.processMgr.GetLatestLog()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"content": content,
		"source":  source,
	})
}

// -------------------------------------------------------------
// Network & Rescan Handlers
// -------------------------------------------------------------

func (h *AdminHandler) handleNetworkInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var ips []string
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && ip.To4() != nil {
					ips = append(ips, ip.String())
				}
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"port":       h.server.opts.Port,
		"host":       h.server.opts.Host,
		"local_ips":  ips,
		"root_dir":   h.server.opts.RootDir,
		"port_open":  true,
	})
}

func (h *AdminHandler) handleRescan(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	m, err := h.server.RefreshManifest()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"file_count": len(m.Files),
	})
}
