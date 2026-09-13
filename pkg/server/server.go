package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
)

// ServerOptions configures the sync server.
type ServerOptions struct {
	RootDir     string
	SyncDirs    []string
	Port        int
	Host        string
	AutoRefresh time.Duration
}

// Server provides dynamic file serving and manifest generation.
type Server struct {
	opts       ServerOptions
	mu         sync.RWMutex
	manifest   *manifest.Manifest
	fileCache  map[string]cachedFile
	lastScanned time.Time
}

type cachedFile struct {
	sha256  string
	size    int64
	modTime time.Time
}

// New creates a new Server instance.
func New(opts ServerOptions) *Server {
	if opts.Port <= 0 {
		opts.Port = 8080
	}
	if len(opts.SyncDirs) == 0 {
		opts.SyncDirs = []string{"mods", "config", "global_packs"}
	}
	if opts.AutoRefresh <= 0 {
		opts.AutoRefresh = 5 * time.Second
	}

	return &Server{
		opts:      opts,
		fileCache: make(map[string]cachedFile),
	}
}

// RefreshManifest scans the root directory and updates the manifest.
// Uses file size and modification time to avoid unnecessary rehashing.
func (s *Server) RefreshManifest() (*manifest.Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := []manifest.FileEntry{}

	scanDirs := append([]string{}, s.opts.SyncDirs...)
	optDir := filepath.Join(s.opts.RootDir, "optional")
	if info, err := os.Stat(optDir); err == nil && info.IsDir() {
		hasOpt := false
		for _, d := range scanDirs {
			if d == "optional" {
				hasOpt = true
				break
			}
		}
		if !hasOpt {
			scanDirs = append(scanDirs, "optional")
		}
	}

	for _, subDir := range scanDirs {
		targetPath := filepath.Join(s.opts.RootDir, subDir)
		info, err := os.Stat(targetPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.Walk(targetPath, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return err
			}

			rel, err := filepath.Rel(s.opts.RootDir, path)
			if err != nil {
				return err
			}
			normRel := manifest.NormalizePath(rel)

			if manifest.ShouldIgnore(normRel, nil) {
				return nil
			}

			var feature string
			var destPath string
			if strings.HasPrefix(normRel, "optional/") {
				parts := strings.Split(normRel, "/")
				if len(parts) >= 3 {
					feature = parts[1]
					destPath = strings.Join(parts[2:], "/")
				}
			}

			// Check cache
			cached, ok := s.fileCache[normRel]
			if ok && cached.modTime.Equal(fi.ModTime()) && cached.size == fi.Size() {
				entries = append(entries, manifest.FileEntry{
					Path:     normRel,
					SHA256:   cached.sha256,
					Size:     cached.size,
					Feature:  feature,
					DestPath: destPath,
				})
				return nil
			}

			// Compute new hash
			hash, size, err := manifest.ComputeSHA256(path)
			if err != nil {
				return fmt.Errorf("failed to hash %s: %w", path, err)
			}

			s.fileCache[normRel] = cachedFile{
				sha256:  hash,
				size:    size,
				modTime: fi.ModTime(),
			}

			entries = append(entries, manifest.FileEntry{
				Path:     normRel,
				SHA256:   hash,
				Size:     size,
				Feature:  feature,
				DestPath: destPath,
			})
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	s.manifest = &manifest.Manifest{
		Version:   1,
		Timestamp: time.Now().Unix(),
		Files:     entries,
	}
	s.lastScanned = time.Now()

	return s.manifest, nil
}

// GetManifest returns the current manifest, refreshing if needed.
func (s *Server) GetManifest() (*manifest.Manifest, error) {
	s.mu.RLock()
	stale := s.manifest == nil || time.Since(s.lastScanned) > s.opts.AutoRefresh
	s.mu.RUnlock()

	if stale {
		return s.RefreshManifest()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest, nil
}

// Handler returns an http.Handler serving the manifest and files.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Manifest endpoint
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.GetManifest()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to generate manifest: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		_ = json.NewEncoder(w).Encode(m)
	})

	// 2. Health & Status Dashboard
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			// Forward other paths to file server
			s.serveFile(w, r)
			return
		}

		m, err := s.GetManifest()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var totalBytes int64
		var modCount, configCount, packCount, optionalCount int
		for _, f := range m.Files {
			totalBytes += f.Size
			if f.Feature != "" {
				optionalCount++
			}
			target := f.ClientPath()
			if strings.HasPrefix(target, "mods/") {
				modCount++
			} else if strings.HasPrefix(target, "config/") {
				configCount++
			} else if strings.HasPrefix(target, "global_packs/") {
				packCount++
			}
		}

		data := struct {
			FileCount     int
			ModCount      int
			ConfigCount   int
			PackCount     int
			OptionalCount int
			TotalMB       float64
			LastScan      string
			Files         []manifest.FileEntry
			ServerPort    int
		}{
			FileCount:     len(m.Files),
			ModCount:      modCount,
			ConfigCount:   configCount,
			PackCount:     packCount,
			OptionalCount: optionalCount,
			TotalMB:       float64(totalBytes) / (1024 * 1024),
			LastScan:      s.lastScanned.Format(time.RFC1123),
			Files:         m.Files,
			ServerPort:    s.opts.Port,
		}

		tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>Lunaris Sync | Jack Frederick</title>
    <style>
        :root {
            --bg: #000000;
            --card-bg: #0d0d12;
            --card-border: #212128;
            --text-main: rgb(212, 214, 221);
            --text-muted: rgb(140, 140, 148);
            --text-heading: aliceblue;
            --nav-bg: #141418;
            --accent: #d8dbe2;
            --code-bg: #121216;
            --hover: #ffffff;
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

        .nav-brand a {
            color: var(--text-heading);
            font-weight: 700;
            text-decoration: none;
            letter-spacing: 0.1em;
        }

        .nav-links {
            display: flex;
            gap: 24px;
            align-items: center;
        }

        .nav-links a {
            color: var(--text-muted);
            text-decoration: none;
            transition: color 0.2s ease;
            position: relative;
            padding-bottom: 4px;
        }

        .nav-links a:hover {
            color: var(--hover);
        }

        .nav-links a.active {
            color: var(--text-heading);
            font-weight: 600;
        }

        .nav-links a.active::after {
            content: '';
            position: absolute;
            bottom: -2px;
            left: 0;
            right: 0;
            height: 2px;
            background: var(--text-heading);
            border-radius: 2px;
        }

        /* Container */
        .container {
            max-width: 1050px;
            width: 100%;
            margin: 0 auto;
            padding: 40px 20px;
            flex: 1;
        }

        .header-section {
            margin-bottom: 36px;
            text-align: left;
        }

        h1 {
            color: var(--text-heading);
            font-size: 2.2rem;
            margin: 0 0 10px 0;
            font-weight: 700;
            letter-spacing: -0.02em;
        }

        .subtitle {
            color: var(--text-muted);
            font-size: 1rem;
            margin: 0;
            line-height: 1.5;
        }

        /* Stats row */
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
            margin-bottom: 32px;
        }

        .stat-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 18px 20px;
        }

        .stat-num {
            font-size: 1.8rem;
            font-weight: 700;
            color: var(--text-heading);
            margin-bottom: 4px;
        }

        .stat-title {
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            color: var(--text-muted);
        }

        /* Instructions Card */
        .setup-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 24px;
            margin-bottom: 36px;
        }

        .setup-card h2 {
            margin-top: 0;
            font-size: 1.15rem;
            color: var(--text-heading);
        }

        .setup-card p {
            font-size: 0.92rem;
            color: var(--text-main);
            margin-bottom: 12px;
        }

        .steps-grid {
            display: grid;
            grid-template-columns: 1fr;
            gap: 20px;
            margin-top: 18px;
        }

        .step-item {
            display: flex;
            align-items: flex-start;
            gap: 16px;
        }

        .step-badge {
            background: #22222b;
            color: var(--text-heading);
            width: 28px;
            height: 28px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.85rem;
            font-weight: 700;
            flex-shrink: 0;
            border: 1px solid var(--card-border);
        }

        .download-buttons {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
            margin-top: 12px;
        }

        .btn {
            display: inline-block;
            background: #191920;
            color: var(--text-main);
            border: 1px solid var(--card-border);
            padding: 8px 16px;
            border-radius: 6px;
            text-decoration: none;
            font-size: 0.86rem;
            font-weight: 500;
            transition: all 0.2s ease;
        }

        .btn:hover {
            background: #252530;
            color: var(--hover);
            border-color: #3f3f4e;
        }

        .btn-primary {
            background: #282838;
            color: #ffffff;
            border-color: #484860;
            font-weight: 600;
        }

        .btn-primary:hover {
            background: #38384f;
            border-color: #656582;
        }

        kbd {
            background: #22222a;
            border: 1px solid #3c3c4a;
            border-radius: 4px;
            padding: 2px 6px;
            font-size: 0.8rem;
            color: var(--text-heading);
        }

        pre {
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            border-radius: 6px;
            padding: 14px;
            font-family: Menlo, Monaco, Consolas, 'Courier New', monospace;
            font-size: 0.86rem;
            color: #d1d5db;
            overflow-x: auto;
            margin: 0;
        }

        /* Search input & table */
        .table-controls {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
            flex-wrap: wrap;
            gap: 12px;
        }

        .table-controls h2 {
            margin: 0;
            font-size: 1.25rem;
            color: var(--text-heading);
        }

        .search-box {
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            padding: 8px 14px;
            border-radius: 6px;
            font-size: 0.88rem;
            width: 260px;
            outline: none;
            transition: border-color 0.2s;
        }

        .search-box:focus {
            border-color: #555;
        }

        .table-wrapper {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            overflow: hidden;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            text-align: left;
        }

        th {
            background: #111116;
            color: var(--text-muted);
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            padding: 12px 18px;
            border-bottom: 1px solid var(--card-border);
        }

        td {
            padding: 12px 18px;
            font-size: 0.88rem;
            border-bottom: 1px solid #16161c;
        }

        tr:last-child td {
            border-bottom: none;
        }

        tr:hover td {
            background: rgba(255, 255, 255, 0.02);
        }

        td a {
            color: var(--text-main);
            text-decoration: none;
            transition: color 0.15s;
        }

        td a:hover {
            color: var(--hover);
            text-decoration: underline;
        }

        .hash-cell {
            font-family: monospace;
            font-size: 0.78rem;
            color: #71717a;
        }

        /* Footer */
        .footer {
            margin-top: auto;
            border-top: 1px solid var(--card-border);
            padding: 30px 20px;
            text-align: center;
            background: var(--bg);
        }

        .footer-name {
            color: var(--text-heading);
            font-weight: 600;
            font-size: 0.95rem;
            margin: 0 0 6px 0;
        }

        .footer-note {
            color: #555;
            font-size: 0.85rem;
            margin: 0;
        }

        .footer-note a {
            color: #777;
            text-decoration: none;
            transition: color 0.2s ease;
        }

        .footer-note a:hover {
            color: #bbb;
        }
    </style>
