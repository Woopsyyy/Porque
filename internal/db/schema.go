package db

const Schema = `
-- Logical Minecraft servers (desired configuration + current state).
CREATE TABLE IF NOT EXISTS servers (
    id             TEXT PRIMARY KEY,
    name           TEXT UNIQUE NOT NULL,
    server_type    TEXT NOT NULL CHECK (server_type IN ('VANILLA','PAPER','FABRIC','FORGE')),
    version        TEXT NOT NULL,                       -- Minecraft version, e.g. 1.20.4
    loader_version TEXT,                                -- optional Fabric loader / Forge version
    image          TEXT NOT NULL DEFAULT 'itzg/minecraft-server',
    memory_mb      INTEGER NOT NULL DEFAULT 2048 CHECK (memory_mb >= 512),
    cpu_cores      REAL NOT NULL DEFAULT 1.0 CHECK (cpu_cores > 0),
    rcon_password  TEXT NOT NULL,                       -- generated server-side, never logged
    volume_name    TEXT NOT NULL,                       -- path where server files live
    state          TEXT NOT NULL DEFAULT 'creating'
                   CHECK (state IN ('creating','starting','running','stopping','stopped',
                                    'crashed','recovering','corrupted','unknown')),
    difficulty     TEXT NOT NULL DEFAULT 'normal',
    online_mode    BOOLEAN NOT NULL DEFAULT 1,
    motd           TEXT NOT NULL DEFAULT 'A Minecraft Server',
    backup_enabled          BOOLEAN NOT NULL DEFAULT 0,
    backup_interval_minutes INTEGER NOT NULL DEFAULT 360,
    backup_keep             INTEGER NOT NULL DEFAULT 5,
    backup_last_run         DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Admin users for dashboard authentication.
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Runtime process/instance tracking for a server (one active row per running server).
CREATE TABLE IF NOT EXISTS server_instances (
    id             TEXT PRIMARY KEY,
    server_id      TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    container_id   TEXT,                                -- stores the OS Process ID (PID)
    mc_host_port   INTEGER,                             -- local game port (usually 25565)
    rcon_host_port INTEGER,                             -- local RCON port
    started_at     DATETIME,
    stopped_at     DATETIME,
    exit_code      INTEGER
);
CREATE INDEX IF NOT EXISTS idx_server_instances_server ON server_instances(server_id);

-- Append-only audit of lifecycle state transitions.
CREATE TABLE IF NOT EXISTS server_state_events (
    id         TEXT PRIMARY KEY,
    server_id  UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    from_state TEXT,
    to_state   TEXT NOT NULL,
    message    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_server_state_events_server ON server_state_events(server_id, created_at DESC);

-- Playit.gg credentials (secret-key-per-agent model).
CREATE TABLE IF NOT EXISTS playit_accounts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    secret_key TEXT NOT NULL,                           -- PLAYIT_SECRET_KEY, never logged
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','invalid','rate_limited')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Tunnels created/claimed on Playit.gg.
CREATE TABLE IF NOT EXISTS playit_tunnels (
    id                TEXT PRIMARY KEY,
    playit_account_id TEXT NOT NULL REFERENCES playit_accounts(id) ON DELETE CASCADE,
    tunnel_id         TEXT UNIQUE NOT NULL,             -- id from Playit.gg
    name              TEXT,
    public_address    TEXT NOT NULL,                    -- e.g. link.ply.gg:12345
    proto             TEXT NOT NULL DEFAULT 'tcp' CHECK (proto IN ('tcp','udp')),
    local_port        INTEGER NOT NULL,
    status            TEXT NOT NULL DEFAULT 'disconnected'
                      CHECK (status IN ('connected','disconnected','expired')),
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Mapping of a server to a Playit tunnel + its background process.
CREATE TABLE IF NOT EXISTS server_tunnels (
    id                   TEXT PRIMARY KEY,
    server_id            TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    playit_tunnel_id     TEXT REFERENCES playit_tunnels(id) ON DELETE CASCADE,
    playit_account_id    TEXT REFERENCES playit_accounts(id) ON DELETE SET NULL,
    sidecar_container_id TEXT,                          -- stores playit background process PID
    public_address       TEXT,                          -- routing address, e.g. link.ply.gg:12345
    proto                TEXT NOT NULL DEFAULT 'tcp',
    status               TEXT NOT NULL DEFAULT 'starting'
                         CHECK (status IN ('starting','connected','disconnected','error')),
    active               BOOLEAN NOT NULL DEFAULT 1,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (server_id, proto)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_server_tunnels_proto ON server_tunnels(server_id, proto) WHERE active;

-- Backup archives.
CREATE TABLE IF NOT EXISTS backups (
    id         TEXT PRIMARY KEY,
    server_id  TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    sha256     TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','validated','corrupted')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_backups_server ON backups(server_id, created_at DESC);

-- Integrity validation results for a backup.
CREATE TABLE IF NOT EXISTS backup_validations (
    id           TEXT PRIMARY KEY,
    backup_id    TEXT NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
    is_valid     BOOLEAN NOT NULL,
    error        TEXT,
    validated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Self-healing audit log.
CREATE TABLE IF NOT EXISTS recovery_events (
    id           TEXT PRIMARY KEY,
    server_id    TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,                         -- crash, stuck_heartbeat, network_failure
    action_taken TEXT NOT NULL,                         -- restart_process, restore_backup
    details      TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_recovery_events_server ON recovery_events(server_id, created_at DESC);

-- Timeseries metrics.
CREATE TABLE IF NOT EXISTS server_metrics (
    id           TEXT PRIMARY KEY,
    server_id    TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_pct      REAL NOT NULL,
    mem_bytes    INTEGER NOT NULL,
    player_count INTEGER NOT NULL DEFAULT 0,
    max_players  INTEGER NOT NULL DEFAULT 20,
    latency_ms   INTEGER,
    storage_bytes INTEGER NOT NULL DEFAULT 0,
    recorded_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_server_metrics_recorded ON server_metrics(server_id, recorded_at DESC);

-- Self-contained settings table.
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Crash/system event logs.
CREATE TABLE IF NOT EXISTS app_logs (
    id          TEXT PRIMARY KEY,
    server_id   TEXT REFERENCES servers(id) ON DELETE CASCADE,
    server_name TEXT NOT NULL,
    message     TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_app_logs_created ON app_logs(created_at DESC);
`
