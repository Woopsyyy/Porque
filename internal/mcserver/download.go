package mcserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// downloadServerJar downloads the server jar if it doesn't already exist.
func downloadServerJar(destDir string, serverType string, version string) (string, error) {
	var jarName string
	switch serverType {
	case "PAPER":
		jarName = fmt.Sprintf("paper-%s.jar", version)
	case "VANILLA":
		jarName = fmt.Sprintf("vanilla-%s.jar", version)
	default:
		jarName = "server.jar"
	}

	jarPath := filepath.Join(destDir, jarName)
	if _, err := os.Stat(jarPath); err == nil {
		return jarName, nil // Already exists
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	// Download logic based on type
	switch serverType {
	case "PAPER":
		if err := downloadPaper(version, jarPath); err != nil {
			return "", fmt.Errorf("download paper jar: %w", err)
		}
	case "VANILLA":
		if err := downloadVanilla(version, jarPath); err != nil {
			return "", fmt.Errorf("download vanilla jar: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported native server type: %s", serverType)
	}

	return jarName, nil
}

type paperVersionInfo struct {
	Builds []int `json:"builds"`
}

func downloadPaper(version, destPath string) error {
	// 1. Get latest build number
	url := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", version)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get version info from papermc: status %d", resp.StatusCode)
	}

	var info paperVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}
	if len(info.Builds) == 0 {
		return fmt.Errorf("no builds found for paper version %s", version)
	}
	latestBuild := info.Builds[len(info.Builds)-1]

	// 2. Download the jar
	jarURL := fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/paper-%s-%d.jar",
		version, latestBuild, version, latestBuild)

	return downloadFile(jarURL, destPath)
}

type mojangManifest struct {
	Versions []struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"versions"`
}

type mojangVersionDetails struct {
	Downloads struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
}

func downloadVanilla(version, destPath string) error {
	// 1. Query manifest
	resp, err := http.Get("https://launchermeta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var manifest mojangManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return err
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		// Fallback to "latest" check if version is "latest" or "release"
		if version == "latest" || version == "release" {
			for _, v := range manifest.Versions {
				if v.Type == "release" {
					versionURL = v.URL
					break
				}
			}
		}
	}

	if versionURL == "" {
		return fmt.Errorf("vanilla version %s not found in mojang manifest", version)
	}

	// 2. Query version details
	respDetails, err := http.Get(versionURL)
	if err != nil {
		return err
	}
	defer respDetails.Body.Close()

	var details mojangVersionDetails
	if err := json.NewDecoder(respDetails.Body).Decode(&details); err != nil {
		return err
	}

	serverURL := details.Downloads.Server.URL
	if serverURL == "" {
		return fmt.Errorf("no server jar found in mojang version details for %s", version)
	}

	// 3. Download the jar
	return downloadFile(serverURL, destPath)
}

func downloadFile(url, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download: status %d", resp.StatusCode)
	}

	_, err = io.Copy(out, resp.Body)
	return err
}
