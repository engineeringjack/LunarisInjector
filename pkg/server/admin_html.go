package server

const AdminHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <title>Lunaris Server Admin | Jack Frederick</title>
    <style>
        :root {
            --bg: #000000;
            --card-bg: #0d0d12;
            --card-border: #212128;
            --card-border-hover: #383845;
            --text-main: rgb(212, 214, 221);
            --text-muted: rgb(140, 140, 148);
            --text-heading: aliceblue;
            --nav-bg: #141418;
            --code-bg: #121216;
            --hover: #ffffff;
            --accent: #6366f1;
            --accent-hover: #4f46e5;
            --green: #10b981;
            --amber: #f59e0b;
            --red: #ef4444;
            --purple: #8b5cf6;
        }

        * { box-sizing: border-box; }

        body {
            margin: 0;
            background: var(--bg);
            color: var(--text-main);
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            -webkit-font-smoothing: antialiased;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }

        /* Navbar */
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

        .nav-badge {
            font-size: 0.72rem;
            background: #232330;
            border: 1px solid #3c3c4e;
            color: #a5b4fc;
            padding: 2px 8px;
            border-radius: 4px;
            font-weight: 600;
        }

        .nav-actions {
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .nav-actions a {
            color: var(--text-muted);
            text-decoration: none;
            transition: color 0.2s;
        }

        .nav-actions a:hover {
            color: var(--hover);
        }

        /* Container */
        .container {
            max-width: 1200px;
            width: 100%;
            margin: 0 auto;
            padding: 28px 20px;
            flex: 1;
        }

        /* Failure Alert Banner */
        .boot-fail-banner {
            display: none;
            background: linear-gradient(90deg, rgba(239, 68, 68, 0.15), rgba(239, 68, 68, 0.05));
            border: 1px solid #ef4444;
            border-radius: 8px;
            padding: 16px 20px;
            margin-bottom: 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 14px;
            animation: pulse-red 2s infinite;
        }

        @keyframes pulse-red {
            0% { border-color: #ef4444; }
            50% { border-color: #b91c1c; }
            100% { border-color: #ef4444; }
        }

        .boot-fail-info {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .boot-fail-icon {
            font-size: 1.6rem;
        }

        .boot-fail-text h3 {
            margin: 0 0 4px 0;
            color: #fca5a5;
            font-size: 1.05rem;
        }

        .boot-fail-text p {
            margin: 0;
            font-size: 0.85rem;
            color: var(--text-muted);
        }

        .boot-fail-actions {
            display: flex;
            gap: 10px;
        }

        /* Top Bar */
        .header-bar {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 24px;
            flex-wrap: wrap;
            gap: 16px;
        }

        .header-title h1 {
            color: var(--text-heading);
            font-size: 1.8rem;
            margin: 0 0 6px 0;
            font-weight: 700;
        }

        .header-title p {
            color: var(--text-muted);
            margin: 0;
            font-size: 0.9rem;
        }

        .header-controls {
            display: flex;
            gap: 10px;
            align-items: center;
            flex-wrap: wrap;
        }

        /* Status Pills */
        .status-pill {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            padding: 6px 14px;
            border-radius: 20px;
            font-size: 0.82rem;
            font-weight: 600;
            background: #14141c;
            border: 1px solid var(--card-border);
        }

        .dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            display: inline-block;
        }

        .dot-green { background: var(--green); box-shadow: 0 0 8px var(--green); }
        .dot-amber { background: var(--amber); box-shadow: 0 0 8px var(--amber); }
        .dot-red { background: var(--red); box-shadow: 0 0 8px var(--red); }
        .dot-purple { background: var(--purple); box-shadow: 0 0 8px var(--purple); }

        /* Buttons */
        .btn {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            background: #191922;
            color: var(--text-main);
            border: 1px solid var(--card-border);
            padding: 8px 16px;
            border-radius: 6px;
            font-size: 0.86rem;
            font-weight: 500;
            cursor: pointer;
            text-decoration: none;
            transition: all 0.2s ease;
        }

        .btn:hover {
            background: #252530;
            color: var(--hover);
            border-color: var(--card-border-hover);
        }

        .btn-primary {
            background: #4f46e5;
            color: #ffffff;
            border-color: #6366f1;
            font-weight: 600;
        }

        .btn-primary:hover {
            background: #4338ca;
            border-color: #818cf8;
        }

        .btn-danger {
            background: #991b1b;
            color: #ffffff;
            border-color: #ef4444;
        }

        .btn-danger:hover {
            background: #7f1d1d;
        }

        .btn-sm {
            padding: 4px 10px;
            font-size: 0.78rem;
        }

        /* Tabs */
        .tabs {
            display: flex;
            gap: 8px;
            border-bottom: 1px solid var(--card-border);
            margin-bottom: 24px;
        }

        .tab-btn {
            background: transparent;
            color: var(--text-muted);
            border: none;
            padding: 12px 18px;
            font-size: 0.9rem;
            font-weight: 500;
            cursor: pointer;
            border-bottom: 2px solid transparent;
            margin-bottom: -1px;
            transition: all 0.2s;
        }

        .tab-btn:hover {
            color: var(--hover);
        }

        .tab-btn.active {
            color: var(--text-heading);
            border-bottom-color: #6366f1;
            font-weight: 600;
        }

        /* Drop Zone */
        .dropzone {
            background: var(--card-bg);
            border: 2px dashed #2e2e3a;
            border-radius: 10px;
            padding: 32px 20px;
            text-align: center;
            margin-bottom: 24px;
            transition: all 0.2s;
            cursor: pointer;
        }

        .dropzone:hover, .dropzone.dragover {
            border-color: #6366f1;
            background: #111119;
        }

        .dropzone-icon {
            font-size: 2.2rem;
            margin-bottom: 10px;
            color: #a5b4fc;
        }

        .dropzone-text {
            font-size: 0.95rem;
            color: var(--text-heading);
            font-weight: 600;
            margin-bottom: 4px;
        }

        .dropzone-sub {
            font-size: 0.82rem;
            color: var(--text-muted);
        }

        /* Tables */
        .table-controls {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 14px;
            flex-wrap: wrap;
            gap: 12px;
        }

        .filters-group {
            display: flex;
            gap: 6px;
        }

        .filter-btn {
            background: #14141c;
            border: 1px solid var(--card-border);
            color: var(--text-muted);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 0.8rem;
            cursor: pointer;
        }

        .filter-btn.active {
            background: #252538;
            border-color: #6366f1;
            color: #ffffff;
            font-weight: 600;
        }

        .search-box {
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            padding: 8px 14px;
            border-radius: 6px;
            font-size: 0.85rem;
            width: 260px;
            outline: none;
        }

        .search-box:focus {
            border-color: #6366f1;
        }

        .table-wrapper {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            overflow: hidden;
            margin-bottom: 30px;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            text-align: left;
        }

        th {
            background: #111116;
            color: var(--text-muted);
            font-size: 0.74rem;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            padding: 12px 16px;
            border-bottom: 1px solid var(--card-border);
        }

        td {
            padding: 12px 16px;
            font-size: 0.86rem;
            border-bottom: 1px solid #16161c;
            vertical-align: middle;
        }

        tr:last-child td {
            border-bottom: none;
        }

        tr:hover td {
            background: rgba(255, 255, 255, 0.015);
        }

        /* Badges */
        .side-badge {
            display: inline-flex;
            align-items: center;
            padding: 3px 8px;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.04em;
        }

        .badge-both {
            background: rgba(99, 102, 241, 0.15);
            border: 1px solid rgba(99, 102, 241, 0.4);
            color: #a5b4fc;
        }

        .badge-client {
            background: rgba(168, 85, 247, 0.15);
            border: 1px solid rgba(168, 85, 247, 0.4);
            color: #d8b4fe;
        }

        .badge-server {
            background: rgba(245, 158, 11, 0.15);
            border: 1px solid rgba(245, 158, 11, 0.4);
            color: #fcd34d;
        }

        .badge-disabled {
            background: rgba(239, 68, 68, 0.15);
            border: 1px solid rgba(239, 68, 68, 0.4);
            color: #fca5a5;
        }

        /* Modals */
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0, 0, 0, 0.85);
            backdrop-filter: blur(4px);
            z-index: 999;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }

        .modal-overlay.active {
            display: flex;
        }

        .modal-card {
            background: #0d0d14;
            border: 1px solid #2d2d3d;
            border-radius: 10px;
            max-width: 650px;
            width: 100%;
            max-height: 85vh;
            display: flex;
            flex-direction: column;
            box-shadow: 0 10px 40px rgba(0, 0, 0, 0.7);
        }

        .modal-header {
            padding: 18px 24px;
            border-bottom: 1px solid var(--card-border);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .modal-header h2 {
            margin: 0;
            font-size: 1.15rem;
            color: var(--text-heading);
        }

        .modal-body {
            padding: 20px 24px;
            overflow-y: auto;
            font-size: 0.9rem;
            color: var(--text-main);
            flex: 1;
        }

        .modal-footer {
            padding: 16px 24px;
            border-top: 1px solid var(--card-border);
            display: flex;
            justify-content: flex-end;
            gap: 12px;
        }

        .choice-card {
            background: #14141d;
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 16px;
            margin-bottom: 12px;
            cursor: pointer;
            transition: all 0.2s;
        }

        .choice-card:hover, .choice-card.selected {
            border-color: #6366f1;
            background: #181825;
        }

        .choice-card strong {
            display: block;
            margin-bottom: 4px;
            color: var(--text-heading);
        }

        .choice-card p {
            margin: 0;
            font-size: 0.82rem;
            color: var(--text-muted);
        }

        /* Console */
        .terminal-box {
            background: #09090d;
            border: 1px solid var(--card-border);
            border-radius: 8px;
            font-family: 'JetBrains Mono', Menlo, Monaco, Consolas, monospace;
            font-size: 0.82rem;
            color: #d1d5db;
            display: flex;
            flex-direction: column;
            height: 480px;
            overflow: hidden;
        }

        .terminal-output {
            flex: 1;
            padding: 14px;
            overflow-y: auto;
            white-space: pre-wrap;
            word-break: break-all;
            line-height: 1.45;
        }

        .terminal-bar {
            border-top: 1px solid var(--card-border);
            padding: 10px 14px;
            display: flex;
            gap: 10px;
            background: #0f0f15;
        }

        .terminal-input {
            flex: 1;
            background: #161620;
            border: 1px solid #282835;
            color: #fff;
            padding: 8px 12px;
            border-radius: 4px;
            font-family: inherit;
            font-size: 0.85rem;
            outline: none;
        }

        /* Forms */
        .form-group {
            margin-bottom: 18px;
        }

        .form-group label {
            display: block;
            margin-bottom: 6px;
            font-size: 0.85rem;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .form-control {
            width: 100%;
            background: var(--code-bg);
            border: 1px solid var(--card-border);
            color: var(--text-heading);
            padding: 10px 14px;
            border-radius: 6px;
            font-size: 0.9rem;
            outline: none;
        }

        .form-control:focus {
            border-color: #6366f1;
        }

        /* Card grid */
        .grid-2 {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }

        .panel-card {
            background: var(--card-bg);
            border: 1px solid var(--card-border);
            border-radius: 8px;
            padding: 22px;
        }

        .panel-card h3 {
            margin: 0 0 14px 0;
            font-size: 1.1rem;
            color: var(--text-heading);
        }

        /* Toast */
        .toast {
            position: fixed;
            bottom: 24px;
            right: 24px;
            background: #1e1e28;
            border: 1px solid #4f46e5;
            color: #fff;
            padding: 12px 20px;
            border-radius: 6px;
            box-shadow: 0 8px 24px rgba(0,0,0,0.5);
            font-size: 0.88rem;
            display: none;
            z-index: 9999;
        }
    </style>
</head>
<body>

    <nav class="navbar">
        <div class="nav-brand">
            <a href="/">LUNARIS SYNC</a>
            <span class="nav-badge">ADMIN PORTAL</span>
        </div>
        <div class="nav-actions">
            <a href="/">Sync Dashboard</a>
            <span id="navUsername" style="color:var(--text-heading); font-weight:600; display:none;"></span>
            <button class="btn btn-sm" id="logoutBtn" style="display:none;" onclick="logout()">Log Out</button>
        </div>
    </nav>

    <!-- AUTH CONTAINER (Login & Setup) -->
    <div id="authView" class="container" style="max-width: 440px; margin-top: 60px;">
        <div class="panel-card" id="setupCard" style="display:none;">
            <h3>Setup Admin Account</h3>
            <p style="color:var(--text-muted); font-size:0.86rem; margin-bottom: 20px;">
                Choose an encrypted username and password for remote administration and mod synchronization.
            </p>
            <div class="form-group">
                <label>Admin Username</label>
                <input type="text" id="setupUsername" class="form-control" placeholder="admin" autofocus/>
            </div>
            <div class="form-group">
                <label>Admin Password</label>
                <input type="password" id="setupPassword" class="form-control" placeholder="••••••••"/>
            </div>
            <button class="btn btn-primary" style="width:100%;" onclick="submitSetup()">Save & Enter Admin</button>
        </div>

        <div class="panel-card" id="loginCard" style="display:none;">
            <h3>Admin Login</h3>
            <p style="color:var(--text-muted); font-size:0.86rem; margin-bottom: 20px;">
                Authenticate to manage mods, sync files, and control the Minecraft server.
            </p>
            <div id="loginError" style="color:#ef4444; font-size:0.85rem; margin-bottom:12px; display:none;"></div>
            <div class="form-group">
                <label>Username</label>
                <input type="text" id="loginUsername" class="form-control" placeholder="admin" autofocus/>
            </div>
            <div class="form-group">
                <label>Password</label>
                <input type="password" id="loginPassword" class="form-control" placeholder="••••••••"/>
            </div>
            <button class="btn btn-primary" style="width:100%;" onclick="submitLogin()">Log In</button>
        </div>
    </div>

    <!-- MAIN ADMIN DASHBOARD -->
    <div id="dashboardView" class="container" style="display:none;">

        <!-- BOOT FAILURE ALERT BANNER -->
        <div id="bootFailBanner" class="boot-fail-banner">
            <div class="boot-fail-info">
                <div class="boot-fail-icon">⚠️</div>
                <div class="boot-fail-text">
                    <h3>Minecraft Server Failed to Boot</h3>
                    <p id="bootFailReason">The server process stopped unexpectedly during startup.</p>
                </div>
            </div>
            <div class="boot-fail-actions">
                <button class="btn btn-primary" onclick="copyLatestLog()">📋 Copy Latest Log for Debugging</button>
                <button class="btn" onclick="viewLatestLogModal()">View Log</button>
                <button class="btn btn-sm" onclick="dismissBootFail()">Dismiss</button>
            </div>
        </div>

        <div class="header-bar">
            <div class="header-title">
                <h1>Server Management</h1>
                <p>Add, remove, or modify mods on the fly with live sync and process management.</p>
            </div>
            <div class="header-controls">
                <div id="statusPill" class="status-pill">
                    <span class="dot dot-amber"></span>
                    <span id="statusText">Checking status...</span>
                </div>
                <div class="status-pill" id="portPill">
                    <span class="dot dot-green"></span>
                    <span id="portText">Port: 8080 (Open)</span>
                </div>
                <button class="btn btn-primary" onclick="restartServer()">🔄 Restart Server</button>
                <button class="btn" id="toggleServerBtn" onclick="toggleServerState()">Start Server</button>
                <button class="btn" onclick="rescanSyncFiles()">⚡ Rescan</button>
            </div>
        </div>

        <div class="tabs">
            <button class="tab-btn active" onclick="switchTab('mods')">Mods (<span id="modCount">0</span>)</button>
            <button class="tab-btn" onclick="switchTab('console')">Live Console</button>
            <button class="tab-btn" onclick="switchTab('network')">Network & Port</button>
            <button class="tab-btn" onclick="switchTab('textfiles')">Text Files</button>
        </div>

        <!-- TAB: MODS -->
        <div id="tab-mods">
            <!-- DRAG AND DROP ZONE -->
            <div class="dropzone" id="dropZone">
                <div class="dropzone-icon">📥</div>
                <div class="dropzone-text">Drag and drop mod files (.jar) or an entire mod folder here</div>
                <div class="dropzone-sub">Dropping a folder will prompt to remove missing mods or just add</div>
                <div style="margin-top: 14px;">
                    <button class="btn btn-sm btn-primary" onclick="triggerFileInput()">Select Files</button>
                    <button class="btn btn-sm" onclick="triggerFolderInput()">Select Folder</button>
                </div>
                <input type="file" id="fileInput" multiple accept=".jar,.zip" style="display:none;" onchange="handleFileSelect(event)"/>
                <input type="file" id="folderInput" webkitdirectory directory style="display:none;" onchange="handleFolderSelect(event)"/>
            </div>

            <div class="table-controls">
                <div class="filters-group">
                    <button class="filter-btn active" onclick="setModFilter('all', this)">All</button>
                    <button class="filter-btn" onclick="setModFilter('both', this)">Both (Client & Server)</button>
                    <button class="filter-btn" onclick="setModFilter('client_only', this)">Client Only</button>
                    <button class="filter-btn" onclick="setModFilter('server_only', this)">Server Only</button>
                    <button class="filter-btn" onclick="setModFilter('disabled', this)">Disabled</button>
                </div>
                <input type="text" id="modSearch" class="search-box" placeholder="Filter mods by name..." onkeyup="filterModsTable()"/>
            </div>

            <div class="table-wrapper">
                <table id="modsTable">
                    <thead>
                        <tr>
                            <th>Mod File Name</th>
                            <th style="width: 170px;">Side Assignment</th>
                            <th style="width: 110px;">Size</th>
                            <th style="width: 220px; text-align: right;">Actions</th>
                        </tr>
                    </thead>
                    <tbody id="modsTableBody">
                        <tr><td colspan="4" style="text-align:center; color:var(--text-muted);">Loading mods...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <!-- TAB: CONSOLE -->
        <div id="tab-console" style="display:none;">
            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:12px;">
                <span style="font-size:0.9rem; color:var(--text-muted);">Real-time server output & command terminal</span>
                <div style="display:flex; gap:8px;">
                    <button class="btn btn-sm" onclick="copyLatestLog()">📋 Copy Latest Log</button>
                    <button class="btn btn-sm" onclick="clearConsoleView()">Clear View</button>
                </div>
            </div>
            <div class="terminal-box">
                <div class="terminal-output" id="terminalOutput"></div>
                <div class="terminal-bar">
                    <input type="text" id="commandInput" class="terminal-input" placeholder="Type Minecraft command (e.g. say Hello, op player, list)..." onkeydown="handleCommandKey(event)"/>
                    <button class="btn btn-sm btn-primary" onclick="sendConsoleCommand()">Send</button>
                </div>
            </div>
        </div>

        <!-- TAB: NETWORK & PORT -->
        <div id="tab-network" style="display:none;">
            <div class="grid-2">
                <div class="panel-card">
                    <h3>Port & Remote Access</h3>
                    <p style="font-size:0.86rem; color:var(--text-muted); margin-bottom: 16px;">
                        The server listens on all interfaces (<code>0.0.0.0</code>) so players and remote admins can connect once the port is open.
                    </p>
                    <div class="form-group">
                        <label>Bound Port</label>
                        <input type="text" id="netPort" class="form-control" readonly/>
                    </div>
                    <div class="form-group">
                        <label>Local Network Access URLs</label>
                        <div id="localUrls" style="font-family:monospace; font-size:0.85rem; line-height: 1.6; color:#a5b4fc;"></div>
                    </div>
                    <div style="background:#14141d; padding:12px; border-radius:6px; font-size:0.82rem; color:var(--text-muted); line-height:1.5;">
                        <strong>Opening your port:</strong><br/>
                        • Linux: <code>sudo ufw allow <span id="portDisplayUfw">8080</span>/tcp</code><br/>
                        • Router: Forward TCP port <span id="portDisplayRouter">8080</span> to your server machine's local IP address.
                    </div>
                </div>

                <div class="panel-card">
                    <h3>Minecraft Server Execution</h3>
                    <p style="font-size:0.86rem; color:var(--text-muted); margin-bottom: 16px;">
                        Configure the launch command or startup script used when starting or restarting the server.
                    </p>
                    <div class="form-group">
                        <label>Start Command / Script</label>
                        <input type="text" id="startCommandInput" class="form-control" placeholder="./run.sh"/>
                    </div>
                    <button class="btn btn-primary" onclick="saveStartCommand()">Save Start Command</button>
                </div>
            </div>
        </div>

        <!-- TAB: TEXT FILES -->
        <div id="tab-textfiles" style="display:none;">
            <div class="panel-card" style="margin-bottom: 24px;">
                <h3>Persistent Side Classification Files</h3>
                <p style="font-size:0.88rem; color:var(--text-muted); margin-bottom: 20px;">
                    Mods placed in <strong>client-only</strong> or <strong>server-only</strong> are listed here (just filenames, no hashes).
                    Anytime these mods are added, removed, or dragged in, Lunaris remembers their classification and automatically keeps them in the right folder.
                </p>
                <div class="grid-2">
                    <div>
                        <label style="font-weight:600; font-size:0.85rem; color:#d8b4fe;">client-only-mods.txt</label>
                        <textarea id="clientOnlyText" class="form-control" style="height:260px; font-family:monospace; margin-top:6px;"></textarea>
                    </div>
                    <div>
                        <label style="font-weight:600; font-size:0.85rem; color:#fcd34d;">server-only-mods.txt</label>
                        <textarea id="serverOnlyText" class="form-control" style="height:260px; font-family:monospace; margin-top:6px;"></textarea>
                    </div>
                </div>
                <div style="margin-top: 16px;">
                    <button class="btn btn-primary" onclick="saveTextFiles()">Save Text Files</button>
                </div>
            </div>
        </div>

    </div>

    <!-- MODAL: FOLDER DROP OPTION (Remove missing vs Just add) -->
    <div id="folderDropModal" class="modal-overlay">
        <div class="modal-card">
            <div class="modal-header">
                <h2>Mod Folder Dropped</h2>
                <button class="btn btn-sm" onclick="closeModal('folderDropModal')">✕</button>
            </div>
            <div class="modal-body">
                <p>You dropped a folder containing <strong id="droppedFileCount">0</strong> mod(s). How would you like to handle existing server mods?</p>
                <div class="choice-card selected" id="choiceRemoveMissing" onclick="selectDropChoice(true)">
                    <strong>1. Remove all missing (Full Sync / Replace)</strong>
                    <p>Deletes any mod currently on the server that is NOT in your dropped folder, matching your server exactly to your local folder.</p>
                </div>
                <div class="choice-card" id="choiceJustAdd" onclick="selectDropChoice(false)">
                    <strong>2. Just add (Additive)</strong>
                    <p>Adds or updates the dropped mods without deleting any existing server mods.</p>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="closeModal('folderDropModal')">Cancel</button>
                <button class="btn btn-primary" onclick="proceedToClassify()">Next: Review Sides</button>
            </div>
        </div>
    </div>

    <!-- MODAL: MOD CLASSIFICATION & CONFIRMATION -->
    <div id="classifyModal" class="modal-overlay">
        <div class="modal-card" style="max-width: 750px;">
            <div class="modal-header">
                <h2>Review & Classify Mods</h2>
                <button class="btn btn-sm" onclick="closeModal('classifyModal')">✕</button>
            </div>
            <div class="modal-body">
                <p style="margin-top:0; color:var(--text-muted); font-size:0.86rem;">
                    Default side is <strong>Both (Client & Server)</strong>. Files matching <code>client-only-mods.txt</code> or <code>server-only-mods.txt</code> are automatically pre-selected.
                </p>
                <div style="max-height: 380px; overflow-y:auto; border:1px solid var(--card-border); border-radius:6px;">
                    <table style="width:100%;">
                        <thead>
                            <tr>
                                <th>File Name</th>
                                <th style="width: 100px;">Size</th>
                                <th style="width: 200px;">Side (Default: Both)</th>
                            </tr>
                        </thead>
                        <tbody id="classifyTableBody"></tbody>
                    </table>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="closeModal('classifyModal')">Cancel</button>
                <button class="btn btn-primary" id="confirmUploadBtn" onclick="executeUpload()">Confirm & Upload</button>
            </div>
        </div>
    </div>

    <!-- MODAL: VIEW LATEST LOG -->
    <div id="logViewModal" class="modal-overlay">
        <div class="modal-card" style="max-width: 850px;">
            <div class="modal-header">
                <h2 id="logViewTitle">Server Debug Log</h2>
                <button class="btn btn-sm" onclick="closeModal('logViewModal')">✕</button>
            </div>
            <div class="modal-body">
                <pre id="logViewContent" style="background:#09090d; border:1px solid var(--card-border); border-radius:6px; padding:12px; font-family:monospace; font-size:0.8rem; height:450px; overflow-y:auto; white-space:pre-wrap; word-break:break-all;"></pre>
            </div>
            <div class="modal-footer">
                <button class="btn btn-primary" onclick="copyLatestLog()">📋 Copy to Clipboard</button>
                <button class="btn" onclick="closeModal('logViewModal')">Close</button>
            </div>
        </div>
    </div>

    <!-- FLOATING TOAST -->
    <div id="toast" class="toast"></div>

    <script>
        let currentTab = 'mods';
        let currentFilter = 'all';
        let allMods = [];
        let clientOnlyNames = new Set();
        let serverOnlyNames = new Set();
        let pendingUploadFiles = [];
        let dropRemoveMissing = true;
        let consolePollInterval = null;
        let statusPollInterval = null;

        // Initialize on load
        window.addEventListener('DOMContentLoaded', () => {
            checkAuth();
            setupDropZone();
        });

        async function checkAuth() {
            try {
                const res = await fetch('/api/admin/auth/status');
                const data = await res.json();
                if (!data.configured) {
                    showView('auth');
                    document.getElementById('setupCard').style.display = 'block';
                    document.getElementById('loginCard').style.display = 'none';
                } else if (!data.authenticated) {
                    showView('auth');
                    document.getElementById('setupCard').style.display = 'none';
                    document.getElementById('loginCard').style.display = 'block';
                } else {
                    showView('dashboard');
                    document.getElementById('navUsername').textContent = data.username;
                    document.getElementById('navUsername').style.display = 'inline';
                    document.getElementById('logoutBtn').style.display = 'inline';
                    loadDashboard();
                }
            } catch (err) {
                console.error(err);
            }
        }

        function showView(view) {
            document.getElementById('authView').style.display = view === 'auth' ? 'block' : 'none';
            document.getElementById('dashboardView').style.display = view === 'dashboard' ? 'block' : 'none';
        }

        async function submitSetup() {
            const u = document.getElementById('setupUsername').value.trim();
            const p = document.getElementById('setupPassword').value;
            if (!u || !p) return showToast('Please enter username and password');

            const res = await fetch('/api/admin/auth/setup', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: u, password: p })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Admin account initialized!');
                checkAuth();
            } else {
                showToast('Error: ' + data.error);
            }
        }

        async function submitLogin() {
            const u = document.getElementById('loginUsername').value.trim();
            const p = document.getElementById('loginPassword').value;
            const errDiv = document.getElementById('loginError');
            errDiv.style.display = 'none';

            const res = await fetch('/api/admin/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: u, password: p })
            });
            const data = await res.json();
            if (data.success) {
                checkAuth();
            } else {
                errDiv.textContent = data.error || 'Login failed';
                errDiv.style.display = 'block';
            }
        }

        async function logout() {
            await fetch('/api/admin/auth/logout', { method: 'POST' });
            location.reload();
        }

        function loadDashboard() {
            loadMods();
            loadServerStatus();
            loadNetworkInfo();
            loadTextFiles();

            if (!statusPollInterval) {
                statusPollInterval = setInterval(loadServerStatus, 2500);
            }
            if (!consolePollInterval) {
                consolePollInterval = setInterval(pollConsoleLogs, 2000);
            }
        }

        function switchTab(tab) {
            currentTab = tab;
            document.querySelectorAll('.tabs .tab-btn').forEach((btn, idx) => {
                const names = ['mods', 'console', 'network', 'textfiles'];
                btn.classList.toggle('active', names[idx] === tab);
            });

            document.getElementById('tab-mods').style.display = tab === 'mods' ? 'block' : 'none';
            document.getElementById('tab-console').style.display = tab === 'console' ? 'block' : 'none';
            document.getElementById('tab-network').style.display = tab === 'network' ? 'block' : 'none';
            document.getElementById('tab-textfiles').style.display = tab === 'textfiles' ? 'block' : 'none';

            if (tab === 'console') pollConsoleLogs();
        }

        // ---------------- MODS MANAGEMENT ----------------
        async function loadMods() {
            const res = await fetch('/api/admin/mods');
            const data = await res.json();
            if (data.success) {
                allMods = data.mods || [];
                clientOnlyNames = new Set((data.client_only || []).map(s => s.toLowerCase()));
                serverOnlyNames = new Set((data.server_only || []).map(s => s.toLowerCase()));
                document.getElementById('modCount').textContent = allMods.length;
                renderModsTable();
            }
        }

        function setModFilter(filter, el) {
            currentFilter = filter;
            document.querySelectorAll('.filters-group .filter-btn').forEach(b => b.classList.remove('active'));
            el.classList.add('active');
            renderModsTable();
        }

        function filterModsTable() {
            renderModsTable();
        }

        function renderModsTable() {
            const query = (document.getElementById('modSearch').value || '').toLowerCase();
            const tbody = document.getElementById('modsTableBody');
            tbody.innerHTML = '';

            const filtered = allMods.filter(m => {
                if (query && !m.filename.toLowerCase().includes(query)) return false;
                if (currentFilter === 'both' && m.side !== 'both') return false;
                if (currentFilter === 'client_only' && m.side !== 'client_only') return false;
                if (currentFilter === 'server_only' && m.side !== 'server_only') return false;
                if (currentFilter === 'disabled' && !m.disabled) return false;
                return true;
            });

            if (filtered.length === 0) {
                tbody.innerHTML = '<tr><td colspan="4" style="text-align:center; color:var(--text-muted); padding:30px;">No mods match your filter.</td></tr>';
                return;
            }

            filtered.forEach(m => {
                const tr = document.createElement('tr');

                let badgeClass = 'badge-both';
                let badgeLabel = 'Both';
                if (m.disabled) {
                    badgeClass = 'badge-disabled';
                    badgeLabel = 'Disabled';
                } else if (m.side === 'client_only') {
                    badgeClass = 'badge-client';
                    badgeLabel = 'Client Only';
                } else if (m.side === 'server_only') {
                    badgeClass = 'badge-server';
                    badgeLabel = 'Server Only';
                }

                const sizeKB = (m.size / 1024).toFixed(1) + ' KB';

                const disSpan = m.disabled ? '<span style="font-size:0.75rem; color:#ef4444; margin-left:6px;">(.disabled)</span>' : '';
                const selBoth = m.side === 'both' ? 'selected' : '';
                const selClient = m.side === 'client_only' ? 'selected' : '';
                const selServer = m.side === 'server_only' ? 'selected' : '';
                const disAction = m.disabled ? 'Enable' : 'Disable';

                tr.innerHTML = '<td><strong>' + m.filename + '</strong>' + disSpan + '</td>' +
                    '<td><select class="form-control" style="padding:4px 8px; font-size:0.78rem;" onchange="changeModSide(\'' + m.filename + '\', this.value)">' +
                    '<option value="both" ' + selBoth + '>Both (Client & Server)</option>' +
                    '<option value="client_only" ' + selClient + '>Client Only</option>' +
                    '<option value="server_only" ' + selServer + '>Server Only</option>' +
                    '</select></td>' +
                    '<td>' + sizeKB + '</td>' +
                    '<td style="text-align:right;">' +
                    '<button class="btn btn-sm" onclick="toggleModDisabled(\'' + m.filename + '\')">' + disAction + '</button> ' +
                    '<button class="btn btn-sm btn-danger" onclick="deleteMod(\'' + m.filename + '\')">Delete</button>' +
                    '</td>';
                tbody.appendChild(tr);
            });
        }

        async function changeModSide(filename, side) {
            const res = await fetch('/api/admin/mods/set-side', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ filename, side })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Mod updated to ' + side);
                loadMods();
            } else {
                showToast('Error: ' + data.error);
            }
        }

        async function toggleModDisabled(filename) {
            const res = await fetch('/api/admin/mods/toggle-disabled', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ filename })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Mod state toggled');
                loadMods();
            } else {
                showToast('Error: ' + data.error);
            }
        }

        async function deleteMod(filename) {
            if (!confirm('Are you sure you want to delete ' + filename + '?')) return;
            const res = await fetch('/api/admin/mods/delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ filename })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Mod deleted');
                loadMods();
            } else {
                showToast('Error: ' + data.error);
            }
        }

        // ---------------- DRAG & DROP FOLDER/FILE HANDLING ----------------
        function setupDropZone() {
            const dz = document.getElementById('dropZone');
            ['dragenter', 'dragover'].forEach(eventName => {
                dz.addEventListener(eventName, e => {
                    e.preventDefault();
                    e.stopPropagation();
                    dz.classList.add('dragover');
                }, false);
            });

            ['dragleave', 'drop'].forEach(eventName => {
                dz.addEventListener(eventName, e => {
                    e.preventDefault();
                    e.stopPropagation();
                    dz.classList.remove('dragover');
                }, false);
            });

            dz.addEventListener('drop', async e => {
                const items = e.dataTransfer.items;
                let files = [];
                let hasDirectory = false;

                if (items && items.length > 0) {
                    for (let i = 0; i < items.length; i++) {
                        const entry = items[i].webkitGetAsEntry ? items[i].webkitGetAsEntry() : null;
                        if (entry && entry.isDirectory) {
                            hasDirectory = true;
                            const folderFiles = await readDirectoryRecursively(entry);
                            files = files.concat(folderFiles);
                        } else {
                            const file = items[i].getAsFile();
                            if (file) files.push(file);
                        }
                    }
                } else if (e.dataTransfer.files) {
                    files = Array.from(e.dataTransfer.files);
                }

                processDroppedFiles(files, hasDirectory);
            });
        }

        async function readDirectoryRecursively(entry) {
            let files = [];
            if (entry.isFile) {
                const file = await new Promise((resolve) => entry.file(resolve));
                if (file.name.endsWith('.jar') || file.name.endsWith('.zip')) {
                    files.push(file);
                }
            } else if (entry.isDirectory) {
                const reader = entry.createReader();
                const entries = await new Promise((resolve) => {
                    reader.readEntries(resolve);
                });
                for (let child of entries) {
                    const subFiles = await readDirectoryRecursively(child);
                    files = files.concat(subFiles);
                }
            }
            return files;
        }

        function triggerFileInput() { document.getElementById('fileInput').click(); }
        function triggerFolderInput() { document.getElementById('folderInput').click(); }

        function handleFileSelect(e) {
            const files = Array.from(e.target.files);
            processDroppedFiles(files, false);
        }

        function handleFolderSelect(e) {
            const files = Array.from(e.target.files);
            processDroppedFiles(files, true);
        }

        function processDroppedFiles(files, isFolder) {
            pendingUploadFiles = files.filter(f => f.name.endsWith('.jar') || f.name.endsWith('.zip') || f.name.includes('.jar.disabled'));
            if (pendingUploadFiles.length === 0) {
                showToast('No valid mod files (.jar) found in dropped item.');
                return;
            }

            if (isFolder) {
                document.getElementById('droppedFileCount').textContent = pendingUploadFiles.length;
                openModal('folderDropModal');
            } else {
                dropRemoveMissing = false;
                openClassifyModal();
            }
        }

        function selectDropChoice(removeMissing) {
            dropRemoveMissing = removeMissing;
            document.getElementById('choiceRemoveMissing').classList.toggle('selected', removeMissing);
            document.getElementById('choiceJustAdd').classList.toggle('selected', !removeMissing);
        }

        function proceedToClassify() {
            closeModal('folderDropModal');
            openClassifyModal();
        }

        function openClassifyModal() {
            const tbody = document.getElementById('classifyTableBody');
            tbody.innerHTML = '';

            pendingUploadFiles.forEach((f, idx) => {
                const tr = document.createElement('tr');
                const cleanName = f.name.replace(/\.disabled$/i, '');
                const cleanLower = cleanName.toLowerCase();

                // Default is Both, unless already present in client-only or server-only lists!
                let defaultSide = 'both';
                if (clientOnlyNames.has(cleanLower)) {
                    defaultSide = 'client_only';
                } else if (serverOnlyNames.has(cleanLower)) {
                    defaultSide = 'server_only';
                }

                const sizeKB = (f.size / 1024).toFixed(1) + ' KB';

                const selB = defaultSide === 'both' ? 'selected' : '';
                const selC = defaultSide === 'client_only' ? 'selected' : '';
                const selS = defaultSide === 'server_only' ? 'selected' : '';

                tr.innerHTML = '<td><strong>' + cleanName + '</strong></td>' +
                    '<td>' + sizeKB + '</td>' +
                    '<td><select class="form-control file-side-select" data-filename="' + cleanName + '" style="padding:4px 8px; font-size:0.8rem;">' +
                    '<option value="both" ' + selB + '>Both (Client & Server) [Default]</option>' +
                    '<option value="client_only" ' + selC + '>Client Only</option>' +
                    '<option value="server_only" ' + selS + '>Server Only</option>' +
                    '</select></td>';
                tbody.appendChild(tr);
            });

            openModal('classifyModal');
        }

        async function executeUpload() {
            const btn = document.getElementById('confirmUploadBtn');
            btn.disabled = true;
            btn.textContent = 'Uploading...';

            const sidesMap = {};
            document.querySelectorAll('.file-side-select').forEach(sel => {
                sidesMap[sel.getAttribute('data-filename')] = sel.value;
            });

            const formData = new FormData();
            formData.append('remove_missing', dropRemoveMissing ? 'true' : 'false');
            formData.append('sides', JSON.stringify(sidesMap));
            pendingUploadFiles.forEach(f => {
                formData.append('files', f);
            });

            try {
                const res = await fetch('/api/admin/mods/upload', {
                    method: 'POST',
                    body: formData
                });
                const data = await res.json();
                if (data.success) {
                    closeModal('classifyModal');
                    showToast('✔ Upload complete: ' + data.added + ' added, ' + data.removed + ' removed');
                    loadMods();
                } else {
                    showToast('Upload error: ' + data.error);
                }
            } catch (err) {
                showToast('Upload failed: ' + err.message);
            } finally {
                btn.disabled = false;
                btn.textContent = 'Confirm & Upload';
            }
        }

        // ---------------- SERVER PROCESS & BOOT MONITOR ----------------
        async function loadServerStatus() {
            try {
                const res = await fetch('/api/admin/server/status');
                const data = await res.json();
                if (data.success) {
                    updateStatusDisplay(data);
                }
            } catch (err) {}
        }

        function updateStatusDisplay(data) {
            const pill = document.getElementById('statusPill');
            const txt = document.getElementById('statusText');
            const toggleBtn = document.getElementById('toggleServerBtn');
            const dot = pill.querySelector('.dot');

            dot.className = 'dot';
            if (data.status === 'running') {
                dot.classList.add('dot-green');
                txt.textContent = 'Running (PID: ' + (data.pid || 'N/A') + ')';
                toggleBtn.textContent = 'Stop Server';
                toggleBtn.className = 'btn btn-danger';
            } else if (data.status === 'starting') {
                dot.classList.add('dot-amber');
                txt.textContent = 'Starting...';
                toggleBtn.textContent = 'Stop Server';
                toggleBtn.className = 'btn';
            } else if (data.status === 'stopping') {
                dot.classList.add('dot-amber');
                txt.textContent = 'Stopping...';
            } else if (data.status === 'failed_to_boot') {
                dot.classList.add('dot-red');
                txt.textContent = 'Failed to Boot';
                toggleBtn.textContent = 'Start Server';
                toggleBtn.className = 'btn btn-primary';
            } else {
                dot.classList.add('dot-red');
                txt.textContent = 'Stopped';
                toggleBtn.textContent = 'Start Server';
                toggleBtn.className = 'btn btn-primary';
            }

            // CRITICAL REQUIREMENT: Boot Failure Alert
            const banner = document.getElementById('bootFailBanner');
            if (data.failed_boot || data.status === 'failed_to_boot') {
                banner.style.display = 'flex';
                document.getElementById('bootFailReason').textContent = data.failed_reason || 'Server process terminated unexpectedly.';
            } else {
                banner.style.display = 'none';
            }

            if (data.start_command && !document.getElementById('startCommandInput').value) {
                document.getElementById('startCommandInput').value = data.start_command;
            }
        }

        function dismissBootFail() {
            document.getElementById('bootFailBanner').style.display = 'none';
        }

        async function toggleServerState() {
            const btn = document.getElementById('toggleServerBtn');
            if (btn.textContent.includes('Stop')) {
                await fetch('/api/admin/server/stop', { method: 'POST' });
                showToast('Stopping server...');
            } else {
                await fetch('/api/admin/server/start', { method: 'POST' });
                showToast('Starting server...');
            }
            loadServerStatus();
        }

        async function restartServer() {
            showToast('Restarting server & refreshing sync manifest...');
            const res = await fetch('/api/admin/server/restart', { method: 'POST' });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Server restarting');
            } else {
                showToast('Restart error: ' + data.error);
            }
            loadServerStatus();
        }

        async function rescanSyncFiles() {
            const res = await fetch('/api/admin/server/rescan', { method: 'POST' });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Manifest refreshed (' + data.file_count + ' files tracked)');
                loadMods();
            }
        }

        async function saveStartCommand() {
            const cmd = document.getElementById('startCommandInput').value.trim();
            const res = await fetch('/api/admin/server/config', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ start_command: cmd })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Start command saved');
            }
        }

        // ---------------- LOGS & DEBUGGING ----------------
        async function copyLatestLog() {
            try {
                const res = await fetch('/api/admin/server/logs/latest');
                const data = await res.json();
                if (data.success && data.content) {
                    await navigator.clipboard.writeText(data.content);
                    showToast('✔ Latest log copied to clipboard (' + data.source + ')! Ready to paste.');
                } else {
                    showToast('No log content available');
                }
            } catch (err) {
                showToast('Failed to copy log: ' + err.message);
            }
        }

        async function viewLatestLogModal() {
            try {
                const res = await fetch('/api/admin/server/logs/latest');
                const data = await res.json();
                if (data.success) {
                    document.getElementById('logViewTitle').textContent = 'Server Log (' + data.source + ')';
                    document.getElementById('logViewContent').textContent = data.content;
                    openModal('logViewModal');
                }
            } catch (err) {}
        }

        async function pollConsoleLogs() {
            if (currentTab !== 'console') return;
            try {
                const res = await fetch('/api/admin/server/logs/console?limit=250');
                const data = await res.json();
                if (data.success && data.lines) {
                    const output = document.getElementById('terminalOutput');
                    const wasAtBottom = output.scrollHeight - output.scrollTop <= output.clientHeight + 50;
                    output.textContent = data.lines.join('\n');
                    if (wasAtBottom) {
                        output.scrollTop = output.scrollHeight;
                    }
                }
            } catch (err) {}
        }

        async function sendConsoleCommand() {
            const input = document.getElementById('commandInput');
            const cmd = input.value.trim();
            if (!cmd) return;

            input.value = '';
            await fetch('/api/admin/server/command', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ command: cmd })
            });
            pollConsoleLogs();
        }

        function handleCommandKey(e) {
            if (e.key === 'Enter') sendConsoleCommand();
        }

        async function clearConsoleView() {
            await fetch('/api/admin/server/logs/clear', { method: 'POST' });
            document.getElementById('terminalOutput').textContent = '';
        }

        // ---------------- NETWORK & TEXT FILES ----------------
        async function loadNetworkInfo() {
            try {
                const res = await fetch('/api/admin/network');
                const data = await res.json();
                if (data.success) {
                    document.getElementById('netPort').value = data.port;
                    document.getElementById('portText').textContent = 'Port: ' + data.port + ' (Open)';
                    document.getElementById('portDisplayUfw').textContent = data.port;
                    document.getElementById('portDisplayRouter').textContent = data.port;

                    const urlsDiv = document.getElementById('localUrls');
                    urlsDiv.innerHTML = '';
                    (data.local_ips || ['localhost']).forEach(ip => {
                        const url = 'http://' + ip + ':' + data.port + '/admin';
                        urlsDiv.innerHTML += '<div>▸ <a href="' + url + '" target="_blank" style="color:#a5b4fc; text-decoration:none;">' + url + '</a></div>';
                    });
                }
            } catch (err) {}
        }

        async function loadTextFiles() {
            try {
                const res = await fetch('/api/admin/mods/text-files');
                const data = await res.json();
                if (data.success) {
                    document.getElementById('clientOnlyText').value = (data.client_only || []).join('\n');
                    document.getElementById('serverOnlyText').value = (data.server_only || []).join('\n');
                }
            } catch (err) {}
        }

        async function saveTextFiles() {
            const clientOnly = document.getElementById('clientOnlyText').value.split('\n').map(s => s.trim()).filter(Boolean);
            const serverOnly = document.getElementById('serverOnlyText').value.split('\n').map(s => s.trim()).filter(Boolean);

            const res = await fetch('/api/admin/mods/text-files', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ client_only: clientOnly, server_only: serverOnly })
            });
            const data = await res.json();
            if (data.success) {
                showToast('✔ Text files saved & server manifest updated');
                loadMods();
            } else {
                showToast('Error saving text files');
            }
        }

        // ---------------- HELPERS & MODALS ----------------
        function openModal(id) { document.getElementById(id).classList.add('active'); }
        function closeModal(id) { document.getElementById(id).classList.remove('active'); }

        function showToast(msg) {
            const t = document.getElementById('toast');
            t.textContent = msg;
            t.style.display = 'block';
            setTimeout(() => { t.style.display = 'none'; }, 3500);
        }
    </script>
</body>
</html>`
