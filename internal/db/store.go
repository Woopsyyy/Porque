package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/woopsy/porque/internal/apperr"
)

// Store provides data-access methods over a sqlx connection pool.
type Store struct {
	db *sqlx.DB
}

// NewStore wraps a connection pool.
func NewStore(conn *sqlx.DB) *Store { return &Store{db: conn} }

// Ping verifies database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ----- users ---------------------------------------------------------------

// CountUsers returns the number of admin users.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.GetContext(ctx, &n, `SELECT count(*) FROM users`); err != nil {
		return 0, apperr.Internal(err)
	}
	return n, nil
}

// CreateUser inserts an admin user and returns it.
func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	u := &User{
		ID:           uuid.New(),
		Username:     username,
		PasswordHash: passwordHash,
	}
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)
		 RETURNING created_at, updated_at`,
		u.ID, username, passwordHash,
	).Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return u, nil
}

// GetUserByUsername looks up a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.GetContext(ctx, &u, `SELECT * FROM users WHERE username = ?`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("user not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &u, nil
}

// ----- servers -------------------------------------------------------------

// CreateServer inserts a server row, populating generated fields on the struct.
func (s *Store) CreateServer(ctx context.Context, srv *Server) error {
	srv.ID = uuid.New()
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO servers
		   (id, name, server_type, version, loader_version, image, memory_mb, cpu_cores,
		    rcon_password, volume_name, state)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 RETURNING created_at, updated_at`,
		srv.ID, srv.Name, srv.ServerType, srv.Version, srv.LoaderVersion, srv.Image,
		srv.MemoryMB, srv.CPUCores, srv.RconPassword, srv.VolumeName, srv.State,
	).Scan(&srv.CreatedAt, &srv.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return apperr.Conflict(fmt.Sprintf("server name %q already exists", srv.Name))
		}
		return apperr.Internal(err)
	}
	return nil
}

