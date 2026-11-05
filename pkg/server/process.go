package server

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type ProcessStatus string

const (
	StatusStopped      ProcessStatus = "stopped"
	StatusStarting     ProcessStatus = "starting"
	StatusRunning      ProcessStatus = "running"
	StatusStopping     ProcessStatus = "stopping"
	StatusFailedToBoot ProcessStatus = "failed_to_boot"
)

// ProcessManager manages the lifecycle and output of the Minecraft server process.
type ProcessManager struct {
	mu              sync.RWMutex
	rootDir         string
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	status          ProcessStatus
	startTime       time.Time
	stopTime        time.Time
	startCommand    string
	intentionalStop bool
	bootCompleted   bool
	failedBoot      bool
	failedReason    string
	lastExitCode    int
	logBuffer       []string
	maxLogLines     int
	statusSubscribers []chan ProcessStatus
}

// NewProcessManager initializes a ProcessManager for the given server directory.
func NewProcessManager(rootDir, startCmd string) *ProcessManager {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	if startCmd == "" {
		startCmd = autoDetectStartCommand(absRoot)
	}

	return &ProcessManager{
		rootDir:      absRoot,
		startCommand: startCmd,
		status:       StatusStopped,
		maxLogLines:  2000,
		logBuffer:    make([]string, 0, 500),
	}
}

// autoDetectStartCommand inspects the directory for common Minecraft server launch scripts or jars.
func autoDetectStartCommand(dir string) string {
	isWindows := runtime.GOOS == "windows"

	// 1. Check for scripts
	if isWindows {
		for _, s := range []string{"run.bat", "start.bat", "launch.bat", "run.cmd", "start.cmd"} {
			p := filepath.Join(dir, s)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	} else {
		for _, s := range []string{"run.sh", "start.sh", "launch.sh", "server.sh"} {
			p := filepath.Join(dir, s)
			if _, err := os.Stat(p); err == nil {
				return "./" + s
			}
		}
	}

	// 2. Check for server jars
	jarPatterns := []string{
		"forge-*.jar",
		"fabric-server-launch.jar",
		"fabric-server-*.jar",
		"neoforge-*.jar",
		"paper*.jar",
		"purpur*.jar",
		"server.jar",
	}

	for _, pat := range jarPatterns {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err == nil && len(matches) > 0 {
			// Find most specific match
			base := filepath.Base(matches[0])
			if !strings.Contains(base, "-installer") {
				return fmt.Sprintf("java -Xmx4G -jar %s nogui", base)
			}
		}
	}

	if isWindows {
		return "run.bat"
	}
	return "./run.sh"
}

// GetStatus returns the current status and diagnostic details.
func (p *ProcessManager) GetStatus() (status ProcessStatus, pid int, uptimeSec int64, failedBoot bool, reason string, startCmd string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status = p.status
	if p.cmd != nil && p.cmd.Process != nil && (p.status == StatusRunning || p.status == StatusStarting) {
		pid = p.cmd.Process.Pid
		uptimeSec = int64(time.Since(p.startTime).Seconds())
	}
	failedBoot = p.failedBoot
	reason = p.failedReason
	startCmd = p.startCommand
	return
}

// SetStartCommand updates the launch command.
func (p *ProcessManager) SetStartCommand(cmdStr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startCommand = strings.TrimSpace(cmdStr)
}

// Start executes the server process.
func (p *ProcessManager) Start() error {
	p.mu.Lock()
	if p.status == StatusRunning || p.status == StatusStarting {
		p.mu.Unlock()
		return fmt.Errorf("server is already running")
	}

	cmdStr := p.startCommand
	if cmdStr == "" {
		cmdStr = autoDetectStartCommand(p.rootDir)
		p.startCommand = cmdStr
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		// Use sh -c to execute scripts or java commands directly
		cmd = exec.Command("sh", "-c", cmdStr)
	}
	cmd.Dir = p.rootDir
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.mu.Unlock()
		_ = stdin.Close()
		return fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.mu.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("failed to open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		_ = stdin.Close()
		return fmt.Errorf("failed to start server process: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.status = StatusStarting
	p.startTime = time.Now()
	p.intentionalStop = false
	p.bootCompleted = false
	p.failedBoot = false
	p.failedReason = ""
	p.appendLogLocked(fmt.Sprintf("[Lunaris] === Minecraft server starting via: %s ===", cmdStr))
	p.mu.Unlock()

	// Stream stdout & stderr into log buffer
	go p.streamOutput(stdout)
	go p.streamOutput(stderr)

	// Monitor process termination
	go p.monitorProcess(cmd)

	return nil
}

func (p *ProcessManager) streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		p.mu.Lock()
		p.appendLogLocked(line)

		// Check for successful Minecraft boot completion:
		// Forge/Fabric/Vanilla log: 'Done (X.XXs)! For help, type "help"'
		if !p.bootCompleted && strings.Contains(line, "Done (") && strings.Contains(line, "help") {
			p.bootCompleted = true
			p.status = StatusRunning
			p.failedBoot = false
		}

		// Detect fatal error messages
		if strings.Contains(line, "Encountered an unexpected exception") ||
			strings.Contains(line, "FAILED TO BIND TO PORT!") ||
			strings.Contains(line, "Crash report saved to") {
			p.failedReason = strings.TrimSpace(line)
		}
		p.mu.Unlock()
	}
}

func (p *ProcessManager) monitorProcess(cmd *exec.Cmd) {
	_ = cmd.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopTime = time.Now()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	p.lastExitCode = exitCode

	p.appendLogLocked(fmt.Sprintf("[Lunaris] === Minecraft server process exited with code %d ===", exitCode))

	if p.intentionalStop {
		p.status = StatusStopped
		p.failedBoot = false
		return
	}

	// Server died unexpectedly!
	// If boot never completed or exit code != 0, flag as failed to boot!
	if !p.bootCompleted || exitCode != 0 {
		p.status = StatusFailedToBoot
		p.failedBoot = true
		if p.failedReason == "" {
			if exitCode != 0 {
				p.failedReason = fmt.Sprintf("Server crashed during startup (Exit code %d)", exitCode)
			} else {
				p.failedReason = "Server stopped unexpectedly before finishing boot sequence."
			}
		}
	} else {
		// Crashed while running
		p.status = StatusStopped
		p.failedBoot = true
		p.failedReason = fmt.Sprintf("Server stopped unexpectedly (Exit code %d)", exitCode)
	}
}

// Stop sends a graceful stop command to the Minecraft server and terminates it after timeout.
func (p *ProcessManager) Stop() error {
	p.mu.Lock()
	if p.status == StatusStopped || p.status == StatusFailedToBoot || p.cmd == nil || p.cmd.Process == nil {
		p.status = StatusStopped
		p.mu.Unlock()
		return nil
	}

	p.intentionalStop = true
	p.status = StatusStopping
	stdin := p.stdin
	cmd := p.cmd
	p.appendLogLocked("[Lunaris] Sending graceful stop command to server...")
	p.mu.Unlock()

	// 1. Send "stop" command over stdin
	if stdin != nil {
		_, _ = stdin.Write([]byte("stop\n"))
	}

	// 2. Wait up to 15 seconds for clean exit
	done := make(chan struct{})
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(500 * time.Millisecond)
			p.mu.RLock()
			st := p.status
			p.mu.RUnlock()
			if st == StatusStopped || st == StatusFailedToBoot {
				close(done)
				return
			}
		}
		close(done)
	}()

	<-done

	// 3. Force kill if still running
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status == StatusStopping && cmd != nil && cmd.Process != nil {
		p.appendLogLocked("[Lunaris] Server did not stop cleanly within 15s; sending force kill.")
		_ = cmd.Process.Kill()
		p.status = StatusStopped
	}

	return nil
}

