package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/slide/LunarisInjector/pkg/manifest"
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
		opts.SyncDirs = []string{"mods"}
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

	for _, subDir := range s.opts.SyncDirs {
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

			// Check cache
			cached, ok := s.fileCache[normRel]
			if ok && cached.modTime.Equal(fi.ModTime()) && cached.size == fi.Size() {
				entries = append(entries, manifest.FileEntry{
					Path:   normRel,
					SHA256: cached.sha256,
					Size:   cached.size,
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
				Path:   normRel,
				SHA256: hash,
				Size:   size,
			})
			return nil
		})

		if err != nil {
			return nil, err
		}
	}

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
		for _, f := range m.Files {
			totalBytes += f.Size
		}

		data := struct {
			FileCount  int
			TotalMB    float64
			LastScan   string
			Files      []manifest.FileEntry
			ServerPort int
		}{
			FileCount:  len(m.Files),
			TotalMB:    float64(totalBytes) / (1024 * 1024),
			LastScan:   s.lastScanned.Format(time.RFC1123),
			Files:      m.Files,
			ServerPort: s.opts.Port,
		}

		tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>Lunaris Sync Server</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #e2e8f0; margin: 40px; }
        .card { background: #1e293b; border-radius: 8px; padding: 24px; max-width: 900px; margin: auto; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.5); }
        h1 { color: #38bdf8; margin-top: 0; }
        .stats { display: flex; gap: 20px; margin-bottom: 24px; }
        .stat-box { background: #334155; padding: 12px 20px; border-radius: 6px; }
        .stat-val { font-size: 24px; font-weight: bold; color: #f8fafc; }
        .stat-label { font-size: 13px; color: #94a3b8; }
        table { width: 100%; border-collapse: collapse; margin-top: 16px; }
        th, td { text-align: left; padding: 10px; border-bottom: 1px solid #334155; font-size: 14px; }
        th { color: #94a3b8; font-weight: 600; }
        .hash { font-family: monospace; font-size: 12px; color: #a5b4fc; }
        a { color: #38bdf8; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
<div class="card">
    <h1>🚀 Lunaris Sync Server</h1>
    <p>Serving synced files for Minecraft clients. Connected clients will automatically synchronize these files before launching.</p>
    
    <div class="stats">
        <div class="stat-box">
            <div class="stat-val">{{.FileCount}}</div>
            <div class="stat-label">Tracked Files</div>
        </div>
        <div class="stat-box">
            <div class="stat-val">{{printf "%.2f" .TotalMB}} MB</div>
            <div class="stat-label">Total Size</div>
        </div>
        <div class="stat-box">
            <div class="stat-val"><a href="/manifest.json">manifest.json</a></div>
            <div class="stat-label">Manifest Endpoint</div>
        </div>
    </div>

    <h3>Tracked Files</h3>
    <table>
        <thead>
            <tr>
                <th>File Path</th>
                <th>Size (KB)</th>
                <th>SHA-256</th>
            </tr>
        </thead>
        <tbody>
            {{range .Files}}
            <tr>
                <td><a href="/{{.Path}}">{{.Path}}</a></td>
                <td>{{printf "%.1f" (div (toFloat .Size) 1024.0)}} KB</td>
                <td class="hash">{{printf "%.16s..." .SHA256}}</td>
            </tr>
            {{else}}
            <tr><td colspan="3" style="text-align:center; color:#94a3b8;">No files found in sync directory. Drop your mods into the folder!</td></tr>
            {{end}}
        </tbody>
    </table>
</div>
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
