package db

import (
	"time"

	"github.com/google/uuid"
)

// ServerState enumerates the lifecycle states a server may occupy. These mirror
// the CHECK constraint on servers.state.
type ServerState string

const (
	StateCreating   ServerState = "creating"
	StateStarting   ServerState = "starting"
	StateRunning    ServerState = "running"
	StateStopping   ServerState = "stopping"
	StateStopped    ServerState = "stopped"
	StateCrashed    ServerState = "crashed"
	StateRecovering ServerState = "recovering"
	StateCorrupted  ServerState = "corrupted"
	StateUnknown    ServerState = "unknown"
)

// ServerType enumerates supported Minecraft server flavours (itzg TYPE values).
type ServerType string

const (
	TypeVanilla ServerType = "VANILLA"
	TypePaper   ServerType = "PAPER"
	TypeFabric  ServerType = "FABRIC"
	TypeForge   ServerType = "FORGE"
)

// User is an admin dashboard account.
type User struct {
	ID           uuid.UUID `db:"id" json:"id"`
	Username     string    `db:"username" json:"username"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// Server is the desired configuration plus current state of a Minecraft server.
// RconPassword is never serialised to API clients (json:"-").
type Server struct {
	ID            uuid.UUID   `db:"id" json:"id"`
	Name          string      `db:"name" json:"name"`
	ServerType    ServerType  `db:"server_type" json:"server_type"`
	Version       string      `db:"version" json:"version"`
	LoaderVersion *string     `db:"loader_version" json:"loader_version"`
	Image         string      `db:"image" json:"image"`
	MemoryMB      int         `db:"memory_mb" json:"memory_mb"`
	CPUCores      float64     `db:"cpu_cores" json:"cpu_cores"`
	RconPassword  string      `db:"rcon_password" json:"-"`
	VolumeName    string      `db:"volume_name" json:"volume_name"`
	Port          int         `db:"port" json:"port"`
	RconPort      int         `db:"rcon_port" json:"rcon_port"`
	State         ServerState `db:"state" json:"state"`
	Difficulty    string      `db:"difficulty" json:"difficulty"`
	OnlineMode    bool        `db:"online_mode" json:"online_mode"`
	MOTD          string      `db:"motd" json:"motd"`
	BackupEnabled         bool       `db:"backup_enabled" json:"backup_enabled"`
	BackupIntervalValue   int        `db:"backup_interval_value" json:"backup_interval_value"`
	BackupIntervalUnit    string     `db:"backup_interval_unit" json:"backup_interval_unit"`
	BackupKeep            int        `db:"backup_keep" json:"backup_keep"`
	BackupLastRun         *time.Time `db:"backup_last_run" json:"backup_last_run"`
	MaintenanceMode       bool       `db:"maintenance_mode" json:"-"`
	MaintenanceStart      *time.Time `db:"maintenance_start" json:"-"`
	MaintenanceEnd        *time.Time `db:"maintenance_end" json:"-"`
	MaintenanceReason     *string    `db:"maintenance_reason" json:"-"`
	MaintenanceBackup     bool       `db:"maintenance_backup" json:"-"`
	BackupIntervalMinutes int        `db:"backup_interval_minutes" json:"-"`
	CreatedAt     time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time   `db:"updated_at" json:"updated_at"`
}

// PlayitAccount stores a Playit.gg agent secret key (secret-key-per-sidecar).
type PlayitAccount struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	SecretKey string    `db:"secret_key" json:"-"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// TunnelStatus enumerates server-tunnel states (mirrors the 0002 CHECK).
type TunnelStatus string

const (
	TunnelStarting     TunnelStatus = "starting"
	TunnelConnected    TunnelStatus = "connected"
	TunnelDisconnected TunnelStatus = "disconnected"
	TunnelError        TunnelStatus = "error"
)

// ServerTunnel links a server to a Playit account + agent sidecar container.
type ServerTunnel struct {
	ID                 uuid.UUID    `db:"id" json:"id"`
	ServerID           uuid.UUID    `db:"server_id" json:"server_id"`
	PlayitAccountID    *uuid.UUID   `db:"playit_account_id" json:"playit_account_id"`
	PlayitTunnelID     *uuid.UUID   `db:"playit_tunnel_id" json:"playit_tunnel_id"`
	SidecarContainerID *string      `db:"sidecar_container_id" json:"sidecar_container_id"`
	PublicAddress      *string      `db:"public_address" json:"public_address"`
	Proto              string       `db:"proto" json:"proto"`
	Status             TunnelStatus `db:"status" json:"status"`
	Active             bool         `db:"active" json:"active"`
	CreatedAt          time.Time    `db:"created_at" json:"created_at"`
}

// ServerMetric is a timeseries sample of a server's resource usage.
type ServerMetric struct {
	ID          uuid.UUID `db:"id" json:"id"`
	ServerID    uuid.UUID `db:"server_id" json:"server_id"`
	CPUPct      float64   `db:"cpu_pct" json:"cpu_pct"`
	MemBytes    int64     `db:"mem_bytes" json:"mem_bytes"`
	PlayerCount int       `db:"player_count" json:"player_count"`
	MaxPlayers   int       `db:"max_players" json:"max_players"`
	LatencyMS    *int      `db:"latency_ms" json:"latency_ms"`
	StorageBytes int64     `db:"storage_bytes" json:"storage_bytes"`
	RecordedAt   time.Time `db:"recorded_at" json:"recorded_at"`
}

// BackupStatus enumerates backup integrity states (mirrors the CHECK).
type BackupStatus string

const (
	BackupPending   BackupStatus = "pending"
	BackupValidated BackupStatus = "validated"
	BackupCorrupted BackupStatus = "corrupted"
)

// Backup is a compressed, checksummed archive of a server's data volume.
type Backup struct {
	ID        uuid.UUID    `db:"id" json:"id"`
	ServerID  uuid.UUID    `db:"server_id" json:"server_id"`
	FilePath  string       `db:"file_path" json:"file_path"`
	SizeBytes int64        `db:"size_bytes" json:"size_bytes"`
	SHA256    string       `db:"sha256" json:"sha256"`
	Status    BackupStatus `db:"status" json:"status"`
	CreatedAt time.Time    `db:"created_at" json:"created_at"`
}

// ServerInstance tracks the running container backing a server.
type ServerInstance struct {
	ID           uuid.UUID  `db:"id"`
	ServerID     uuid.UUID  `db:"server_id"`
	ContainerID  *string    `db:"container_id"`
	MCHostPort   *int       `db:"mc_host_port"`
	RconHostPort *int       `db:"rcon_host_port"`
	StartedAt    *time.Time `db:"started_at"`
	StoppedAt    *time.Time `db:"stopped_at"`
	ExitCode     *int       `db:"exit_code"`
}

// AppLog represents a system or server event log entry.
type AppLog struct {
	ID         string    `db:"id" json:"id"`
	ServerID   string    `db:"server_id" json:"server_id"`
	ServerName string    `db:"server_name" json:"server_name"`
	Message    string    `db:"message" json:"message"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}
