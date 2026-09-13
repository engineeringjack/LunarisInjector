package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/config"
	"github.com/engineeringjack/LunarisInjector/pkg/installer"
	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
	"github.com/engineeringjack/LunarisInjector/pkg/syncer"
	"github.com/engineeringjack/LunarisInjector/pkg/updater"
)

// GUIOptions configures the graphical installer interface.
type GUIOptions struct {
	Port         int
	OpenBrowser  bool
	ServerURL    string
	InitialVR    bool
	Logger       func(format string, args ...interface{})
	ShutdownChan chan struct{}
}

// Server provides the embedded HTTP server powering the desktop GUI.
type Server struct {
	opts         GUIOptions
	listener     net.Listener
	httpServer   *http.Server
	mu           sync.Mutex
	hasConnected bool
	lastPing     time.Time
	shutdownOnce sync.Once
	shutdownChan chan struct{}
}

// New creates a new GUI Server instance.
func New(opts GUIOptions) *Server {
	if opts.ServerURL == "" {
		opts.ServerURL = config.DefaultServerURL
	}
	if opts.Logger == nil {
		opts.Logger = func(format string, args ...interface{}) {
			fmt.Printf(format+"\n", args...)
		}
	}
	shChan := opts.ShutdownChan
	if shChan == nil {
		shChan = make(chan struct{})
	}

	return &Server{
		opts:         opts,
		lastPing:     time.Now(),
		shutdownChan: shChan,
	}
}

