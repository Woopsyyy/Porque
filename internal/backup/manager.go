package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
	"github.com/woopsy/porque/internal/rcon"
)

// ServerStopper stops a server so its volume can be safely rewritten.
type ServerStopper interface {
	EnsureStopped(ctx context.Context, id uuid.UUID) error
}

// Service orchestrates backup creation, validation, rotation, and restore.
type Service struct {
	store   *db.Store
	stopper ServerStopper
	root    string
	keep    int
}

// NewService wires a backup service.
func NewService(store *db.Store, stopper ServerStopper, root string, keep int) *Service {
	if keep < 1 {
		keep = 1
	}
	return &Service{store: store, stopper: stopper, root: root, keep: keep}
}

// List returns a server's backups (newest first).
func (s *Service) List(ctx context.Context, serverID uuid.UUID) ([]db.Backup, error) {
	if _, err := s.store.GetServer(ctx, serverID); err != nil {
		return nil, err
	}
	return s.store.ListBackups(ctx, serverID)
}

// Create performs a zero-downtime backup of a running server: it freezes world
// saving via RCON, streams the data volume to a compressed archive, verifies
// integrity, then re-enables saving. Active players are not disconnected.
func (s *Service) Create(ctx context.Context, serverID uuid.UUID) (*db.Backup, error) {
	srv, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	isOffline := srv.State != db.StateRunning

	dir := filepath.Join(s.root, slugify(srv.Name))
	serversPath, errSrv := s.store.GetSetting(ctx, "servers_path")
	withinSrv, errWithin := s.store.GetSetting(ctx, "backups_within_server")
	if errSrv == nil && serversPath != "" && (errWithin != nil || withinSrv != "false") {
		dir = filepath.Join(serversPath, slugify(srv.Name), "backups")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, apperr.Internal(err)
	}
	path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405Z")+".tar.gz")

	var sha string
	if isOffline {
		// Offline backup: directly compress the server volume
		var err error
		sha, err = createArchiveFromDir(srv.VolumeName, path)
		if err != nil {
			_ = os.Remove(path)
			return nil, apperr.Internal(fmt.Errorf("failed to create backup archive: %w", err))
		}
	} else {
		// Online/hot backup: zero-downtime backup with RCON
		inst, err := s.store.LatestInstance(ctx, serverID)
		if err != nil {
			return nil, err
		}
		if inst == nil || inst.ContainerID == nil {
			return nil, apperr.BadState("server must be running to take a zero-downtime backup")
		}

		rc := rcon.New(fmt.Sprintf("127.0.0.1:%d", srv.RconPort), srv.RconPassword)
		if err := rc.SaveOff(ctx); err != nil {
			return nil, apperr.Internal(fmt.Errorf("RCON save-off failed: %w", err))
		}
		savedOn := false
		defer func() {
			if !savedOn {
				_ = rc.SaveOn(ctx)
			}
		}()
		if err := rc.SaveAll(ctx); err != nil {
			return nil, apperr.Internal(fmt.Errorf("RCON save-all failed: %w", err))
		}

		time.Sleep(1 * time.Second)

		tempDir := srv.VolumeName + "_bak_tmp"
		_ = os.RemoveAll(tempDir)
		defer os.RemoveAll(tempDir)
		if err := copyDirFiltered(srv.VolumeName, tempDir); err != nil {
			return nil, apperr.Internal(fmt.Errorf("failed to snapshot world: %w", err))
		}

		_ = rc.SaveOn(ctx)
		savedOn = true

		var archErr error
		sha, archErr = createArchiveFromDir(tempDir, path)
		if archErr != nil {
			_ = os.Remove(path)
			return nil, apperr.Internal(fmt.Errorf("failed to create backup archive: %w", archErr))
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	b := &db.Backup{
		ServerID:  serverID,
		FilePath:  path,
		SizeBytes: info.Size(),
		SHA256:    sha,
		Status:    db.BackupPending,
	}
	if err := s.store.CreateBackup(ctx, b); err != nil {
		return nil, err
	}

	// Integrity check: a corrupt archive is recorded and discarded.
	if verr := validateArchive(path); verr != nil {
		_ = s.store.InsertBackupValidation(ctx, b.ID, false, verr.Error())
		_ = s.store.UpdateBackupStatus(ctx, b.ID, db.BackupCorrupted)
		_ = os.Remove(path)
		b.Status = db.BackupCorrupted
		return b, apperr.Wrap(apperr.KindInternal, "backup failed integrity check", verr)
	}
	_ = s.store.InsertBackupValidation(ctx, b.ID, true, "")
	_ = s.store.UpdateBackupStatus(ctx, b.ID, db.BackupValidated)
	b.Status = db.BackupValidated

	s.rotate(ctx, serverID)
	return b, nil
}

// Restore stops the server, wipes its data volume, and extracts a validated
// backup back into it natively.
func (s *Service) Restore(ctx context.Context, serverID, backupID uuid.UUID) error {
	srv, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}
	b, err := s.store.GetBackup(ctx, backupID)
	if err != nil {
		return err
	}
	if b.ServerID != serverID {
		return apperr.Validation("backup does not belong to this server")
	}
	if b.Status != db.BackupValidated {
		return apperr.BadState("refusing to restore a non-validated backup")
	}
	// Re-verify the on-disk checksum before destroying current state.
	if sum, err := sha256File(b.FilePath); err != nil || sum != b.SHA256 {
		return apperr.BadState("backup checksum mismatch; refusing to restore")
	}

	if err := s.stopper.EnsureStopped(ctx, serverID); err != nil {
		return err
	}

	oldDir := srv.VolumeName + "_old"
	_ = os.RemoveAll(oldDir) // clear any leftover from a previous crashed restore

	// If the backup archive lives inside the server directory, it moves with the
	// rename below — track its new location and preserve the backups folder.
	archiveSource := b.FilePath
	backupsInside := false
	if rel, relErr := filepath.Rel(srv.VolumeName, b.FilePath); relErr == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		backupsInside = true
		archiveSource = filepath.Join(oldDir, rel)
	}

	// Move the current server directory aside instead of wiping it, so we can
	// roll back if extraction fails partway (full disk, locked file, etc.).
	if err := os.Rename(srv.VolumeName, oldDir); err != nil {
		if !os.IsNotExist(err) {
			return apperr.Internal(fmt.Errorf("failed to set aside current server dir: %w", err))
		}
		// Nothing to set aside — extract fresh from the original archive path.
		if mkErr := os.MkdirAll(srv.VolumeName, 0o755); mkErr != nil {
			return apperr.Internal(mkErr)
		}
		if exErr := extractArchiveToDir(b.FilePath, srv.VolumeName); exErr != nil {
			return apperr.Internal(fmt.Errorf("failed to extract backup archive: %w", exErr))
		}
		return nil
	}

	if err := os.MkdirAll(srv.VolumeName, 0o755); err != nil {
		_ = os.Rename(oldDir, srv.VolumeName) // roll back
		return apperr.Internal(err)
	}

	if err := extractArchiveToDir(archiveSource, srv.VolumeName); err != nil {
		// Roll back to the original state.
		_ = os.RemoveAll(srv.VolumeName)
		_ = os.Rename(oldDir, srv.VolumeName)
		return apperr.Internal(fmt.Errorf("failed to extract backup archive: %w", err))
	}

	// Preserve other backups stored inside the server directory.
	if backupsInside {
		oldBackups := filepath.Join(oldDir, "backups")
		if _, statErr := os.Stat(oldBackups); statErr == nil {
			_ = os.RemoveAll(filepath.Join(srv.VolumeName, "backups"))
			_ = os.Rename(oldBackups, filepath.Join(srv.VolumeName, "backups"))
		}
	}

	_ = os.RemoveAll(oldDir)
	return nil
}

