package playit

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

// DownloadPlayitAgent downloads the playit binary if it doesn't already exist.
func DownloadPlayitAgent(destDir string) (string, error) {
	var binaryName string
	var downloadURL string

	switch runtime.GOOS {
	case "windows":
		binaryName = "playit.exe"
		downloadURL = "https://github.com/playit-cloud/playit-agent/releases/download/v0.15.26/playit-windows-x86_64.exe"
	case "linux":
		binaryName = "playit"
		downloadURL = "https://github.com/playit-cloud/playit-agent/releases/download/v0.15.26/playit-linux-amd64"
	case "darwin":
		binaryName = "playit"
		if runtime.GOARCH == "arm64" {
			downloadURL = "https://github.com/playit-cloud/playit-agent/releases/download/v0.15.26/playit-macos-arm64"
		} else {
			downloadURL = "https://github.com/playit-cloud/playit-agent/releases/download/v0.15.26/playit-macos-amd64"
		}
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	destPath := filepath.Join(destDir, binaryName)
	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil // Already exists
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	defer out.Close()

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download playit: status %d", resp.StatusCode)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return destPath, nil
}
