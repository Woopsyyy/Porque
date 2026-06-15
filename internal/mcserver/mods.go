package mcserver

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
)

// ModFile is an uploaded mod/plugin jar, referenced by its source path on disk
// so it can be streamed into place rather than buffered in memory.
type ModFile struct {
	Name string // destination filename
	Path string // source path on disk
}

// ModInfo describes an installed mod/plugin file.
type ModInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// modsFolder returns the subfolder for a server type: Paper loads
// plugins/, mod loaders load mods/.
func modsFolder(t db.ServerType) string {
	if t == db.TypePaper {
		return "plugins"
	}
	return "mods"
}

// ModsFolder returns the folder mods are installed into for a server.
func (c *Controller) ModsFolder(ctx context.Context, serverID uuid.UUID) (string, error) {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return "", err
	}
	return modsFolder(srv.ServerType), nil
}

// UploadMods writes the given jar files into the server's mods/plugins folder.
func (c *Controller) UploadMods(ctx context.Context, serverID uuid.UUID, files []ModFile) error {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	folder := modsFolder(srv.ServerType)
	targetDir := filepath.Join(srv.VolumeName, folder)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return apperr.Internal(err)
	}

	wrote := 0
	for _, f := range files {
		name := sanitizeFilename(f.Name)
		if name == "" || !strings.HasSuffix(strings.ToLower(name), ".jar") {
			continue
		}
		if err := copyModFile(f.Path, filepath.Join(targetDir, name)); err != nil {
			return apperr.Internal(err)
		}
		wrote++
	}

	if wrote == 0 {
		return apperr.Validation("no .jar files in upload")
	}

	return nil
}

// copyModFile streams a jar from src to dst without buffering it in memory.
func copyModFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ListMods lists the jar files installed in the server's mods/plugins folder.
func (c *Controller) ListMods(ctx context.Context, serverID uuid.UUID) ([]ModInfo, string, error) {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, "", err
	}
	folder := modsFolder(srv.ServerType)
	targetDir := filepath.Join(srv.VolumeName, folder)

	var mods []ModInfo
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, folder, nil // treat as empty rather than erroring the UI
		}
		return nil, folder, apperr.Internal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jar") {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			mods = append(mods, ModInfo{Name: name, Size: info.Size()})
		}
	}

	return mods, folder, nil
}

// DeleteMod removes a single jar from the server's mods/plugins folder.
func (c *Controller) DeleteMod(ctx context.Context, serverID uuid.UUID, name string) error {
	name = sanitizeFilename(name)
	if name == "" {
		return apperr.Validation("invalid mod name")
	}
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	folder := modsFolder(srv.ServerType)
	targetPath := filepath.Join(srv.VolumeName, folder, name)

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return apperr.Internal(err)
	}

	return nil
}

// sanitizeFilename strips any path components and rejects traversal.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return ""
	}
	return name
}