</head>
<body>

    <nav class="navbar">
        <div class="nav-brand">
            <a href="https://csfrederick.com">JACK FREDERICK</a>
        </div>
        <div class="nav-links">
            <a href="https://csfrederick.com">Home</a>
            <a href="https://csfrederick.com/minecraft">Minecraft</a>
            <a href="/" class="active">Lunaris Sync</a>
            <a href="https://csfrederick.com/contact">Contact</a>
        </div>
    </nav>

    <div class="container">
        <div class="header-section">
            <h1>Lunaris Sync Server</h1>
            <p class="subtitle">Modpack synchronization and SHA-256 integrity verification for Minecraft 1.20.1.</p>
        </div>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-num">{{.FileCount}}</div>
                <div class="stat-title">Total Tracked Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-num">{{.ModCount}}</div>
                <div class="stat-title">Mod Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-num">{{.ConfigCount}} / {{.PackCount}}</div>
                <div class="stat-title">Configs / Global Packs</div>
            </div>
            <div class="stat-card">
                <div class="stat-num">{{.OptionalCount}}</div>
                <div class="stat-title">Optional VR Files</div>
            </div>
            <div class="stat-card">
                <div class="stat-num">{{printf "%.1f" .TotalMB}} MB</div>
                <div class="stat-title">Repository Size</div>
            </div>
            <div class="stat-card">
                <div class="stat-num">1.20.1</div>
                <div class="stat-title">Minecraft Version</div>
            </div>
            <div class="stat-card">
                <div class="stat-num"><a href="/manifest.json" style="color:var(--text-heading); text-decoration:none;">JSON</a></div>
                <div class="stat-title">Manifest Feed</div>
            </div>
        </div>

        <div class="setup-card">
            <h2>Quick Setup for CurseForge</h2>
            <div class="steps-grid">
                <div class="step-item">
                    <span class="step-badge">1</span>
                    <div>
                        <strong>Download the Lunaris Installer</strong>
                        <p>Select your operating system:</p>
                        <div class="download-buttons">
                            <a href="/installers/lunaris-windows-amd64.exe?v=1.0.1" class="btn btn-primary" download>Download for Windows (.exe)</a>
                            <a href="/installers/lunaris-linux-amd64?v=1.0.1" class="btn" download>Download for Linux (64-bit)</a>
                            <a href="/installers/lunaris-darwin-arm64?v=1.0.1" class="btn" download>macOS (Apple Silicon)</a>
                            <a href="/installers/lunaris-darwin-amd64?v=1.0.1" class="btn" download>macOS (Intel)</a>
                        </div>
                    </div>
                </div>
                <div class="step-item">
                    <span class="step-badge">2</span>
                    <div>
                        <strong>Run the Installer</strong>
                        <p>Double-click the downloaded installer to open the setup window. Select your auto-detected <strong>Lunaris V.2</strong> instance, choose whether to enable Windows VR, and click <strong>Install & Hook CurseForge</strong>. Zero manual launcher configuration required.</p>
                    </div>
                </div>
                <div class="step-item">
                    <span class="step-badge">3</span>
                    <div>
                        <strong>Click 'Play' in CurseForge</strong>
                        <p>Click Play in CurseForge on <strong>Lunaris V.2</strong>. Lunaris will automatically sync all files before starting Minecraft!</p>
                    </div>
                </div>
            </div>
        </div>

        <div class="table-controls">
            <h2>Tracked Files</h2>
            <input type="text" id="modSearch" class="search-box" placeholder="Filter files (e.g. mods, config, vr)..." onkeyup="filterMods()"/>
        </div>

        <div class="table-wrapper">
            <table id="modsTable">
                <thead>
                    <tr>
                        <th>File Name</th>
                        <th style="width: 140px;">Size</th>
                        <th style="width: 220px;">SHA-256 Digest</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Files}}
                    <tr>
                        <td>
                            <a href="/{{.Path}}">{{if .DestPath}}{{.DestPath}}{{else}}{{.Path}}{{end}}</a>
                            {{if .Feature}}<span style="display:inline-block; font-size:0.75rem; padding: 2px 6px; border-radius: 4px; background: #282838; border: 1px solid #484860; color: #a5b4fc; margin-left: 8px; text-transform: uppercase;">Optional: {{.Feature}}</span>{{end}}
                        </td>
                        <td>{{printf "%.1f" (div (toFloat .Size) 1024.0)}} KB</td>
                        <td class="hash-cell">{{printf "%.16s..." .SHA256}}</td>
                    </tr>
                    {{else}}
                    <tr><td colspan="3" style="text-align:center; color:var(--text-muted);">No files found in sync repository.</td></tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>

    <footer class="footer">
        <p class="footer-name">Jack Frederick</p>
        <p class="footer-note">This site is home made and self hosted. | <a href="https://csfrederick.com/privacy">Privacy Policy</a></p>
    </footer>

    <script>
        function filterMods() {
            var input = document.getElementById("modSearch");
            var filter = input.value.toLowerCase();
            var rows = document.querySelectorAll("#modsTable tbody tr");
            rows.forEach(function(row) {
                var text = row.querySelector("td a") ? row.querySelector("td a").textContent.toLowerCase() : "";
                row.style.display = text.indexOf(filter) > -1 ? "" : "none";
            });
        }
    </script>
