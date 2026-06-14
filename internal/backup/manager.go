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
	inst, err := s.store.LatestInstance(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv.State != db.StateRunning || inst == nil || inst.ContainerID == nil {
		return nil, apperr.BadState("server must be running to take a zero-downtime backup")
	}

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

	// Freeze saves, flush to disk, and guarantee re-enable on every exit path.
	// RCON is hosted locally on loopback port 25575.
	rc := rcon.New("127.0.0.1:25575", srv.RconPassword)
	if err := rc.SaveOff(ctx); err != nil {
		return nil, apperr.Internal(fmt.Errorf("RCON save-off failed: %w", err))
	}
	defer func() { _ = rc.SaveOn(ctx) }()
	if err := rc.SaveAll(ctx); err != nil {
		return nil, apperr.Internal(fmt.Errorf("RCON save-all failed: %w", err))
	}

	sha, err := createArchiveFromDir(srv.VolumeName, path)
	if err != nil {
		_ = os.Remove(path)
		return nil, apperr.Internal(fmt.Errorf("failed to create backup archive: %w", err))
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

	// Wipe existing volume contents so stale files don't survive the restore.
	entries, err := os.ReadDir(srv.VolumeName)
	if err == nil {
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(srv.VolumeName, entry.Name()))
		}
	}

	// Extract the archive back to the server directory natively
	if err := extractArchiveToDir(b.FilePath, srv.VolumeName); err != nil {
		return apperr.Internal(fmt.Errorf("failed to extract backup archive: %w", err))
	}

	return nil
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