// GetServer fetches a server by id.
func (s *Store) GetServer(ctx context.Context, id uuid.UUID) (*Server, error) {
	var srv Server
	err := s.db.GetContext(ctx, &srv, `SELECT * FROM servers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("server not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &srv, nil
}

// ListServers returns all servers ordered by creation time.
func (s *Store) ListServers(ctx context.Context) ([]Server, error) {
	var out []Server
	if err := s.db.SelectContext(ctx, &out, `SELECT * FROM servers ORDER BY created_at DESC`); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// UpdateServerState transitions a server's state and bumps updated_at.
func (s *Store) UpdateServerState(ctx context.Context, id uuid.UUID, state ServerState) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, state, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// DeleteServer removes a server (cascades to instances, events, etc.).
func (s *Store) DeleteServer(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.NotFound("server not found")
	}
	return nil
}

// ----- server instances ----------------------------------------------------

// CreateInstance records a new runtime container instance for a server.
func (s *Store) CreateInstance(ctx context.Context, inst *ServerInstance) error {
	inst.ID = uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO server_instances
		   (id, server_id, container_id, mc_host_port, rcon_host_port, started_at)
		 VALUES (?,?,?,?,?,?)`,
		inst.ID, inst.ServerID, inst.ContainerID, inst.MCHostPort, inst.RconHostPort, inst.StartedAt,
	)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// LatestInstance returns the most recent instance for a server, or nil.
func (s *Store) LatestInstance(ctx context.Context, serverID uuid.UUID) (*ServerInstance, error) {
	var inst ServerInstance
	err := s.db.GetContext(ctx, &inst,
		`SELECT * FROM server_instances WHERE server_id = ?
		 ORDER BY started_at DESC LIMIT 1`, serverID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &inst, nil
}

// MarkInstanceStopped records the stop time and exit code of an instance.
func (s *Store) MarkInstanceStopped(ctx context.Context, instanceID uuid.UUID, exitCode *int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE server_instances SET stopped_at = ?, exit_code = ? WHERE id = ?`,
		time.Now(), exitCode, instanceID)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ----- playit accounts ------------------------------------------------------

// CreatePlayitAccount stores a Playit agent secret key.
func (s *Store) CreatePlayitAccount(ctx context.Context, name, secretKey string) (*PlayitAccount, error) {
	// Enforce single connected account by deleting any existing playit accounts
	_, err := s.db.ExecContext(ctx, `DELETE FROM playit_accounts`)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	a := &PlayitAccount{ID: uuid.New(), Name: name, SecretKey: secretKey, Status: "active"}
	err = s.db.QueryRowxContext(ctx,
		`INSERT INTO playit_accounts (id, name, secret_key) VALUES (?,?,?)
		 RETURNING status, created_at, updated_at`,
		a.ID, name, secretKey,
	).Scan(&a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return a, nil
}

// ListPlayitAccounts returns all accounts.
func (s *Store) ListPlayitAccounts(ctx context.Context) ([]PlayitAccount, error) {
	var out []PlayitAccount
	if err := s.db.SelectContext(ctx, &out, `SELECT * FROM playit_accounts ORDER BY created_at DESC`); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// GetPlayitAccount fetches an account by id.
func (s *Store) GetPlayitAccount(ctx context.Context, id uuid.UUID) (*PlayitAccount, error) {
	var a PlayitAccount
	err := s.db.GetContext(ctx, &a, `SELECT * FROM playit_accounts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("playit account not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &a, nil
}

// DeletePlayitAccount removes an account.
func (s *Store) DeletePlayitAccount(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM playit_accounts WHERE id = ?`, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.NotFound("playit account not found")
	}
	return nil
}

// ----- server tunnels -------------------------------------------------------

// CreateServerTunnel records an attached tunnel + sidecar for a server.
func (s *Store) CreateServerTunnel(ctx context.Context, t *ServerTunnel) error {
	t.ID = uuid.New()
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO server_tunnels
		   (id, server_id, playit_account_id, sidecar_container_id, public_address, status, active)
		 VALUES (?,?,?,?,?,?,?)
		 RETURNING created_at`,
		t.ID, t.ServerID, t.PlayitAccountID, t.SidecarContainerID, t.PublicAddress, t.Status, t.Active,
	).Scan(&t.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return apperr.Conflict("server already has an active tunnel")
		}
		return apperr.Internal(err)
	}
	return nil
}

// ActiveServerTunnel returns the active tunnel for a server, or nil.
func (s *Store) ActiveServerTunnel(ctx context.Context, serverID uuid.UUID) (*ServerTunnel, error) {
	var t ServerTunnel
	err := s.db.GetContext(ctx, &t,
		`SELECT * FROM server_tunnels WHERE server_id = ? AND active ORDER BY created_at DESC LIMIT 1`, serverID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &t, nil
}

// ListActiveTunnels returns all active server tunnels.
func (s *Store) ListActiveTunnels(ctx context.Context) ([]ServerTunnel, error) {
	var out []ServerTunnel
	if err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM server_tunnels WHERE active ORDER BY created_at DESC`); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// UpdateServerTunnel sets the status and (optional) discovered public address.
func (s *Store) UpdateServerTunnel(ctx context.Context, id uuid.UUID, status TunnelStatus, publicAddr *string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE server_tunnels SET status = ?, public_address = COALESCE(?, public_address) WHERE id = ?`,
		status, publicAddr, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// DeactivateServerTunnels marks a server's tunnels inactive (on detach).
func (s *Store) DeactivateServerTunnels(ctx context.Context, serverID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE server_tunnels SET active = 0, status = 'disconnected' WHERE server_id = ? AND active`, serverID)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ActiveServerTunnels returns all active tunnels for a server.
func (s *Store) ActiveServerTunnels(ctx context.Context, serverID uuid.UUID) ([]ServerTunnel, error) {
	var out []ServerTunnel
	if err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM server_tunnels WHERE server_id = ? AND active ORDER BY created_at DESC`, serverID); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// ActiveTunnelByProto returns the active tunnel for a server with the specified protocol.
func (s *Store) ActiveTunnelByProto(ctx context.Context, serverID uuid.UUID, proto string) (*ServerTunnel, error) {
	var t ServerTunnel
	err := s.db.GetContext(ctx, &t,
		`SELECT * FROM server_tunnels WHERE server_id = ? AND proto = ? AND active LIMIT 1`, serverID, proto)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &t, nil
}

// DeactivateServerTunnelByProto deactivates a specific protocol's tunnel for a server.
func (s *Store) DeactivateServerTunnelByProto(ctx context.Context, serverID uuid.UUID, proto string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE server_tunnels SET active = 0, status = 'disconnected' WHERE server_id = ? AND proto = ? AND active`, serverID, proto)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ----- backups -------------------------------------------------------------

// CreateBackup inserts a backup row, populating id and created_at.
func (s *Store) CreateBackup(ctx context.Context, b *Backup) error {
	b.ID = uuid.New()
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO backups (id, server_id, file_path, size_bytes, sha256, status)
		 VALUES (?,?,?,?,?,?)
		 RETURNING created_at`,
		b.ID, b.ServerID, b.FilePath, b.SizeBytes, b.SHA256, b.Status,
	).Scan(&b.CreatedAt)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// GetBackup fetches a backup by id.
func (s *Store) GetBackup(ctx context.Context, id uuid.UUID) (*Backup, error) {
	var b Backup
	err := s.db.GetContext(ctx, &b, `SELECT * FROM backups WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("backup not found")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &b, nil
}

// ListBackups returns a server's backups, newest first.
func (s *Store) ListBackups(ctx context.Context, serverID uuid.UUID) ([]Backup, error) {
	var out []Backup
	err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM backups WHERE server_id = ? ORDER BY created_at DESC`, serverID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// UpdateBackupStatus sets a backup's integrity status.
func (s *Store) UpdateBackupStatus(ctx context.Context, id uuid.UUID, status BackupStatus) error {
	_, err := s.db.ExecContext(ctx, `UPDATE backups SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// DeleteBackup removes a backup row.
func (s *Store) DeleteBackup(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// InsertBackupValidation records an integrity-check result.
func (s *Store) InsertBackupValidation(ctx context.Context, backupID uuid.UUID, valid bool, validationErr string) error {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO backup_validations (id, backup_id, is_valid, error) VALUES (?,?,?,?)`,
		id, backupID, valid, nullIfEmpty(validationErr))
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ----- worker: recovery, metrics, running servers --------------------------

// RunningServers returns all servers currently in the running state.
func (s *Store) RunningServers(ctx context.Context) ([]Server, error) {
	var out []Server
	if err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM servers WHERE state = ?`, StateRunning); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// InsertRecoveryEvent records a self-healing action in the audit log.
func (s *Store) InsertRecoveryEvent(ctx context.Context, serverID uuid.UUID, eventType, action, details string) error {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO recovery_events (id, server_id, event_type, action_taken, details)
		 VALUES (?,?,?,?,?)`, id, serverID, eventType, action, nullIfEmpty(details))
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ListMetrics returns the most recent metric samples for a server (newest
// first), capped at limit.
func (s *Store) ListMetrics(ctx context.Context, serverID uuid.UUID, limit int) ([]ServerMetric, error) {
	if limit <= 0 || limit > 1000 {
		limit = 120
	}
	var out []ServerMetric
	err := s.db.SelectContext(ctx, &out,
		`SELECT * FROM server_metrics WHERE server_id = ? ORDER BY recorded_at DESC LIMIT ?`,
		serverID, limit)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// InsertMetric stores a timeseries sample for a server.
func (s *Store) InsertMetric(ctx context.Context, serverID uuid.UUID, cpuPct float64, memBytes int64, players, maxPlayers int, latency *int, storage int64) error {
	id := uuid.New()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO server_metrics (id, server_id, cpu_pct, mem_bytes, player_count, max_players, latency_ms, storage_bytes)
		 VALUES (?,?,?,?,?,?,?,?)`, id, serverID, cpuPct, memBytes, players, maxPlayers, latency, storage)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// DueBackups returns running servers that are due for a scheduled backup.
func (s *Store) DueBackups(ctx context.Context) ([]Server, error) {
	var out []Server
	query := `
		SELECT * FROM servers
		WHERE backup_enabled = 1
		  AND state = 'running'
		  AND (
		      backup_last_run IS NULL
		      OR datetime(backup_last_run, '+' || backup_interval_minutes || ' minutes') <= datetime('now')
		  )
	`
	if err := s.db.SelectContext(ctx, &out, query); err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// MarkBackupRun updates backup_last_run to CURRENT_TIMESTAMP for the server.
func (s *Store) MarkBackupRun(ctx context.Context, serverID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET backup_last_run = CURRENT_TIMESTAMP WHERE id = ?`, serverID)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}


// ----- state events --------------------------------------------------------

// InsertStateEvent appends a lifecycle transition to the audit log.
func (s *Store) InsertStateEvent(ctx context.Context, serverID uuid.UUID, from, to ServerState, message string) error {
	id := uuid.New()
	var fromVal interface{}
	if from != "" {
		fromVal = from
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO server_state_events (id, server_id, from_state, to_state, message)
		 VALUES (?,?,?,?,?)`, id, serverID, fromVal, to, nullIfEmpty(message))
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// UpdateServerConfig updates the dynamic settings of a server.
func (s *Store) UpdateServerConfig(ctx context.Context, id uuid.UUID, difficulty string, onlineMode bool, motd string, memoryMB int, cpuCores float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers
		 SET difficulty = ?, online_mode = ?, motd = ?, memory_mb = ?, cpu_cores = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, difficulty, onlineMode, motd, memoryMB, cpuCores, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// UpdateBackupSchedule updates the backup configuration for a server.
func (s *Store) UpdateBackupSchedule(ctx context.Context, id uuid.UUID, enabled bool, intervalMinutes int, keep int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers
		 SET backup_enabled = ?, backup_interval_minutes = ?, backup_keep = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, enabled, intervalMinutes, keep, id)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// ----- helpers -------------------------------------------------------------

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// isUniqueViolation reports whether err is an SQLite unique-constraint error.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// GetSetting retrieves a configuration setting value by key.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var val string
	err := s.db.GetContext(ctx, &val, `SELECT value FROM settings WHERE key = ?`, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", apperr.NotFound("setting not found")
	}
	if err != nil {
		return "", apperr.Internal(err)
	}
	return val, nil
}

// SetSetting upserts a configuration setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, value)
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}
