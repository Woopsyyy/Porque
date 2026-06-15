package mcserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/woopsy/porque/internal/winproc"
)

// prepareServerLaunch ensures the server files for the given loader exist
// (downloading/installing as needed) and returns the java arguments to launch
// it, excluding the -Xms/-Xmx heap flags which the caller prepends. This keeps
// the lifecycle launch code loader-agnostic.
func prepareServerLaunch(destDir, serverType, version, loaderVersion string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}

	switch serverType {
	case "PAPER":
		jarName := fmt.Sprintf("paper-%s.jar", version)
		if err := ensureJar(filepath.Join(destDir, jarName), func(p string) error { return downloadPaper(version, p) }); err != nil {
			return nil, fmt.Errorf("download paper jar: %w", err)
		}
		return []string{"-jar", jarName, "nogui"}, nil
	case "VANILLA":
		jarName := fmt.Sprintf("vanilla-%s.jar", version)
		if err := ensureJar(filepath.Join(destDir, jarName), func(p string) error { return downloadVanilla(version, p) }); err != nil {
			return nil, fmt.Errorf("download vanilla jar: %w", err)
		}
		return []string{"-jar", jarName, "nogui"}, nil
	case "FABRIC":
		return prepareFabric(destDir, version, loaderVersion)
	case "FORGE":
		return prepareForge(destDir, version, loaderVersion)
	default:
		return nil, fmt.Errorf("unsupported native server type: %s", serverType)
	}
}

// ensureJar downloads to jarPath via dl only if the file is not already present.
func ensureJar(jarPath string, dl func(destPath string) error) error {
	if _, err := os.Stat(jarPath); err == nil {
		return nil
	}
	return dl(jarPath)
}

// ----- Fabric --------------------------------------------------------------

// prepareFabric downloads the self-contained Fabric server launcher jar, which
// is runnable directly with `-jar`.
func prepareFabric(destDir, version, loaderVersion string) ([]string, error) {
	loaderVer := loaderVersion
	if loaderVer == "" {
		v, err := latestFabricLoader(version)
		if err != nil {
			return nil, err
		}
		loaderVer = v
	}
	installerVer, err := latestFabricInstaller()
	if err != nil {
		return nil, err
	}

	jarName := fmt.Sprintf("fabric-server-%s-%s.jar", version, loaderVer)
	jarPath := filepath.Join(destDir, jarName)
	if _, statErr := os.Stat(jarPath); statErr != nil {
		url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/%s/server/jar", version, loaderVer, installerVer)
		if err := downloadFile(url, jarPath); err != nil {
			return nil, fmt.Errorf("download fabric server jar: %w", err)
		}
	}
	return []string{"-jar", jarName, "nogui"}, nil
}

func latestFabricLoader(version string) (string, error) {
	var arr []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := getJSON(fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", version), &arr); err != nil {
		return "", fmt.Errorf("resolve fabric loader for %s: %w", version, err)
	}
	if len(arr) == 0 {
		return "", fmt.Errorf("no fabric loader available for minecraft %s", version)
	}
	return arr[0].Loader.Version, nil
}

func latestFabricInstaller() (string, error) {
	var arr []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := getJSON("https://meta.fabricmc.net/v2/versions/installer", &arr); err != nil {
		return "", fmt.Errorf("resolve fabric installer: %w", err)
	}
	for _, it := range arr {
		if it.Stable {
			return it.Version, nil
		}
	}
	if len(arr) > 0 {
		return arr[0].Version, nil
	}
	return "", fmt.Errorf("no fabric installer available")
}

// ----- Forge ---------------------------------------------------------------

// prepareForge downloads and runs the Forge installer headlessly, then returns
// the launch arguments. Modern Forge (1.17+) produces an args file consumed via
// @file; older Forge produces a runnable universal jar.
func prepareForge(destDir, version, loaderVersion string) ([]string, error) {
	// Reuse an existing install if present.
	if args, ok := findForgeLaunch(destDir, version, loaderVersion); ok {
		return args, nil
	}

	forgeVer := loaderVersion
	if forgeVer == "" {
		v, err := resolveForgeVersion(version)
		if err != nil {
			return nil, err
		}
		forgeVer = v
	}

	installerURL := fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		version, forgeVer, version, forgeVer)
	installerPath := filepath.Join(destDir, "forge-installer.jar")
	if err := downloadFile(installerURL, installerPath); err != nil {
		return nil, fmt.Errorf("download forge installer (mc %s forge %s): %w", version, forgeVer, err)
	}

	cmd := exec.Command("java", "-jar", "forge-installer.jar", "--installServer")
	cmd.Dir = destDir
	winproc.Hide(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("forge installer failed: %w: %s", err, truncate(string(out), 500))
	}
	_ = os.Remove(installerPath)
	_ = os.Remove(installerPath + ".log")

	if args, ok := findForgeLaunch(destDir, version, forgeVer); ok {
		return args, nil
	}
	return nil, fmt.Errorf("forge install for mc %s (forge %s) produced no runnable server", version, forgeVer)
}

// findForgeLaunch locates a Forge install in destDir and returns its launch args.
func findForgeLaunch(destDir, version, forgeVer string) ([]string, bool) {
	argFile := "win_args.txt"
	if runtime.GOOS != "windows" {
		argFile = "unix_args.txt"
	}

	// Preferred exact path when the forge version is known.
	if forgeVer != "" {
		rel := filepath.Join("libraries", "net", "minecraftforge", "forge", version+"-"+forgeVer, argFile)
		if _, err := os.Stat(filepath.Join(destDir, rel)); err == nil {
			return []string{"@" + rel, "nogui"}, true
		}
	}
	// Glob fallback for the args file (modern Forge).
	if matches, _ := filepath.Glob(filepath.Join(destDir, "libraries", "net", "minecraftforge", "forge", "*", argFile)); len(matches) > 0 {
		if rel, err := filepath.Rel(destDir, matches[0]); err == nil {
			return []string{"@" + rel, "nogui"}, true
		}
	}
	// Old Forge: a runnable universal/server jar.
	if jars, _ := filepath.Glob(filepath.Join(destDir, "forge-*.jar")); len(jars) > 0 {
		for _, j := range jars {
			base := filepath.Base(j)
			if strings.Contains(base, "installer") {
				continue
			}
			return []string{"-jar", base, "nogui"}, true
		}
	}
	return nil, false
}

func resolveForgeVersion(version string) (string, error) {
	var data struct {
		Promos map[string]string `json:"promos"`
	}
	if err := getJSON("https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json", &data); err != nil {
		return "", fmt.Errorf("resolve forge version for %s: %w", version, err)
	}
	if v, ok := data.Promos[version+"-recommended"]; ok {
		return v, nil
	}
	if v, ok := data.Promos[version+"-latest"]; ok {
		return v, nil
	}
	return "", fmt.Errorf("no forge build published for minecraft %s", version)
}

// getJSON fetches url and decodes the JSON body into out.
func getJSON(url string, out any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d from %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
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
