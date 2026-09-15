package gui

import (
	"bufio"
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

	"github.com/engineeringjack/LunarisInjector/pkg/modtracker"
)

// PromptOptions configures the mod change decision prompt.
type PromptOptions struct {
	InstanceDir  string
	InstanceName string
	Changes      []modtracker.ModChange
	IsCLI        bool
	Logger       func(format string, args ...interface{})
}

// ShowChangePrompt displays a prompt (GUI window or terminal fallback) asking the user
// how to handle detected modpack modifications.
// It returns a decision map (path -> "keep" | "revert"), a boolean indicating if launch was cancelled, and error.
func ShowChangePrompt(opts PromptOptions) (map[string]modtracker.UserDecision, bool, error) {
	if len(opts.Changes) == 0 {
		return map[string]modtracker.UserDecision{}, false, nil
	}

	if opts.Logger == nil {
		opts.Logger = func(string, ...interface{}) {}
	}

	if opts.InstanceName == "" && opts.InstanceDir != "" {
		opts.InstanceName = filepath.Base(opts.InstanceDir)
	}

	// Terminal CLI fallback if explicitly in CLI mode
	if opts.IsCLI {
		return promptCLI(opts)
	}

	// Launch GUI prompt server with browser window
	decisions, cancelled, err := promptGUI(opts)
	if err != nil {
		opts.Logger("[Lunaris] GUI prompt window could not be opened (%v); falling back to terminal prompt...", err)
		return promptCLI(opts)
	}

	return decisions, cancelled, nil
}