// Delete removes a single backup's archive file from disk and its DB row.
func (s *Service) Delete(ctx context.Context, backupID uuid.UUID) error {
	b, err := s.store.GetBackup(ctx, backupID)
	if err != nil {
		return err
	}
	if err := os.Remove(b.FilePath); err != nil && !os.IsNotExist(err) {
		return apperr.Internal(err)
	}
	return s.store.DeleteBackup(ctx, backupID)
}

// PurgeServer removes all backup archives for a server from disk. Backup DB
// rows are removed by the server delete cascade; this clears the files.
func (s *Service) PurgeServer(serverName string) {
	_ = os.RemoveAll(filepath.Join(s.root, slugify(serverName)))
}

// rotate deletes the oldest backups beyond the retention limit (files + rows).
// The per-server backup_keep wins over the global default when set.
func (s *Service) rotate(ctx context.Context, serverID uuid.UUID) {
	keep := s.keep
	if srv, err := s.store.GetServer(ctx, serverID); err == nil && srv.BackupKeep > 0 {
		keep = srv.BackupKeep
	}
	backups, err := s.store.ListBackups(ctx, serverID) // newest first
	if err != nil || len(backups) <= keep {
		return
	}
	for _, old := range backups[keep:] {
		_ = os.Remove(old.FilePath)
		_ = s.store.DeleteBackup(ctx, old.ID)
	}
}

func slugify(name string) string {
	var sb strings.Builder
	lastWasDash := true
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			lastWasDash = false
		} else if !lastWasDash {
			sb.WriteRune('-')
			lastWasDash = true
		}
	}
	res := strings.TrimSuffix(sb.String(), "-")
	if len(res) == 0 {
		return "server"
	}
	if len(res) > 32 {
		res = res[:32]
		res = strings.TrimSuffix(res, "-")
	}
	return res
}
