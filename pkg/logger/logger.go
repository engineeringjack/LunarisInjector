package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultLogFileName is the primary log file name written to the instance/working directory.
	DefaultLogFileName = "lunaris.log"

	// LogsDirName is the standard Minecraft logs directory name.
	LogsDirName = "logs"
)

var (
	// ansiRegex matches ANSI escape codes to strip them when writing to log files.
	ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

	// Package-level singleton mutex and default logger.
	defaultMu     sync.RWMutex
	defaultLogger *Logger
)

// Level represents log message severity.
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
	LevelDebug
)

func (l Level) String() string {
	switch l {
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelDebug:
		return "DEBUG"
	default:
		return "INFO"
	}
}

// Options configures the Logger instance.
type Options struct {
	// TargetDir is the primary directory where lunaris.log will be written.
	TargetDir string

	// CustomLogFile allows specifying an explicit log file path instead of the default.
	CustomLogFile string

	// Console receives formatted output (defaults to os.Stdout).
	Console io.Writer

	// QuietConsole suppresses output to the console.
	QuietConsole bool

	// EnableLogsDir controls whether a secondary log is written to <TargetDir>/logs/lunaris.log.
	// Defaults to true when TargetDir is provided.
	EnableLogsDir *bool

	// Version to print in the run header (optional).
	Version string

	// Args to print in the run header (optional).
	Args []string
}

// Logger provides thread-safe logging to both console and log file(s).
// Each initialization removes/overwrites the log from previous runs so only the current run is preserved.
type Logger struct {
	mu           sync.Mutex
	files        []*os.File
	filePaths    []string
	console      io.Writer
	quietConsole bool
	targetDir    string
	closed       bool
}

// New creates a new Logger according to the provided options.
// Any previous log files from earlier runs are removed/overwritten.
func New(opts Options) (*Logger, error) {
	console := opts.Console
	if console == nil {
		console = os.Stdout
	}

	l := &Logger{
		console:      console,
		quietConsole: opts.QuietConsole,
		targetDir:    opts.TargetDir,
	}

	enableLogsDir := true
	if opts.EnableLogsDir != nil {
		enableLogsDir = *opts.EnableLogsDir
	}

	var targetFiles []string

	if opts.CustomLogFile != "" {
		targetFiles = append(targetFiles, opts.CustomLogFile)
	} else if opts.TargetDir != "" {
		cleanTarget := filepath.Clean(opts.TargetDir)
		targetFiles = append(targetFiles, filepath.Join(cleanTarget, DefaultLogFileName))

		if enableLogsDir {
			logsSubdir := filepath.Join(cleanTarget, LogsDirName)
			targetFiles = append(targetFiles, filepath.Join(logsSubdir, DefaultLogFileName))
		}
	} else {
		// Fallback to current working directory
		cwd, err := os.Getwd()
		if err == nil {
			targetFiles = append(targetFiles, filepath.Join(cwd, DefaultLogFileName))
		} else {
			targetFiles = append(targetFiles, filepath.Join(os.TempDir(), DefaultLogFileName))
		}
	}

	// Open and truncate/remove previous run's log files
	for _, path := range targetFiles {
		cleanPath := filepath.Clean(path)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
			continue
		}

		// Explicitly remove previous log if present to guarantee a fresh start
		_ = os.Remove(cleanPath)

		// Open with O_TRUNC | O_CREATE | O_WRONLY
		f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			continue
		}

		l.files = append(l.files, f)
		l.filePaths = append(l.filePaths, cleanPath)
	}

	// Write initial header to log files
	l.writeHeader(opts.Version, opts.Args)

	return l, nil
}

