//go:build windows

package gui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// OpenBrowser attempts to open an Edge/Chrome standalone app window,
// falling back to OpenDefaultBrowser.
func OpenBrowser(targetURL string) error {
	tempProfile := filepath.Join(os.TempDir(), "lunaris-app-profile")

	// 1. Try Microsoft Edge in standalone app window with dedicated profile
	var edgeCandidates []string
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		edgeCandidates = append(edgeCandidates, p)
	}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if v := os.Getenv(env); v != "" {
			edgeCandidates = append(edgeCandidates, filepath.Join(v, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	edgeCandidates = append(edgeCandidates,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	)

	for _, p := range edgeCandidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			cmd := exec.Command(p,
				"--app="+targetURL,
				"--window-size=920,840",
				"--user-data-dir="+tempProfile,
				"--no-first-run",
				"--no-default-browser-check",
				"--disable-session-crashed-bubble",
			)
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}

	// 2. Try Google Chrome in standalone app window with dedicated profile
	var chromeCandidates []string
	if p, err := exec.LookPath("chrome.exe"); err == nil {
		chromeCandidates = append(chromeCandidates, p)
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if v := os.Getenv(env); v != "" {
			chromeCandidates = append(chromeCandidates, filepath.Join(v, "Google", "Chrome", "Application", "chrome.exe"))
		}
	}
	chromeCandidates = append(chromeCandidates,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	)

	for _, p := range chromeCandidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			cmd := exec.Command(p,
				"--app="+targetURL,
				"--window-size=920,840",
				"--user-data-dir="+tempProfile,
				"--no-first-run",
				"--no-default-browser-check",
				"--disable-session-crashed-bubble",
			)
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}

	return OpenDefaultBrowser(targetURL)
}

// OpenDefaultBrowser uses ShellExecuteW and native Windows fallbacks to open the default browser.
func OpenDefaultBrowser(targetURL string) error {
	// 1. ShellExecuteW via shell32.dll (primary native API)
	shell32 := syscall.NewLazyDLL("shell32.dll")
	procShellExecute := shell32.NewProc("ShellExecuteW")
	verbPtr, err1 := syscall.UTF16PtrFromString("open")
	urlPtr, err2 := syscall.UTF16PtrFromString(targetURL)
	if err1 == nil && err2 == nil {
		ret, _, _ := procShellExecute.Call(
			0,
			uintptr(unsafe.Pointer(verbPtr)),
			uintptr(unsafe.Pointer(urlPtr)),
			0,
			0,
			1, // SW_SHOWNORMAL
		)
		if ret > 32 {
			return nil
		}
	}

	// 2. rundll32 fallback (hidden window)
	cmd1 := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	cmd1.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd1.Start(); err == nil {
		return nil
	}

	// 3. cmd /c start "" "<url>" fallback (hidden window)
	cmd2 := exec.Command("cmd", "/c", "start", "", targetURL)
	cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd2.Start(); err == nil {
		return nil
	}

	// 4. powershell Start-Process fallback (hidden window)
	cmd3 := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf("Start-Process '%s'", targetURL))
	cmd3.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd3.Start()
}

// ShowNativeAlert displays a native Windows MessageBox if an error occurs.
func ShowNativeAlert(title, message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBox := user32.NewProc("MessageBoxW")
	tPtr, _ := syscall.UTF16PtrFromString(title)
	mPtr, _ := syscall.UTF16PtrFromString(message)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(mPtr)), uintptr(unsafe.Pointer(tPtr)), 0x00000010) // MB_ICONERROR
}