// Restart stops the server if running, then starts it again.
func (p *ProcessManager) Restart() error {
	p.mu.RLock()
	isRunning := p.status == StatusRunning || p.status == StatusStarting || p.status == StatusStopping
	p.mu.RUnlock()

	if isRunning {
		_ = p.Stop()
		time.Sleep(500 * time.Millisecond)
	}

	return p.Start()
}

// SendCommand writes a command line to the server's stdin.
func (p *ProcessManager) SendCommand(cmdText string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status != StatusRunning && p.status != StatusStarting {
		return fmt.Errorf("cannot send command: server is not running")
	}
	if p.stdin == nil {
		return fmt.Errorf("server stdin is not available")
	}

	cmdText = strings.TrimSpace(cmdText)
	p.appendLogLocked(fmt.Sprintf("> %s", cmdText))
	_, err := p.stdin.Write([]byte(cmdText + "\n"))
	return err
}

func (p *ProcessManager) appendLogLocked(line string) {
	p.logBuffer = append(p.logBuffer, line)
	if len(p.logBuffer) > p.maxLogLines {
		p.logBuffer = p.logBuffer[len(p.logBuffer)-p.maxLogLines:]
	}
}

// GetConsoleLogs returns the most recent lines from the in-memory rolling buffer.
func (p *ProcessManager) GetConsoleLogs(limit int) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if limit <= 0 || limit > len(p.logBuffer) {
		limit = len(p.logBuffer)
	}

	start := len(p.logBuffer) - limit
	result := make([]string, limit)
	copy(result, p.logBuffer[start:])
	return result
}

// ClearConsoleBuffer empties the in-memory console buffer.
func (p *ProcessManager) ClearConsoleBuffer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logBuffer = make([]string, 0, 500)
}

// GetLatestLog retrieves the latest log for debugging (from crash-reports, logs/latest.log, or console buffer).
func (p *ProcessManager) GetLatestLog() (content string, source string, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 1. Check for newest crash-report (generated within last 30 minutes)
	crashReportsDir := filepath.Join(p.rootDir, "crash-reports")
	if files, err := os.ReadDir(crashReportsDir); err == nil && len(files) > 0 {
		var newest os.DirEntry
		var newestTime time.Time
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".txt") {
				if info, err := f.Info(); err == nil {
					if info.ModTime().After(newestTime) {
						newestTime = info.ModTime()
						newest = f
					}
				}
			}
		}
		if newest != nil && time.Since(newestTime) < 30*time.Minute {
			crashPath := filepath.Join(crashReportsDir, newest.Name())
			if data, err := os.ReadFile(crashPath); err == nil && len(data) > 0 {
				return string(data), filepath.Join("crash-reports", newest.Name()), nil
			}
		}
	}

	// 2. Check logs/latest.log
	latestLogPath := filepath.Join(p.rootDir, "logs", "latest.log")
	if data, err := os.ReadFile(latestLogPath); err == nil && len(data) > 0 {
		return string(data), "logs/latest.log", nil
	}

	// 3. Check logs/debug.log
	debugLogPath := filepath.Join(p.rootDir, "logs", "debug.log")
	if data, err := os.ReadFile(debugLogPath); err == nil && len(data) > 0 {
		return string(data), "logs/debug.log", nil
	}

	// 4. Fallback to in-memory console log buffer
	if len(p.logBuffer) > 0 {
		return strings.Join(p.logBuffer, "\n"), "console buffer", nil
	}

	return "No server log files found.", "none", nil
}
