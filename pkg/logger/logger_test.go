package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoggerOverwritePreviousRun(t *testing.T) {
	tempDir := t.TempDir()

	// Run 1: Write some actions
	l1, err := New(Options{
		TargetDir:    tempDir,
		QuietConsole: true,
		Version:      "1.0.0",
		Args:         []string{"--gameDir", tempDir},
	})
	if err != nil {
		t.Fatalf("Failed to create logger 1: %v", err)
	}

	l1.Infof("Run 1: Action A performed")
	l1.Infof("Run 1: Action B performed")
	_ = l1.Close()

	logPath := filepath.Join(tempDir, DefaultLogFileName)
	content1, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file after run 1: %v", err)
	}

	if !strings.Contains(string(content1), "Run 1: Action A performed") {
		t.Errorf("Expected content1 to contain Run 1 actions, got: %s", string(content1))
	}

	// Run 2: Start new run in same directory. It MUST overwrite or remove the last run with current one!
	l2, err := New(Options{
		TargetDir:    tempDir,
		QuietConsole: true,
		Version:      "1.0.1",
		Args:         []string{"--gameDir", tempDir, "sync"},
	})
	if err != nil {
		t.Fatalf("Failed to create logger 2: %v", err)
	}

	l2.Infof("Run 2: Only this action should be present")
	_ = l2.Close()

	content2, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file after run 2: %v", err)
	}

	str2 := string(content2)
	// Must contain Run 2
	if !strings.Contains(str2, "Run 2: Only this action should be present") {
		t.Errorf("Expected content2 to contain Run 2 actions, got: %s", str2)
	}

	// Must NOT contain Run 1 actions (overwritten/removed)
	if strings.Contains(str2, "Run 1: Action A performed") {
		t.Errorf("Log file was not overwritten! Still contains Run 1 content: %s", str2)
	}
	if strings.Contains(str2, "Run 1: Action B performed") {
		t.Errorf("Log file was not overwritten! Still contains Run 1 content: %s", str2)
	}
}

func TestLoggerDualFiles(t *testing.T) {
	tempDir := t.TempDir()

	var consoleBuf bytes.Buffer
	l, err := New(Options{
		TargetDir: tempDir,
		Console:   &consoleBuf,
		Version:   "1.0.1",
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	l.Infof("[Lunaris] Downloading: test-mod.jar")
	l.Warnf("[Lunaris] Server response took 200ms")
	l.Errorf("[Lunaris] Failed to find optional pack")
	_ = l.Close()

	primaryPath := filepath.Join(tempDir, DefaultLogFileName)
	secondaryPath := filepath.Join(tempDir, LogsDirName, DefaultLogFileName)

	// Check primary file
	primaryData, err := os.ReadFile(primaryPath)
	if err != nil {
		t.Fatalf("Primary log file missing: %v", err)
	}
	if !strings.Contains(string(primaryData), "Downloading: test-mod.jar") {
		t.Errorf("Primary log missing expected content: %s", string(primaryData))
	}
	if !strings.Contains(string(primaryData), "[WARN]") {
		t.Errorf("Primary log missing WARN level: %s", string(primaryData))
	}

	// Check secondary file in logs/
	secondaryData, err := os.ReadFile(secondaryPath)
	if err != nil {
		t.Fatalf("Secondary log file missing: %v", err)
	}
	if !strings.Contains(string(secondaryData), "Downloading: test-mod.jar") {
		t.Errorf("Secondary log missing expected content: %s", string(secondaryData))
	}

	// Check console buffer
	consoleOutput := consoleBuf.String()
	if !strings.Contains(consoleOutput, "[Lunaris] Downloading: test-mod.jar") {
		t.Errorf("Console output missing expected content: %s", consoleOutput)
	}
}

func TestLoggerConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()

	l, err := New(Options{
		TargetDir:    tempDir,
		QuietConsole: true,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	const workerCount = 10
	const linesPerWorker = 50

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < linesPerWorker; j++ {
				l.Infof("Worker %d logged action %d", workerID, j)
			}
		}()
	}

	wg.Wait()
	_ = l.Close()

	logPath := filepath.Join(tempDir, DefaultLogFileName)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	for i := 0; i < workerCount; i++ {
		firstLine := fmt.Sprintf("Worker %d logged action 0", i)
		lastLine := fmt.Sprintf("Worker %d logged action %d", i, linesPerWorker-1)
		if !strings.Contains(content, firstLine) {
			t.Errorf("Missing expected line: %s", firstLine)
		}
		if !strings.Contains(content, lastLine) {
			t.Errorf("Missing expected line: %s", lastLine)
		}
	}
}

func TestLoggerCustomFile(t *testing.T) {
	tempDir := t.TempDir()
	customPath := filepath.Join(tempDir, "custom", "my_special.log")

	l, err := New(Options{
		CustomLogFile: customPath,
		QuietConsole:  true,
	})
	if err != nil {
		t.Fatalf("Failed to create logger with custom path: %v", err)
	}

	l.Infof("Custom action recorded")
	_ = l.Close()

	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("Failed to read custom log: %v", err)
	}
	if !strings.Contains(string(data), "Custom action recorded") {
		t.Errorf("Custom log does not contain action: %s", string(data))
	}
}

func TestLoggerPackageGlobal(t *testing.T) {
	tempDir := t.TempDir()

	files, err := Init(tempDir, Options{QuietConsole: true, Version: "2.0.0"})
	if err != nil {
		t.Fatalf("Global Init failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("Expected files returned from Init")
	}

	Infof("Global action 1")
	Warnf("Global warning 2")
	Errorf("Global error 3")
	_ = Sync()
	_ = Close()

	data, err := os.ReadFile(filepath.Join(tempDir, DefaultLogFileName))
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}
	str := string(data)
	if !strings.Contains(str, "Global action 1") || !strings.Contains(str, "Global warning 2") {
		t.Errorf("Global log content missing: %s", str)
	}

	// Verify second Init overwrites
	_, _ = Init(tempDir, Options{QuietConsole: true, Version: "2.0.1"})
	Infof("Overwritten run action")
	_ = Close()

	data2, _ := os.ReadFile(filepath.Join(tempDir, DefaultLogFileName))
	str2 := string(data2)
	if !strings.Contains(str2, "Overwritten run action") {
		t.Errorf("Second run content missing: %s", str2)
	}
	if strings.Contains(str2, "Global action 1") {
		t.Errorf("Second run still has first run content: %s", str2)
	}
}