func promptCLI(opts PromptOptions) (map[string]modtracker.UserDecision, bool, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("╭─────────────────────────────────────────────────────────────╮")
	fmt.Println("│          ✦ LUNARIS: MODPACK MODIFICATIONS DETECTED ✦         │")
	fmt.Println("╰─────────────────────────────────────────────────────────────╯")
	fmt.Printf("Instance: %s (%s)\n", opts.InstanceName, opts.InstanceDir)
	fmt.Println("Modifications detected since last launch:")
	fmt.Println()

	for i, ch := range opts.Changes {
		tag := "[+ ADDED]   "
		if ch.Type == modtracker.ChangeRemoved {
			tag = "[- REMOVED] "
		} else if ch.Type == modtracker.ChangeModified {
			tag = "[~ MODIFIED]"
		}

		sizeStr := ""
		if ch.Size > 0 {
			sizeStr = fmt.Sprintf(" (%.2f MB)", float64(ch.Size)/(1024*1024))
		}
		fmt.Printf("  [%d] %s %s%s\n", i+1, tag, ch.Path, sizeStr)
	}

	fmt.Println()
	fmt.Println("How would you like to proceed?")
	fmt.Println("  [1] Keep all my changes (remember custom mods/configs & launch) [DEFAULT]")
	fmt.Println("  [2] Revert all changes to match server pack")
	fmt.Println("  [3] Decide for each item individually")
	fmt.Println("  [4] Reset & forget all custom modifications")
	fmt.Println("  [q] Cancel launch")

	for {
		fmt.Print("\nSelect an option [default: 1]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" || input == "1" {
			decisions := make(map[string]modtracker.UserDecision, len(opts.Changes))
			for _, ch := range opts.Changes {
				decisions[ch.Path] = modtracker.DecisionKeep
			}
			return decisions, false, nil
		}

		if input == "2" {
			decisions := make(map[string]modtracker.UserDecision, len(opts.Changes))
			for _, ch := range opts.Changes {
				decisions[ch.Path] = modtracker.DecisionRevert
			}
			return decisions, false, nil
		}

		if input == "4" {
			_ = modtracker.ResetToPack(opts.InstanceDir)
			decisions := make(map[string]modtracker.UserDecision, len(opts.Changes))
			for _, ch := range opts.Changes {
				decisions[ch.Path] = modtracker.DecisionRevert
			}
			fmt.Println("✔ Reset all custom modifications. Reverting to server pack...")
			return decisions, false, nil
		}

		if strings.EqualFold(input, "q") || strings.EqualFold(input, "quit") || strings.EqualFold(input, "exit") {
			fmt.Println("Launch cancelled by user.")
			return nil, true, nil
		}

		if input == "3" {
			decisions := make(map[string]modtracker.UserDecision, len(opts.Changes))
			fmt.Println("\nItemized Decisions (Enter 'k' to keep, 'r' to revert):")
			for i, ch := range opts.Changes {
				promptText := fmt.Sprintf("  [%d/%d] %s (%s) [K/r, default: Keep]: ", i+1, len(opts.Changes), ch.Path, ch.Type)
				fmt.Print(promptText)
				itemIn, _ := reader.ReadString('\n')
				itemIn = strings.TrimSpace(itemIn)
				if strings.EqualFold(itemIn, "r") || strings.EqualFold(itemIn, "revert") {
					decisions[ch.Path] = modtracker.DecisionRevert
				} else {
					decisions[ch.Path] = modtracker.DecisionKeep
				}
			}
			return decisions, false, nil
		}

		fmt.Println("Please enter 1, 2, 3, 4, or 'q' to cancel.")
	}
}

type promptResponse struct {
	Decisions map[string]modtracker.UserDecision
	Cancelled bool
}

func promptGUI(opts PromptOptions) (map[string]modtracker.UserDecision, bool, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, false, fmt.Errorf("failed to bind prompt listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	opts.Logger("[Lunaris] Launching mod change review window: %s", url)

	resultChan := make(chan promptResponse, 1)
	var closeOnce sync.Once

	mux := http.NewServeMux()

	// Serve HTML
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		_, _ = w.Write([]byte(PromptHTML))
	})

	// API: Get Changes
	mux.HandleFunc("/api/changes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance_name": opts.InstanceName,
			"instance_dir":  opts.InstanceDir,
			"changes":       opts.Changes,
		})
	})

	// API: Submit Decisions
	mux.HandleFunc("/api/decisions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Decisions map[string]modtracker.UserDecision `json:"decisions"`
			Cancel    bool                               `json:"cancel"`
			ResetAll  bool                               `json:"reset_all"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if payload.ResetAll {
			_ = modtracker.ResetToPack(opts.InstanceDir)
			for _, ch := range opts.Changes {
				if payload.Decisions == nil {
					payload.Decisions = make(map[string]modtracker.UserDecision)
				}
				payload.Decisions[ch.Path] = modtracker.DecisionRevert
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		closeOnce.Do(func() {
			resultChan <- promptResponse{
				Decisions: payload.Decisions,
				Cancelled: payload.Cancel,
			}
		})
	})

	srv := &http.Server{Handler: mux}

	go func() {
		_ = srv.Serve(listener)
	}()

	// Open standalone browser window
	go func() {
		time.Sleep(100 * time.Millisecond)
		if err := OpenBrowser(url); err != nil {
			_ = OpenDefaultBrowser(url)
		}
	}()

	// Wait indefinitely for user's decision (as requested)
	resp := <-resultChan

	// Clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	return resp.Decisions, resp.Cancelled, nil
}

// PromptHTML is the embedded single-page application for reviewing detected mod changes.
const PromptHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>Modpack Modifications Detected | Lunaris</title>
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
            --code-bg: #121216;
            --primary: #4338ca;
            --primary-hover: #4f46e5;
            --success: #4ade80;
            --success-bg: rgba(34, 197, 94, 0.12);
            --warning: #fbbf24;
            --warning-bg: rgba(251, 191, 36, 0.12);
            --danger: #f87171;
            --danger-bg: rgba(248, 113, 113, 0.12);
            --added: #a5b4fc;
            --added-bg: rgba(99, 102, 241, 0.12);
        }

        * { box-sizing: border-box; }

        body {
            margin: 0;
            background: var(--bg);
            color: var(--text-main);
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            -webkit-font-smoothing: antialiased;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }

        .navbar {
            background: var(--nav-bg);
            border-bottom: 1px solid var(--card-border);
            padding: 12px 24px;
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
            gap: 10px;
            color: var(--text-heading);
            font-weight: 700;
        }

        .container {
            max-width: 820px;
            width: 100%;
            margin: 0 auto;
            padding: 28px 20px 48px;
            flex: 1;
        }

        .header-section {
            margin-bottom: 24px;
        }

        h1 {
            color: var(--text-heading);
            font-size: 1.8rem;
            margin: 0 0 6px 0;
            font-weight: 700;
            letter-spacing: -0.02em;
        }

        .subtitle {
            color: var(--text-muted);
            font-size: 0.92rem;
            margin: 0;
            line-height: 1.5;
        }

        /* Bulk Bar */
        .bulk-bar {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 14px 18px;
            margin-bottom: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 12px;
        }

        .bulk-info {
            font-size: 0.9rem;
            color: var(--text-heading);
            font-weight: 600;
        }

        .bulk-actions {
            display: flex;
            gap: 10px;
        }

        button {
            border-radius: 6px;
            font-size: 0.85rem;
            font-weight: 600;
            padding: 8px 14px;
            cursor: pointer;
            transition: all 0.2s ease;
            border: 1px solid transparent;
        }

        .btn-bulk {
            background: #181822;
            color: var(--text-main);
            border-color: var(--card-border);
        }
        .btn-bulk:hover {
            background: #252535;
            border-color: #444458;
            color: #ffffff;
        }

        /* Changes List */
        .changes-list {
            display: flex;
            flex-direction: column;
            gap: 12px;
            margin-bottom: 28px;
        }

        .change-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 16px;
            transition: border-color 0.2s;
        }

        .change-card:hover {
            border-color: #383848;
        }

        .change-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 12px;
            flex-wrap: wrap;
            gap: 10px;
        }

        .change-title-area {
            display: flex;
            align-items: center;
            gap: 10px;
            min-width: 0;
            flex: 1;
        }

        .badge {
            font-size: 0.72rem;
            padding: 3px 8px;
            border-radius: 4px;
            font-weight: 700;
            letter-spacing: 0.04em;
            text-transform: uppercase;
            flex-shrink: 0;
        }

        .badge-added { background: var(--added-bg); color: var(--added); border: 1px solid rgba(165, 180, 252, 0.3); }
        .badge-removed { background: var(--danger-bg); color: var(--danger); border: 1px solid rgba(248, 113, 113, 0.3); }
        .badge-modified { background: var(--warning-bg); color: var(--warning); border: 1px solid rgba(251, 191, 36, 0.3); }

        .change-path {
            font-family: Menlo, Monaco, Consolas, monospace;
            font-size: 0.88rem;
            color: var(--text-heading);
            font-weight: 600;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .change-size {
            font-size: 0.8rem;
            color: var(--text-muted);
        }

        .decision-selector {
            display: flex;
            gap: 10px;
            background: #111118;
            border: 1px solid var(--card-border);
            padding: 4px;
            border-radius: 6px;
        }

        .decision-opt {
            padding: 6px 14px;
            font-size: 0.82rem;
            font-weight: 600;
            border-radius: 4px;
            cursor: pointer;
            color: var(--text-muted);
            transition: all 0.2s;
            user-select: none;
        }

        .decision-opt:hover {
            color: var(--text-main);
        }

        .decision-opt.active-keep {
            background: #232238;
            color: #c7d2fe;
            box-shadow: 0 0 10px rgba(99, 102, 241, 0.15);
        }

        .decision-opt.active-revert {
            background: #2d1818;
            color: #fca5a5;
        }

        /* Footer Controls */
        .footer-controls {
            border-top: 1px solid var(--card-border);
            padding-top: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 14px;
        }

        .reset-link {
            font-size: 0.84rem;
            color: var(--text-muted);
            cursor: pointer;
            text-decoration: underline;
        }
        .reset-link:hover {
            color: #fca5a5;
        }

        .right-buttons {
            display: flex;
            gap: 12px;
        }

        .btn-cancel {
            background: transparent;
            color: var(--text-muted);
            border: 1px solid var(--card-border);
        }
        .btn-cancel:hover {
            color: #ffffff;
            border-color: #555;
        }

        .btn-primary {
            background: var(--primary);
            color: #ffffff;
            border: 1px solid #4f46e5;
            padding: 10px 22px;
            font-size: 0.92rem;
            box-shadow: 0 0 16px rgba(79, 70, 229, 0.25);
        }
        .btn-primary:hover {
            background: var(--primary-hover);
            box-shadow: 0 0 20px rgba(79, 70, 229, 0.4);
        }
    </style>
</head>
<body>
    <div class="navbar">
        <div class="nav-brand">
            <span>🌙 Lunaris Synchronizer</span>
        </div>
        <div style="color: var(--text-muted); font-size: 0.8rem;" id="instance-title">Instance Check</div>
    </div>

    <div class="container">
        <div class="header-section">
            <h1>Modpack Modifications Detected</h1>
            <p class="subtitle">
                Local changes were detected in your modpack files between launches.
                You can choose to keep your custom client mods/edits on top of the pack, or revert them to match the official server modpack.
            </p>
        </div>

        <div class="bulk-bar">
            <div class="bulk-info" id="changes-count-text">Scanning changes...</div>
            <div class="bulk-actions">
                <button class="btn-bulk" onclick="setAll('keep')">✔ Keep All Changes</button>
                <button class="btn-bulk" onclick="setAll('revert')">↺ Revert All to Server Pack</button>
            </div>
        </div>

        <div class="changes-list" id="changes-list">
            <!-- Rendered by JS -->
        </div>

        <div class="footer-controls">
            <div class="reset-link" onclick="resetAllModifications()">
                ↺ Resync & forget all custom modifications (Match Server 100%)
            </div>
            <div class="right-buttons">
                <button class="btn-cancel" onclick="cancelLaunch()">Cancel Launch</button>
                <button class="btn-primary" id="launch-btn" onclick="submitDecisions()">Proceed & Launch Minecraft →</button>
            </div>
        </div>
    </div>

    <script>
        let changesData = [];
        let decisions = {};

        async function init() {
            try {
                const res = await fetch('/api/changes');
                const data = await res.json();
                changesData = data.changes || [];
                if (data.instance_name) {
                    document.getElementById('instance-title').textContent = data.instance_name;
                }
                document.getElementById('changes-count-text').textContent = 
                    changesData.length + " modified or custom item(s) detected";
                
                // Default all decisions to 'keep'
                changesData.forEach(c => {
                    decisions[c.path] = 'keep';
                });

                renderChanges();
            } catch (err) {
                console.error("Failed to load changes:", err);
            }
        }

        function renderChanges() {
            const container = document.getElementById('changes-list');
            container.innerHTML = '';

            changesData.forEach((ch, idx) => {
                const card = document.createElement('div');
                card.className = 'change-card';

                let badgeClass = 'badge-added';
                let badgeText = '+ Added Mod';
                let keepLabel = 'Keep this mod';
                let revertLabel = 'Remove / Discard';

                if (ch.type === 'removed') {
                    badgeClass = 'badge-removed';
                    badgeText = '- Removed Pack Mod';
                    keepLabel = 'Keep removed';
                    revertLabel = 'Re-download from server';
                } else if (ch.type === 'modified') {
                    badgeClass = 'badge-modified';
                    badgeText = '~ Modified Locally';
                    keepLabel = 'Keep local version';
                    revertLabel = 'Revert to server version';
                }

                const currentDecision = decisions[ch.path] || 'keep';

                card.innerHTML = 
                    '<div class="change-header">' +
                        '<div class="change-title-area">' +
                            '<span class="badge ' + badgeClass + '">' + badgeText + '</span>' +
                            '<span class="change-path" title="' + ch.path + '">' + ch.path + '</span>' +
                        '</div>' +
                        '<div class="decision-selector">' +
                            '<div class="decision-opt ' + (currentDecision === 'keep' ? 'active-keep' : '') + '" ' +
                                 'onclick="setDecision(\'' + ch.path + '\', \'keep\')">' +
                                 '● ' + keepLabel +
                            '</div>' +
                            '<div class="decision-opt ' + (currentDecision === 'revert' ? 'active-revert' : '') + '" ' +
                                 'onclick="setDecision(\'' + ch.path + '\', \'revert\')">' +
                                 revertLabel +
                            '</div>' +
                        '</div>' +
                    '</div>';
                container.appendChild(card);
            });
        }

        function setDecision(path, decision) {
            decisions[path] = decision;
            renderChanges();
        }

        function setAll(decision) {
            changesData.forEach(c => {
                decisions[c.path] = decision;
            });
            renderChanges();
        }

        async function submitDecisions() {
            const btn = document.getElementById('launch-btn');
            btn.disabled = true;
            btn.textContent = 'Saving preferences & starting Minecraft...';

            try {
                await fetch('/api/decisions', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ decisions: decisions, cancel: false })
                });
                // Attempt to close window
                setTimeout(() => {
                    window.close();
                }, 400);
            } catch (err) {
                alert('Error submitting decisions: ' + err);
                btn.disabled = false;
                btn.textContent = 'Proceed & Launch Minecraft →';
            }
        }

        async function resetAllModifications() {
            if (!confirm("Are you sure you want to forget all custom modifications and resync 100% with the server?")) {
                return;
            }
            const btn = document.getElementById('launch-btn');
            btn.disabled = true;
            btn.textContent = 'Resetting to official server pack...';

            try {
                await fetch('/api/decisions', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ reset_all: true, cancel: false })
                });
                setTimeout(() => {
                    window.close();
                }, 400);
            } catch (err) {
                alert('Error resetting modifications: ' + err);
                btn.disabled = false;
            }
        }

        async function cancelLaunch() {
            try {
                await fetch('/api/decisions', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ cancel: true })
                });
                window.close();
            } catch (err) {
                window.close();
            }
        }

        window.addEventListener('DOMContentLoaded', init);
    </script>
</body>
</html>`
