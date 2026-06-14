package mcserver

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
)

// ImportParams describe an existing server uploaded as a ZIP archive.
type ImportParams struct {
	Name          string
	Type          db.ServerType
	Version       string
	LoaderVersion string
	MemoryMB      int
	CPUCores      float64
}

// ImportFromZip creates a server record and seeds its data volume from a ZIP
// archive, extracting the contents directly into the server directory.
// A single common top-level directory in the archive is stripped so files land
// at the volume root. The server is left stopped.
func (c *Controller) ImportFromZip(ctx context.Context, p ImportParams, ra io.ReaderAt, size int64) (*db.Server, error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, apperr.Validation("uploaded file is not a valid ZIP archive")
	}
	prefix := detectStripPrefix(zr.File)

	srv, err := c.Create(ctx, CreateParams{
		Name:          p.Name,
		Type:          p.Type,
		Version:       p.Version,
		LoaderVersion: p.LoaderVersion,
		MemoryMB:      p.MemoryMB,
	})
	if err != nil {
		return nil, err
	}

	// Extract zip entries directly to srv.VolumeName
	for _, f := range zr.File {
		raw := normalizeZipName(f.Name)
		name := strings.TrimPrefix(raw, prefix)
		if name == "" {
			continue
		}

		target := filepath.Join(srv.VolumeName, name)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				c.transition(ctx, srv, db.StateCorrupted, "import: failed to create folder: "+err.Error())
				return nil, apperr.Internal(err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			c.transition(ctx, srv, db.StateCorrupted, "import: failed to create folder: "+err.Error())
			return nil, apperr.Internal(err)
		}

		rc, err := f.Open()
		if err != nil {
			c.transition(ctx, srv, db.StateCorrupted, "import: failed to open zip entry: "+err.Error())
			return nil, apperr.Internal(err)
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			c.transition(ctx, srv, db.StateCorrupted, "import: failed to create file: "+err.Error())
			return nil, apperr.Internal(err)
		}

		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			c.transition(ctx, srv, db.StateCorrupted, "import: failed to write file: "+err.Error())
			return nil, apperr.Internal(err)
		}

		rc.Close()
		out.Close()
	}

	c.transition(ctx, srv, db.StateStopped, "imported from zip archive")
	return srv, nil
}

// normalizeZipName converts backslash separators (e.g. from Windows
// PowerShell's Compress-Archive) to the forward slashes tar expects.
func normalizeZipName(name string) string {
	return strings.ReplaceAll(name, "\\", "/")
}

// detectStripPrefix returns a single common top-level directory shared by all
// file entries (e.g. "myserver/"), or "" when files live at the archive root.
func detectStripPrefix(files []*zip.File) string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, normalizeZipName(f.Name))
	}
	return stripPrefixForNames(names)
}

// stripPrefixForNames holds the pure logic, operating on normalized names.
func stripPrefixForNames(names []string) string {
	root := ""
	seen := false
	for _, name := range names {
		if strings.HasSuffix(name, "/") {
			continue // ignore directory entries
		}
		seg := name
		if i := strings.IndexByte(seg, '/'); i >= 0 {
			seg = seg[:i]
		} else {
			return "" // a file at the archive root => nothing to strip
		}
		if !seen {
			root, seen = seg, true
		} else if seg != root {
			return ""
		}
	}
	if !seen || root == "" {
		return ""
	}
	return root + "/"
}

// DetectServerType inspects a host server directory natively in Go and returns
// the detected server type and best-effort version. Empty type means detection was inconclusive.
func (c *Controller) DetectServerType(ctx context.Context, hostPath string) (db.ServerType, string) {
	if strings.TrimSpace(hostPath) == "" {
		return "", ""
	}

	files, err := os.ReadDir(hostPath)
	if err != nil {
		return "", ""
	}

	hasFabric := false
	hasForge := false
	hasPaper := false
	hasJar := false

	for _, file := range files {
		name := strings.ToLower(file.Name())
		if file.IsDir() {
			if name == ".fabric" {
				hasFabric = true
			} else if name == "plugins" {
				hasPaper = true
			}
		} else {
			if name == "fabric-server-launch.jar" || name == "fabric-server-launcher.properties" || (strings.HasPrefix(name, "fabric") && strings.HasSuffix(name, ".jar")) {
				hasFabric = true
			} else if (strings.HasPrefix(name, "forge-") && strings.HasSuffix(name, ".jar")) || (strings.HasPrefix(name, "forge") && strings.HasSuffix(name, ".jar")) {
				hasForge = true
			} else if (strings.HasPrefix(name, "paper-") && strings.HasSuffix(name, ".jar")) || name == "version_history.json" {
				hasPaper = true
			} else if strings.HasSuffix(name, ".jar") {
				hasJar = true
			}
		}
	}

	if !hasForge {
		if _, err := os.Stat(filepath.Join(hostPath, "libraries", "net", "minecraftforge")); err == nil {
			hasForge = true
		}
	}

	var t db.ServerType = db.TypeVanilla
	if hasFabric {
		t = db.TypeFabric
	} else if hasForge {
		t = db.TypeForge
	} else if hasPaper {
		t = db.TypePaper
	} else if hasJar {
		t = db.TypeVanilla
	}

	// Extract version
	ver := ""
	versionHistoryPath := filepath.Join(hostPath, "version_history.json")
	if data, err := os.ReadFile(versionHistoryPath); err == nil {
		re := regexp.MustCompile(`MC: ([0-9][0-9.]*)`)
		if m := re.FindSubmatch(data); len(m) > 1 {
			ver = string(m[1])
		}
	}

	if ver == "" {
		re := regexp.MustCompile(`([0-9][0-9]*\.[0-9][0-9]*(\.[0-9][0-9]*)?)`)
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".jar") {
				if m := re.FindString(file.Name()); m != "" {
					ver = m
					break
				}
			}
		}
	}

	return t, ver
}

// ImportFromHostPath creates a server record pointing directly to an existing directory on the host.
func (c *Controller) ImportFromHostPath(ctx context.Context, p CreateParams, hostPath string) (*db.Server, error) {
	if len(strings.TrimSpace(p.Name)) < 1 || len(p.Name) > 64 {
		return nil, apperr.Validation("name must be between 1 and 64 characters")
	}
	if hostPath == "" {
		return nil, apperr.Validation("host_path is required")
	}

	rconPw, err := randomSecret(16)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	srv := &db.Server{
		Name:         p.Name,
		ServerType:   p.Type,
		Version:      p.Version,
		MemoryMB:     p.MemoryMB,
		CPUCores:     AutoCPUCores(p.MemoryMB),
		RconPassword: rconPw,
		VolumeName:   hostPath,
		State:        db.StateStopped,
	}
	if p.LoaderVersion != "" {
		srv.LoaderVersion = &p.LoaderVersion
	}

	if err := c.store.CreateServer(ctx, srv); err != nil {
		return nil, err
	}

	c.transition(ctx, srv, db.StateStopped, "imported from host path")
	return srv, nil
}