// Handler returns the HTTP handler for the installer GUI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Static HTML/CSS/JS Single-Page App
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write([]byte(IndexHTML))
	})

	// 2. API: Detect Instances
	mux.HandleFunc("/api/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		instances := installer.DetectInstances()
		if instances == nil {
			instances = []installer.InstanceInfo{}
		}
		_ = json.NewEncoder(w).Encode(instances)
	})

	// 3. API: Status of sync server
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		srvURL := r.URL.Query().Get("server")
		if srvURL == "" {
			srvURL = s.opts.ServerURL
		}
		srvURL = strings.TrimRight(srvURL, "/")

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(srvURL + "/manifest.json")
		if err != nil || resp.StatusCode != http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"online":     false,
				"server_url": srvURL,
				"message":    "Server is offline or unreachable (offline launch supported)",
			})
			return
		}
		defer resp.Body.Close()

		var m manifest.Manifest
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"online":     true,
				"server_url": srvURL,
				"file_count": 0,
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"online":       true,
			"server_url":   srvURL,
			"file_count":   len(m.Files),
			"game_version": m.GameVersion,
		})
	})

	// 4. API: Execute Install
	mux.HandleFunc("/api/install", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req struct {
			InstancePath   string `json:"instance_path"`
			ServerURL      string `json:"server_url"`
			EnableVR       bool   `json:"enable_vr"`
			RealJavaPath   string `json:"real_java_path"`
			HookCurseForge *bool  `json:"hook_curseforge"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid request format",
			})
			return
		}

		if req.InstancePath == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Please select or specify a Minecraft instance directory",
			})
			return
		}

		serverURL := req.ServerURL
		if serverURL == "" {
			serverURL = s.opts.ServerURL
		}

		selfExe, _ := os.Executable()
		selfExe, _ = filepath.Abs(selfExe)

		err := installer.Install(installer.InstallConfig{
			InstanceDir:         req.InstancePath,
			ServerURL:           serverURL,
			RealJavaPath:        req.RealJavaPath,
			LunarisBinary:       selfExe,
			HookCurseForge:      req.HookCurseForge,
			EnableVR:            req.EnableVR,
			RequiredGameVersion: "1.20.1",
		})

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		// Find hooked runtimes for feedback
		var hookedRuntimes []string
		runtimes := installer.FindCurseForgeJavaRuntimeDirs(req.InstancePath)
		for _, rDir := range runtimes {
			if installer.IsCurseForgeJavaHooked(rDir) {
				hookedRuntimes = append(hookedRuntimes, rDir)
			}
		}

		// Calculate quick file sync preview
		cfg, _, _ := config.FindInstanceConfig(req.InstancePath)
		var syncPreview *syncer.Plan
		if cfg != nil {
			syn := syncer.New(syncer.SyncOptions{
				Config:  cfg,
				GameDir: req.InstancePath,
				Logger:  func(format string, args ...interface{}) {},
			})
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			if remoteM, err := syn.FetchRemoteManifest(ctx); err == nil {
				syncPreview, _ = syn.CalculatePlan(remoteM)
			}
		}

		respData := map[string]interface{}{
			"success":         true,
			"instance_path":   req.InstancePath,
			"instance_name":   filepath.Base(req.InstancePath),
			"server_url":      serverURL,
			"enable_vr":       req.EnableVR,
			"hooked_runtimes": hookedRuntimes,
		}

		if syncPreview != nil {
			respData["unchanged_count"] = syncPreview.Unchanged
			respData["download_count"] = len(syncPreview.Downloads)
			respData["delete_count"] = len(syncPreview.Deletions)
			respData["total_bytes"] = syncPreview.TotalBytes
		}

		_ = json.NewEncoder(w).Encode(respData)
	})

	// 5. API: Heartbeat ping
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.lastPing = time.Now()
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// 6. API: Clean Exit / Close
	mux.HandleFunc("/api/exit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		go func() {
			time.Sleep(300 * time.Millisecond)
			s.Stop()
		}()
	})

	// 7. API: Check for GitHub update
	mux.HandleFunc("/api/check-update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		repo := r.URL.Query().Get("repo")
		if repo == "" {
			repo = updater.DefaultGitHubRepo
		}

		info, hasUpdate, err := updater.CheckUpdate(ctx, repo, "1.0.0")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"has_update": false,
				"error":      err.Error(),
			})
			return
		}

		resp := map[string]interface{}{
			"has_update": hasUpdate,
		}
		if info != nil {
			resp["version"] = info.Version
			resp["asset_name"] = info.AssetName
			resp["asset_size"] = info.AssetSize
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Wrap mux so any incoming HTTP request flags that the client has connected
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hasConnected = true
		s.lastPing = time.Now()
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	})
}

// Start launches the GUI HTTP server and opens the application window.
func (s *Server) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.opts.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind local GUI server: %w", err)
	}
	s.listener = listener

	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	s.opts.Logger("╭─────────────────────────────────────────────────────────────╮")
	s.opts.Logger("│                ✦ LUNARIS INSTALLER GUI ✦                    │")
	s.opts.Logger("╰─────────────────────────────────────────────────────────────╯")
	s.opts.Logger("[Lunaris GUI] Running at: %s", url)
	s.opts.Logger("[Lunaris GUI] Opening application window...")
	s.opts.Logger("[Lunaris GUI] (Press Ctrl+C in this terminal to exit at any time)")

	s.httpServer = &http.Server{
		Handler: s.Handler(),
	}

	// Launch web app window in background with fail-safe watchdog
	if s.opts.OpenBrowser {
		go func() {
			time.Sleep(150 * time.Millisecond)
			if err := OpenBrowser(url); err != nil {
				s.opts.Logger("[Lunaris GUI] App window launch returned error: %v; opening default browser...", err)
				_ = OpenDefaultBrowser(url)
			}

			// Watchdog: after 2 seconds, if no browser has connected, launch default browser fallback!
			time.Sleep(2 * time.Second)
			s.mu.Lock()
			connected := s.hasConnected
			s.mu.Unlock()

			if !connected {
				s.opts.Logger("[Lunaris GUI] App window did not connect within 2s; launching default browser fallback...")
				_ = OpenDefaultBrowser(url)
			}
		}()
	}

	// Auto-shutdown monitor:
	// - Once connected, if all windows close (no ping for 20s), cleanly shut down.
	// - If never connected, wait at least 60 seconds before timing out.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		startTime := time.Now()
		for {
			select {
			case <-s.shutdownChan:
				return
			case <-ticker.C:
				s.mu.Lock()
				connected := s.hasConnected
				elapsed := time.Since(s.lastPing)
				s.mu.Unlock()

				if connected {
					if elapsed > 20*time.Second {
						s.opts.Logger("[Lunaris GUI] Browser window closed. Shutting down installer.")
						s.Stop()
						return
					}
				} else {
					if time.Since(startTime) > 60*time.Second {
						s.opts.Logger("[Lunaris GUI] No browser connection detected after 60s. Closing installer.")
						s.Stop()
						return
					}
				}
			}
		}
	}()

	err = s.httpServer.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownChan)
		if s.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_ = s.httpServer.Shutdown(ctx)
		}
	})
}

// IndexHTML is the single-page application matching the visual design of csfrederick.com / lunaris.csfrederick.com.
const IndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>Lunaris Injector Setup | Jack Frederick</title>
    <style>
        :root {
            --bg: #000000;
            --card-bg: #0d0d12;
            --card-border: #212128;
            --card-hover: #15151e;
            --text-main: rgb(212, 214, 221);
            --text-muted: rgb(140, 140, 148);
            --text-heading: aliceblue;
            --nav-bg: #141418;
            --accent: #d8dbe2;
            --code-bg: #121216;
            --hover: #ffffff;
            --primary: #282838;
            --primary-border: #484860;
            --primary-hover: #38384f;
            --success: #4ade80;
            --success-bg: rgba(34, 197, 94, 0.12);
            --success-border: rgba(34, 197, 94, 0.3);
            --vr-accent: #a5b4fc;
            --vr-bg: rgba(99, 102, 241, 0.15);
            --vr-border: rgba(99, 102, 241, 0.35);
        }

        * { box-sizing: border-box; }

        body {
            margin: 0;
            background: var(--bg);
            color: var(--text-main);
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            user-select: none;
        }

        /* Navbar matching csfrederick.com */
        .navbar {
            background: var(--nav-bg);
            border-bottom: 1px solid var(--card-border);
            padding: 14px 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 0.85rem;
            letter-spacing: 0.08em;
            text-transform: uppercase;
        }

        .nav-brand {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .nav-brand a {
            color: var(--text-heading);
            font-weight: 700;
            text-decoration: none;
            letter-spacing: 0.1em;
        }

        .nav-tag {
            background: #21212c;
            border: 1px solid var(--card-border);
            color: var(--text-muted);
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 0.72rem;
            letter-spacing: 0.05em;
        }

        .nav-links {
            display: flex;
            gap: 20px;
            align-items: center;
        }

        .server-status {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 0.8rem;
            color: var(--text-muted);
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #888;
            transition: background 0.3s;
        }
        .status-dot.online { background: var(--success); box-shadow: 0 0 8px rgba(74, 222, 128, 0.4); }
        .status-dot.offline { background: #f59e0b; }

        /* Container */
        .container {
            max-width: 840px;
            width: 100%;
            margin: 0 auto;
            padding: 32px 20px 48px;
            flex: 1;
        }

        .header-section {
            margin-bottom: 28px;
            text-align: left;
        }

        h1 {
            color: var(--text-heading);
            font-size: 2rem;
            margin: 0 0 8px 0;
            font-weight: 700;
            letter-spacing: -0.02em;
        }

        .subtitle {
            color: var(--text-muted);
            font-size: 0.95rem;
            margin: 0;
            line-height: 1.5;
        }

        /* Card components */
        .section-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 22px;
            margin-bottom: 20px;
        }

        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
        }

        .section-title {
            font-size: 1.05rem;
            font-weight: 600;
            color: var(--text-heading);
            margin: 0;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .step-num {
            background: #1c1c24;
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            width: 24px;
            height: 24px;
            border-radius: 50%;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            font-size: 0.78rem;
            font-weight: 700;
        }

        /* Instances Grid */
        .instances-list {
            display: flex;
            flex-direction: column;
            gap: 10px;
        }

        .instance-item {
            background: #101017;
            border: 1px solid var(--card-border);
            border-radius: 6px;
            padding: 14px 16px;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: flex-start;
            gap: 14px;
        }

        .instance-item:hover {
            border-color: #3f3f50;
            background: var(--card-hover);
        }

        .instance-item.selected {
            border-color: #5c6288;
            background: #151522;
            box-shadow: 0 0 16px rgba(99, 102, 241, 0.08);
        }

        .instance-radio {
            margin-top: 3px;
            cursor: pointer;
            accent-color: #6366f1;
        }

        .instance-content {
            flex: 1;
            min-width: 0;
        }

        .instance-top-row {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 4px;
            flex-wrap: wrap;
        }

        .instance-name {
            color: var(--text-heading);
            font-weight: 600;
            font-size: 0.96rem;
        }

        .badge {
            font-size: 0.72rem;
            padding: 2px 7px;
            border-radius: 4px;
            font-weight: 600;
            letter-spacing: 0.03em;
            text-transform: uppercase;
        }

        .badge-cf { background: #261f18; border: 1px solid #573a21; color: #fb923c; }
        .badge-prism { background: #16252b; border: 1px solid #1e4b57; color: #38bdf8; }
        .badge-vanilla { background: #1c261c; border: 1px solid #2f542f; color: #4ade80; }
        .badge-compat { background: var(--success-bg); border: 1px solid var(--success-border); color: var(--success); }
        .badge-rec { background: #262438; border: 1px solid #484478; color: #c4b5fd; }
        .badge-hooked { background: #1f2937; border: 1px solid #374151; color: #9ca3af; }

        .instance-path {
            font-family: Menlo, Monaco, Consolas, monospace;
            font-size: 0.78rem;
            color: var(--text-muted);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        /* Custom path input */
        .custom-path-card {
            margin-top: 10px;
            padding: 12px 14px;
            background: #0d0d12;
            border: 1px dashed var(--card-border);
            border-radius: 6px;
        }

        .custom-path-input {
            width: 100%;
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            padding: 8px 12px;
            border-radius: 5px;
            font-size: 0.85rem;
            font-family: monospace;
            outline: none;
            box-sizing: border-box;
            margin-top: 6px;
        }

        .custom-path-input:focus {
            border-color: #555;
        }

        /* VR Toggle Card */
        .toggle-row {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 16px;
        }

        .toggle-info h3 {
            margin: 0 0 4px 0;
            font-size: 0.98rem;
            color: var(--text-heading);
        }

        .toggle-info p {
            margin: 0;
            font-size: 0.86rem;
            color: var(--text-muted);
            line-height: 1.4;
        }

        /* Switch */
        .switch {
            position: relative;
            display: inline-block;
            width: 48px;
            height: 26px;
            flex-shrink: 0;
        }

        .switch input {
            opacity: 0;
            width: 0;
            height: 0;
        }

        .slider {
            position: absolute;
            cursor: pointer;
            top: 0; left: 0; right: 0; bottom: 0;
            background-color: #1e1e26;
            border: 1px solid var(--card-border);
            transition: .25s;
            border-radius: 26px;
        }

        .slider:before {
            position: absolute;
            content: "";
            height: 18px;
            width: 18px;
            left: 3px;
            bottom: 3px;
            background-color: #8c8c94;
            transition: .25s;
            border-radius: 50%;
        }

        input:checked + .slider {
            background-color: #3b4261;
            border-color: #6366f1;
        }

        input:checked + .slider:before {
            transform: translateX(22px);
            background-color: #ffffff;
        }

        .vr-banner {
            margin-top: 14px;
            padding: 10px 14px;
            border-radius: 6px;
            font-size: 0.84rem;
            display: flex;
            align-items: center;
            gap: 8px;
            transition: all 0.2s ease;
        }

        .vr-banner.vr-off {
            background: #121217;
            border: 1px solid var(--card-border);
            color: var(--text-muted);
        }

        .vr-banner.vr-on {
            background: var(--vr-bg);
            border: 1px solid var(--vr-border);
            color: var(--vr-accent);
        }

        /* Advanced Options Collapsible */
        details {
            margin-top: 16px;
            border-top: 1px solid var(--card-border);
            padding-top: 14px;
        }

        summary {
            font-size: 0.84rem;
            color: var(--text-muted);
            cursor: pointer;
            user-select: none;
            outline: none;
        }

        summary:hover {
            color: var(--text-heading);
        }

        .advanced-grid {
            display: grid;
            grid-template-columns: 1fr;
            gap: 14px;
            margin-top: 14px;
        }

        .form-group label {
            display: block;
            font-size: 0.8rem;
            color: var(--text-muted);
            margin-bottom: 6px;
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .form-input {
            width: 100%;
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            padding: 8px 12px;
            border-radius: 5px;
            font-size: 0.86rem;
            outline: none;
            box-sizing: border-box;
            font-family: monospace;
        }

        .form-input:focus {
            border-color: #555;
        }

        /* Action button */
        .action-section {
            display: flex;
            flex-direction: column;
            align-items: stretch;
            gap: 12px;
            margin-top: 24px;
        }

        .btn-install {
            background: #282838;
            color: #ffffff;
            border: 1px solid #484860;
            padding: 14px 24px;
            border-radius: 7px;
            font-size: 1.05rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 10px;
        }

        .btn-install:hover:not(:disabled) {
            background: #38384f;
            border-color: #656582;
            color: #ffffff;
        }

        .btn-install:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .btn-secondary {
            background: #141418;
            color: var(--text-main);
            border: 1px solid var(--card-border);
            padding: 10px 20px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.88rem;
            transition: all 0.2s;
            text-decoration: none;
            display: inline-flex;
            align-items: center;
            justify-content: center;
        }

        .btn-secondary:hover {
            background: #202028;
            color: var(--hover);
        }

        /* Result View */
        .result-card {
            display: none;
            background: var(--card-bg);
            border: 1px solid var(--success-border);
            border-radius: 8px;
            padding: 28px;
            margin-bottom: 24px;
            animation: fadeIn 0.3s ease;
        }

        .result-badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            background: var(--success-bg);
            color: var(--success);
            padding: 4px 10px;
            border-radius: 20px;
            font-size: 0.85rem;
            font-weight: 600;
            margin-bottom: 12px;
        }

        .result-title {
            font-size: 1.5rem;
            color: var(--text-heading);
            margin: 0 0 8px 0;
            font-weight: 700;
        }

        .result-details {
            background: #111116;
            border: 1px solid var(--card-border);
            border-radius: 6px;
            padding: 14px 18px;
            margin: 18px 0;
            font-size: 0.88rem;
        }

        .result-row {
            display: flex;
            justify-content: space-between;
            padding: 6px 0;
            border-bottom: 1px solid #1a1a22;
        }

        .result-row:last-child {
            border-bottom: none;
        }

        .result-label {
            color: var(--text-muted);
        }

        .result-val {
            color: var(--text-heading);
            font-weight: 500;
            font-family: monospace;
        }

        .instructions-box {
            background: #14141d;
            border-left: 3px solid #6366f1;
            padding: 14px 16px;
            border-radius: 0 6px 6px 0;
            margin-bottom: 20px;
            font-size: 0.92rem;
            color: #d1d5db;
            line-height: 1.5;
        }

        .result-buttons {
            display: flex;
            gap: 12px;
            flex-wrap: wrap;
        }

        /* Error alert */
        .error-banner {
            display: none;
            background: rgba(239, 68, 68, 0.15);
            border: 1px solid rgba(239, 68, 68, 0.35);
            color: #fca5a5;
            padding: 12px 16px;
            border-radius: 6px;
            margin-top: 14px;
            font-size: 0.88rem;
        }

        /* Spinner */
        .spinner {
            display: none;
            width: 18px;
            height: 18px;
            border: 2px solid rgba(255,255,255,0.2);
            border-radius: 50%;
            border-top-color: #fff;
            animation: spin 0.8s linear infinite;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(6px); }
            to { opacity: 1; transform: translateY(0); }
        }

        /* Footer */
        .footer {
            margin-top: auto;
            border-top: 1px solid var(--card-border);
            padding: 24px 20px;
            text-align: center;
            background: var(--bg);
            font-size: 0.85rem;
        }

        .footer a {
            color: #777;
            text-decoration: none;
            transition: color 0.2s;
        }
        .footer a:hover { color: #bbb; }
    </style>
</head>
<body>

    <nav class="navbar">
        <div class="nav-brand">
            <a href="https://csfrederick.com">JACK FREDERICK</a>
            <span class="nav-tag">Lunaris Injector</span>
        </div>
        <div class="nav-links">
            <div class="server-status">
                <span id="statusDot" class="status-dot"></span>
                <span id="statusText">Checking server...</span>
            </div>
        </div>
    </nav>

    <div class="container">
        <div class="header-section">
            <h1>Lunaris Injector Setup</h1>
            <p class="subtitle">Automatic modpack integrity verification and synchronization for Minecraft 1.20.1.</p>
        </div>

        <div id="mainForm">
            <!-- Step 1: Instance Selection -->
            <div class="section-card">
                <div class="section-header">
                    <h2 class="section-title">
                        <span class="step-num">1</span>
                        Select Minecraft Instance
                    </h2>
                    <span id="detectCount" style="font-size: 0.8rem; color: var(--text-muted);">Scanning...</span>
                </div>

                <div id="instancesContainer" class="instances-list">
                    <!-- Injected via JS -->
                </div>

                <div id="existingModsWarning" style="display:none; margin-top:12px; padding:12px 14px; background:rgba(245, 158, 11, 0.12); border:1px solid rgba(245, 158, 11, 0.4); border-radius:6px; color:#fde68a; font-size:0.86rem; line-height:1.45;">
                    <div style="font-weight:600; margin-bottom:4px; display:flex; align-items:center; gap:6px;">
                        <span>⚠️ Warning: Existing Mods Detected</span>
                    </div>
                    <div id="existingModsWarningText">
                        This instance already contains existing mods. Installing Lunaris will synchronize your mods with the server and will OVERWRITE or DELETE local mods not present on the server.
                    </div>
                </div>

                <div class="custom-path-card">
                    <label style="font-size:0.84rem; color:var(--text-muted); cursor:pointer;">
                        <input type="radio" name="instanceRadio" value="__custom__" id="radioCustom" style="vertical-align:middle; margin-right:6px; accent-color:#6366f1;">
                        Specify custom instance folder path:
                    </label>
                    <input type="text" id="customPathInput" class="custom-path-input" placeholder="/path/to/curseforge/minecraft/Instances/MyPack" oninput="selectCustomRadio()"/>
                </div>
            </div>

            <!-- Step 2: Modpack Options -->
            <div class="section-card">
                <div class="section-header">
                    <h2 class="section-title">
                        <span class="step-num">2</span>
                        Features & Options
                    </h2>
                </div>

                <div class="toggle-row">
                    <div class="toggle-info">
                        <h3>Windows VR Mode (Vivecraft & VR Musket)</h3>
                        <p>Enable only if you are playing in Virtual Reality with a headset on Windows. Desktop keyboard/mouse players should keep this disabled.</p>
                    </div>
                    <label class="switch">
                        <input type="checkbox" id="vrToggle" onchange="updateVRBanner()">
                        <span class="slider"></span>
                    </label>
                </div>

                <div id="vrBanner" class="vr-banner vr-off">
                    <span>• Desktop mode active: VR mods will NOT be installed.</span>
                </div>

                <details>
                    <summary>Advanced Configuration (Server URL, Java Path)</summary>
                    <div class="advanced-grid">
                        <div class="form-group">
                            <label for="serverUrlInput">Sync Server URL</label>
                            <input type="text" id="serverUrlInput" class="form-input" value="https://lunaris.csfrederick.com"/>
                        </div>
                        <div class="form-group">
                            <label for="javaPathInput">Custom Java 17 Binary (optional)</label>
                            <input type="text" id="javaPathInput" class="form-input" placeholder="Leave blank to auto-detect CurseForge Java runtime"/>
                        </div>
                    </div>
                </details>
            </div>

            <!-- Error Banner -->
            <div id="errorBanner" class="error-banner"></div>

            <!-- Action Button -->
            <div class="action-section">
                <button id="btnInstall" class="btn-install" onclick="submitInstall()">
                    <span id="spinner" class="spinner"></span>
                    <span id="btnText">Install & Hook CurseForge</span>
                </button>
                <div style="text-align:center; font-size:0.8rem; color:var(--text-muted);">
                    Hooks your CurseForge Java runtime automatically. Zero launcher profile tampering required.
                </div>
            </div>
        </div>

        <!-- Result View -->
        <div id="resultCard" class="result-card">
            <div class="result-badge">✔ Installation Complete</div>
            <h2 class="result-title" id="resInstanceName">Lunaris V.2</h2>
            <p style="color:var(--text-muted); margin:0 0 16px 0; font-size:0.92rem;">
                LunarisInjector has been successfully configured and hooked into your CurseForge environment.
            </p>

            <div class="instructions-box">
                <strong>You're ready to play!</strong><br/>
                Simply open <strong>CurseForge</strong> and click <strong>PLAY</strong> on your modpack.<br/>
                Lunaris will automatically verify and synchronize all files before starting Minecraft.
            </div>

            <div class="result-details">
                <div class="result-row">
                    <span class="result-label">Instance Location</span>
                    <span class="result-val" id="resPath">-</span>
                </div>
                <div class="result-row">
                    <span class="result-label">Sync Server</span>
                    <span class="result-val" id="resServer">-</span>
                </div>
                <div class="result-row">
                    <span class="result-label">VR Feature Support</span>
                    <span class="result-val" id="resVR">-</span>
                </div>
                <div class="result-row">
                    <span class="result-label">CurseForge Java Hook</span>
                    <span class="result-val" id="resHook">-</span>
                </div>
            </div>

            <div class="result-buttons">
                <button class="btn-install" style="padding:10px 20px; font-size:0.9rem;" onclick="exitInstaller()">Done & Close Window</button>
                <a href="https://lunaris.csfrederick.com" target="_blank" class="btn-secondary">View Sync Server Portal</a>
            </div>
        </div>
    </div>

    <footer class="footer">
        <p style="margin:0 0 4px 0; color:var(--text-heading); font-weight:600;">Jack Frederick</p>
        <p style="margin:0; color:#555;">LunarisInjector Installer | <a href="https://csfrederick.com">csfrederick.com</a></p>
    </footer>

    <script>
        let detectedInstances = [];
        let selectedPath = "";

        // Send periodic keepalive ping to local backend
        setInterval(() => {
            fetch('/api/ping').catch(() => {});
        }, 4000);

        // Check remote server status
        function checkServerStatus() {
            const srv = document.getElementById('serverUrlInput').value;
            fetch('/api/status?server=' + encodeURIComponent(srv))
                .then(r => r.json())
                .then(data => {
                    const dot = document.getElementById('statusDot');
                    const text = document.getElementById('statusText');
                    if (data.online) {
                        dot.className = "status-dot online";
                        text.textContent = "Sync Server: Online (" + (data.file_count || 0) + " files)";
                    } else {
                        dot.className = "status-dot offline";
                        text.textContent = "Sync Server: Offline Launch Mode";
                    }
                })
                .catch(() => {
                    document.getElementById('statusDot').className = "status-dot offline";
                    document.getElementById('statusText').textContent = "Offline Mode";
                });
        }

        // Load detected instances
        function loadInstances() {
            fetch('/api/instances')
                .then(r => r.json())
                .then(list => {
                    detectedInstances = list || [];
                    renderInstances();
                })
                .catch(err => {
                    document.getElementById('detectCount').textContent = "Error scanning instances";
                });
        }

        function renderInstances() {
            const container = document.getElementById('instancesContainer');
            const countLabel = document.getElementById('detectCount');
            container.innerHTML = "";

            if (detectedInstances.length === 0) {
                countLabel.textContent = "No instances auto-detected";
                selectCustomRadio();
                return;
            }

            countLabel.textContent = detectedInstances.length + " detected";

            let firstCompatibleIndex = -1;
            detectedInstances.forEach((inst, i) => {
                if (inst.is_compatible && firstCompatibleIndex === -1) {
                    firstCompatibleIndex = i;
                }
            });

            detectedInstances.forEach((inst, i) => {
                const item = document.createElement('div');
                item.className = "instance-item";
                item.id = "inst_item_" + i;

                const isSelected = (i === (firstCompatibleIndex >= 0 ? firstCompatibleIndex : 0));
                if (isSelected) {
                    item.classList.add("selected");
                    selectedPath = inst.path;
                    if (inst.enable_vr) {
                        document.getElementById('vrToggle').checked = true;
                        updateVRBanner();
                    }
                    updateModsWarning(inst);
                }

                let badges = "";
                if (i === 0 && inst.is_compatible) {
                    badges += '<span class="badge badge-rec">✦ Recommended</span> ';
                }
                if (inst.launcher === "CurseForge") {
                    badges += '<span class="badge badge-cf">CurseForge</span> ';
                } else if (inst.launcher === "Prism") {
                    badges += '<span class="badge badge-prism">Prism</span> ';
                } else {
                    badges += '<span class="badge badge-vanilla">' + escapeHtml(inst.launcher) + '</span> ';
                }

                if (inst.is_compatible) {
                    badges += '<span class="badge badge-compat">1.20.1</span> ';
                } else {
                    badges += '<span class="badge" style="background:#2b1818; border:1px solid #572121; color:#f87171;">MC ' + escapeHtml(inst.game_version || 'Unknown') + '</span> ';
                }

                if (inst.mod_count > 0 && !inst.is_injected) {
                    badges += '<span class="badge" style="background:#2b1d0c; border:1px solid #78350f; color:#fcd34d;">⚠️ ' + inst.mod_count + ' Mods</span> ';
                }

                if (inst.is_injected) {
                    badges += '<span class="badge badge-hooked">Hooked</span> ';
                }

                item.innerHTML = ` + "`" + `
                    <input type="radio" name="instanceRadio" class="instance-radio" value="${i}" ${isSelected ? 'checked' : ''}/>
                    <div class="instance-content">
                        <div class="instance-top-row">
                            <span class="instance-name">${escapeHtml(inst.name)}</span>
                            ${badges}
                        </div>
                        <div class="instance-path" title="${escapeHtml(inst.path)}">${escapeHtml(inst.path)}</div>
                    </div>
                ` + "`" + `;

                item.onclick = function() {
                    selectInstance(i);
                };

                container.appendChild(item);
            });
        }

        function updateModsWarning(inst) {
            const warnBox = document.getElementById('existingModsWarning');
            const warnText = document.getElementById('existingModsWarningText');
            if (!warnBox || !warnText) return;

            if (inst && inst.mod_count > 0 && !inst.is_injected) {
                warnBox.style.display = "block";
                warnText.innerHTML = "This instance already contains <strong>" + inst.mod_count + " mod file(s)</strong> in its 'mods/' folder. Installing LunarisInjector will synchronize your files with the server, which will <strong>OVERWRITE or DELETE</strong> any existing mods that are not part of the remote modpack.";
            } else {
                warnBox.style.display = "none";
            }
        }

        function selectInstance(index) {
            document.querySelectorAll('.instance-item').forEach(el => el.classList.remove('selected'));
            document.getElementById('radioCustom').checked = false;

            const target = document.getElementById('inst_item_' + index);
            if (target) {
                target.classList.add('selected');
                target.querySelector('input').checked = true;
            }

            const inst = detectedInstances[index];
            selectedPath = inst.path;
            if (inst.enable_vr) {
                document.getElementById('vrToggle').checked = true;
            } else if (!document.getElementById('vrToggle').dataset.userModified) {
                document.getElementById('vrToggle').checked = false;
            }
            updateVRBanner();
            updateModsWarning(inst);
        }

        function selectCustomRadio() {
            document.querySelectorAll('.instance-item').forEach(el => el.classList.remove('selected'));
            document.querySelectorAll('.instance-radio').forEach(el => el.checked = false);
            document.getElementById('radioCustom').checked = true;
            selectedPath = document.getElementById('customPathInput').value.trim();
            updateModsWarning(null);
        }

        function updateVRBanner() {
            const toggle = document.getElementById('vrToggle');
            const banner = document.getElementById('vrBanner');
            toggle.dataset.userModified = "true";

            if (toggle.checked) {
                banner.className = "vr-banner vr-on";
                banner.innerHTML = "<span>✦ <strong>VR Mode Active</strong>: Vivecraft and VR Musket will be synchronized.</span>";
            } else {
                banner.className = "vr-banner vr-off";
                banner.innerHTML = "<span>• Desktop mode active: VR mods will NOT be installed.</span>";
            }
        }

        function submitInstall() {
            const btn = document.getElementById('btnInstall');
            const spinner = document.getElementById('spinner');
            const btnText = document.getElementById('btnText');
            const errBanner = document.getElementById('errorBanner');

            errBanner.style.display = "none";

            let path = selectedPath;
            if (document.getElementById('radioCustom').checked) {
                path = document.getElementById('customPathInput').value.trim();
            }

            if (!path) {
                showError("Please select an instance or enter a custom directory path.");
                return;
            }

            // Check if selected instance already has mods and warn user
            const currentInst = detectedInstances.find(it => it.path === path);
            if (currentInst && currentInst.mod_count > 0 && !currentInst.is_injected) {
                const confirmed = confirm(
                    "⚠️ WARNING: OVERWRITE EXISTING MODS?\n\n" +
                    "Instance '" + currentInst.name + "' already contains " + currentInst.mod_count + " mod(s) in its 'mods/' folder.\n\n" +
                    "Installing LunarisInjector will synchronize your files with the server and will OVERWRITE or DELETE existing mods not present in the modpack manifest.\n\n" +
                    "Are you sure you want to proceed and overwrite existing mods?"
                );
                if (!confirmed) {
                    return;
                }
            }

            const payload = {
                instance_path: path,
                server_url: document.getElementById('serverUrlInput').value.trim(),
                enable_vr: document.getElementById('vrToggle').checked,
                real_java_path: document.getElementById('javaPathInput').value.trim()
            };

            btn.disabled = true;
            spinner.style.display = "inline-block";
            btnText.textContent = "Installing Lunaris...";

            fetch('/api/install', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
            .then(r => r.json().then(data => ({ status: r.status, body: data })))
            .then(({ status, body }) => {
                btn.disabled = false;
                spinner.style.display = "none";
                btnText.textContent = "Install & Hook CurseForge";

                if (status !== 200 || !body.success) {
                    showError(body.error || "Installation failed.");
                    return;
                }

                // Show success screen
                document.getElementById('mainForm').style.display = "none";
                const resCard = document.getElementById('resultCard');
                resCard.style.display = "block";

                document.getElementById('resInstanceName').textContent = body.instance_name || "Minecraft Modpack";
                document.getElementById('resPath').textContent = body.instance_path;
                document.getElementById('resServer').textContent = body.server_url;
                document.getElementById('resVR').textContent = body.enable_vr ? "ENABLED (Vivecraft Active)" : "Disabled (Desktop Mode)";
                document.getElementById('resHook').textContent = (body.hooked_runtimes && body.hooked_runtimes.length > 0) ? "Active (" + body.hooked_runtimes[0] + ")" : "Configured";
            })
            .catch(err => {
                btn.disabled = false;
                spinner.style.display = "none";
                btnText.textContent = "Install & Hook CurseForge";
                showError("Network communication error: " + err.message);
            });
        }

        function showError(msg) {
            const b = document.getElementById('errorBanner');
            b.textContent = "❌ " + msg;
            b.style.display = "block";
            b.scrollIntoView({ behavior: 'smooth' });
        }

        function exitInstaller() {
            fetch('/api/exit', { method: 'POST' }).finally(() => {
                window.close();
                document.body.innerHTML = ` + "`" + `
                    <div style="max-width:500px; margin:80px auto; text-align:center; color:aliceblue;">
                        <h2>Installer Closed</h2>
                        <p style="color:#8c8c94;">You can now close this tab and return to CurseForge!</p>
                    </div>
                ` + "`" + `;
            });
        }

        function escapeHtml(text) {
            if (!text) return "";
            return String(text)
                .replace(/&/g, "&amp;")
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;")
                .replace(/"/g, "&quot;")
                .replace(/'/g, "&#039;");
        }

        // Initialize on load
        window.addEventListener('DOMContentLoaded', () => {
            loadInstances();
            checkServerStatus();
        });
    </script>
</body>
</html>
`
