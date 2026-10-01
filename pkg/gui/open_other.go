//go:build !windows

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens the given URL in an application window (Chrome app mode)
// or the user's default browser.
func OpenBrowser(targetURL string) error {
	switch runtime.GOOS {
	case "darwin":
		chromeCandidates := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		for _, p := range chromeCandidates {
			if _, err := os.Stat(p); err == nil {
				if err := exec.Command(p, "--app="+targetURL, "--window-size=920,840").Start(); err == nil {
					return nil
				}
			}
		}
		return OpenDefaultBrowser(targetURL)

	default: // Linux
		for _, browser := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser", "microsoft-edge"} {
			if p, err := exec.LookPath(browser); err == nil {
				if err := exec.Command(p, "--app="+targetURL, "--window-size=920,840").Start(); err == nil {
					return nil
				}
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

// ShowNativeAlert displays a native dialog on macOS or is a no-op on Linux.
func ShowNativeAlert(title, message string) {
	if runtime.GOOS == "darwin" {
		cleanTitle := strings.ReplaceAll(title, `"`, `\"`)
		cleanMsg := strings.ReplaceAll(message, `"`, `\"`)
		script := fmt.Sprintf(`display alert "%s" message "%s" as critical buttons {"OK"} default button "OK"`, cleanTitle, cleanMsg)
		_ = exec.Command("osascript", "-e", script).Run()
	}
}
