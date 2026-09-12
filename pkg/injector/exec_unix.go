//go:build !windows

package injector

import (
	"os"
	"syscall"
)

// ExecOrRunJava replaces the current process with the real Java runtime on Unix-like systems,
// falling back to running it as a child process if syscall.Exec fails.
func ExecOrRunJava(javaPath string, args []string) (int, error) {
	argv := append([]string{javaPath}, args...)
	err := syscall.Exec(javaPath, argv, os.Environ())
	if err != nil {
		return RunJava(javaPath, args)
	}
	return 0, nil
}
