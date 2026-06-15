// Package updater checks GitHub Releases for a newer build of Porque and
// applies it by self-replacing the running executable.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

const (
	// repo is the GitHub "owner/name" that publishes Porque releases.
	repo = "Woopsyyy/Porque"
	// assetName is the standalone binary asset attached to each release.
	assetName = "porque.exe"
)

// Asset is a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is the subset of the GitHub release payload we use.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Latest fetches the most recent published (non-prerelease) release.
func Latest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases returned %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// AssetURL returns the download URL of the standalone binary, if present.
func (r *Release) AssetURL() string {
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, assetName) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// IsNewer reports whether latestTag (e.g. "v0.1.2") is a newer version than
// current (e.g. "v0.1.1" or "0.1.1"). A non-release current ("dev") is treated
// as never-newer by callers; unparseable inputs return false (no update).
func IsNewer(latestTag, current string) bool {
	l := parseVersion(latestTag)
	c := parseVersion(current)
	if l == nil || c == nil {
		return false
	}
	for i := 0; i < len(l) && i < len(c); i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return len(l) > len(c)
}

// parseVersion turns "v0.1.10" into [0 1 10]; returns nil on any non-numeric segment.
func parseVersion(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// DownloadAndApply downloads the release's standalone binary and replaces the
// running executable in place. On success the new version takes effect on the
// next launch (or after a restart). A permission error here typically means the
// app is installed to a protected location (e.g. Program Files) — callers
// should fall back to opening the release page.
func DownloadAndApply(ctx context.Context, rel *Release) error {
	url := rel.AssetURL()
	if url == "" {
		return fmt.Errorf("release %s has no %s asset", rel.TagName, assetName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned %d", assetName, resp.StatusCode)
	}

	if err := selfupdate.Apply(resp.Body, selfupdate.Options{}); err != nil {
		// Best-effort rollback if selfupdate left the binary in a bad state.
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("update failed and rollback failed: %w", rerr)
		}
		return err
	}
	return nil
}
