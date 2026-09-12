//go:build windows

package injector

// ExecOrRunJava launches the real Java process and waits for it to complete on Windows.
func ExecOrRunJava(javaPath string, args []string) (int, error) {
	return RunJava(javaPath, args)
}