</body>
</html>`

		funcMap := template.FuncMap{
			"div": func(a, b float64) float64 {
				if b == 0 {
					return 0
				}
				return a / b
			},
			"toFloat": func(i int64) float64 {
				return float64(i)
			},
		}

		t, parseErr := template.New("index").Funcs(funcMap).Parse(tmpl)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = t.Execute(w, data)
	})

	return mux
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/")
	// Also allow /files/<path>
	relPath = strings.TrimPrefix(relPath, "files/")

	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	fullPath := filepath.Join(s.opts.RootDir, cleanRel)

	// Directory traversal guard
	if !strings.HasPrefix(fullPath, filepath.Clean(s.opts.RootDir)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if strings.HasPrefix(cleanRel, "installers") {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}
	http.ServeFile(w, r, fullPath)
}

// Start runs the HTTP server.
func (s *Server) Start() error {
	if _, err := s.RefreshManifest(); err != nil {
		return fmt.Errorf("initial scan failed: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", s.opts.Host, s.opts.Port)
	fmt.Printf("[Lunaris Server] Listening on http://%s\n", addr)
	fmt.Printf("[Lunaris Server] Serving directory: %s\n", s.opts.RootDir)
	fmt.Printf("[Lunaris Server] Manifest endpoint: http://%s/manifest.json\n", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	return srv.ListenAndServe()
}