func (l *Logger) writeHeader(version string, args []string) {
	if len(l.files) == 0 {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	verStr := version
	if verStr == "" {
		verStr = "unknown"
	}

	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString(fmt.Sprintf("LunarisInjector v%s - Action Log\n", verStr))
	sb.WriteString(fmt.Sprintf("Started At : %s\n", now))
	sb.WriteString(fmt.Sprintf("Process ID : %d\n", os.Getpid()))
	if l.targetDir != "" {
		sb.WriteString(fmt.Sprintf("Target Dir : %s\n", l.targetDir))
	}
	if len(args) > 0 {
		sb.WriteString(fmt.Sprintf("Arguments  : %s\n", strings.Join(args, " ")))
	}
	sb.WriteString("================================================================================\n")

	headerBytes := []byte(sb.String())
	for _, f := range l.files {
		_, _ = f.Write(headerBytes)
		_ = f.Sync()
	}
}

// Log writes a message at INFO level.
func (l *Logger) Log(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Infof writes a message at INFO level.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warnf writes a message at WARN level.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Errorf writes a message at ERROR level.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Debugf writes a message at DEBUG level.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Printf writes a message at INFO level, maintaining standard fmt.Printf behavior.
func (l *Logger) Printf(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Println writes arguments followed by a newline at INFO level.
func (l *Logger) Println(args ...interface{}) {
	l.log(LevelInfo, fmt.Sprint(args...))
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	rawMsg := fmt.Sprintf(format, args...)
	// Strip trailing newline if present so we can consistently format
	cleanMsg := strings.TrimRight(rawMsg, "\r\n")

	// 1. Output to Console if enabled
	if !l.quietConsole && l.console != nil {
		fmt.Fprintf(l.console, "%s\n", cleanMsg)
	}

	if l.closed || len(l.files) == 0 {
		return
	}

	// 2. Format for File with timestamp & level
	plainMsg := stripANSI(cleanMsg)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	var fileLine string
	// If message is a pure visual banner / border, write it without level clutter for cleaner readability
	if isBannerLine(plainMsg) {
		fileLine = fmt.Sprintf("[%s] %s\n", timestamp, plainMsg)
	} else {
		fileLine = fmt.Sprintf("[%s] [%s] %s\n", timestamp, level.String(), plainMsg)
	}

	lineBytes := []byte(fileLine)
	for _, f := range l.files {
		_, _ = f.Write(lineBytes)
		// Immediately flush to disk so no log data is lost if terminated or syscall.Exec is invoked
		_ = f.Sync()
	}
}

// Sync flushes buffered data to physical disk for all open log files.
func (l *Logger) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var lastErr error
	for _, f := range l.files {
		if err := f.Sync(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Close flushes and closes all open log file descriptors.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	var lastErr error
	for _, f := range l.files {
		_ = f.Sync()
		if err := f.Close(); err != nil {
			lastErr = err
		}
	}
	l.files = nil
	return lastErr
}

// FilePaths returns the absolute paths of all active log files being written to.
func (l *Logger) FilePaths() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	res := make([]string, len(l.filePaths))
	copy(res, l.filePaths)
	return res
}

// AsFunc returns a func(string, ...interface{}) compatible with various logging hooks.
func (l *Logger) AsFunc() func(string, ...interface{}) {
	return func(format string, args ...interface{}) {
		l.Infof(format, args...)
	}
}

func stripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func isBannerLine(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 4 {
		return false
	}
	firstChar := trimmed[0]
	if firstChar == '=' || firstChar == '-' || firstChar == '*' {
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] != firstChar {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(trimmed, "╭──") || strings.HasPrefix(trimmed, "╰──") || strings.HasPrefix(trimmed, "├──") {
		return true
	}
	return false
}

// Package-level global singleton methods

// Init initializes or resets the global logger for the specified target directory.
// Any previous log files from prior runs in that directory are removed/overwritten.
func Init(targetDir string, opts ...Options) ([]string, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultLogger != nil {
		_ = defaultLogger.Close()
	}

	opt := Options{TargetDir: targetDir}
	if len(opts) > 0 {
		opt = opts[0]
		if opt.TargetDir == "" {
			opt.TargetDir = targetDir
		}
	}

	l, err := New(opt)
	defaultLogger = l
	if err != nil {
		return nil, err
	}
	return l.FilePaths(), nil
}

// Get returns the current default logger, creating a fallback instance if none exists.
func Get() *Logger {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()

	if l != nil {
		return l
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultLogger == nil {
		defaultLogger, _ = New(Options{})
	}
	return defaultLogger
}

// Infof writes to the default logger at INFO level.
func Infof(format string, args ...interface{}) {
	Get().Infof(format, args...)
}

// Warnf writes to the default logger at WARN level.
func Warnf(format string, args ...interface{}) {
	Get().Warnf(format, args...)
}

// Errorf writes to the default logger at ERROR level.
func Errorf(format string, args ...interface{}) {
	Get().Errorf(format, args...)
}

// Debugf writes to the default logger at DEBUG level.
func Debugf(format string, args ...interface{}) {
	Get().Debugf(format, args...)
}

// Printf writes to the default logger at INFO level.
func Printf(format string, args ...interface{}) {
	Get().Printf(format, args...)
}

// Println writes to the default logger at INFO level.
func Println(args ...interface{}) {
	Get().Println(args...)
}

// Sync flushes the default logger.
func Sync() error {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultLogger != nil {
		return defaultLogger.Sync()
	}
	return nil
}

// Close closes the default logger.
func Close() error {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultLogger != nil {
		err := defaultLogger.Close()
		defaultLogger = nil
		return err
	}
	return nil
}

// AsFunc returns a logging function tied to the default logger.
func AsFunc() func(string, ...interface{}) {
	return func(format string, args ...interface{}) {
		Get().Infof(format, args...)
	}
}

// FilePaths returns the paths of log files created by the default logger.
func FilePaths() []string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultLogger != nil {
		return defaultLogger.FilePaths()
	}
	return nil
}
