//go:build !windows

package gui

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the given URL in an application window (Chrome app mode)
// or the user's default browser.
func OpenBrowser(targetURL string) error {
	switch runtime.GOOS {
	case "darwin":
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := exec.LookPath(chromePath); err == nil {
			return exec.Command(chromePath, "--app="+targetURL, "--window-size=920,840").Start()
		}
		return OpenDefaultBrowser(targetURL)

	default: // Linux
		for _, browser := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(browser); err == nil {
				return exec.Command(p, "--app="+targetURL, "--window-size=920,840").Start()
			}
		}
		return OpenDefaultBrowser(targetURL)
	}
}

// OpenDefaultBrowser opens the target URL using the system's default browser.
func OpenDefaultBrowser(targetURL string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", targetURL).Start()
	}
	return exec.Command("xdg-open", targetURL).Start()
}

// ShowNativeAlert is a no-op on non-Windows platforms.
func ShowNativeAlert(title, message string) {}
