import * as App from "../../wailsjs/go/main/App";

// ----- types (mirror the Go API JSON) --------------------------------------

export type ServerState =
  | "creating"
  | "starting"
  | "running"
  | "stopping"
  | "stopped"
  | "crashed"
  | "recovering"
  | "corrupted"
  | "unknown";

export type ServerType = "VANILLA" | "PAPER" | "FABRIC" | "FORGE";

export interface Server {
  id: string;
  name: string;
  server_type: ServerType;
  version: string;
  loader_version: string | null;
  image: string;
  memory_mb: number;
  cpu_cores: number;
  volume_name: string;
  state: ServerState;
  difficulty: string;
  online_mode: boolean;
  motd: string;
  backup_enabled: boolean;
  backup_interval_minutes: number;
  backup_keep: number;
  backup_last_run: string | null;
  created_at: string;
  updated_at: string;
}

export type BackupStatus = "pending" | "validated" | "corrupted";

export interface Backup {
  id: string;
  server_id: string;
  file_path: string;
  size_bytes: number;
  sha256: string;
  status: BackupStatus;
  created_at: string;
}

export type TunnelStatus = "starting" | "connected" | "disconnected" | "error";

export interface ServerTunnel {
  id: string;
  server_id: string;
  playit_account_id: string | null;
  playit_tunnel_id: string | null;
  sidecar_container_id: string | null;
  public_address: string | null;
  proto: string;
  status: TunnelStatus;
  active: boolean;
  created_at: string;
}

export interface PlayitAccount {
  id: string;
  name: string;
  status: string;
  claim_url?: string;
  created_at: string;
  updated_at: string;
}

export interface ModInfo {
  name: string;
  size: number;
}

export interface ModsResponse {
  folder: string;
  mods: ModInfo[];
}

export interface ServerMetric {
  id: string;
  server_id: string;
  cpu_pct: number;
  mem_bytes: number;
  player_count: number;
  max_players: number;
  latency_ms: number | null;
  storage_bytes: number;
  recorded_at: string;
}

export interface CreateServerInput {
  name: string;
  type: ServerType;
  version: string;
  loader_version?: string;
  memory_mb?: number;
  cpu_cores?: number;
}

// ----- fetch layer ---------------------------------------------------------

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "ApiError";
  }
}

async function call<T>(fn: (...args: any[]) => Promise<any>, ...args: any[]): Promise<T> {
  try {
    const res = await fn(...args);
    return res as T;
  } catch (err: any) {
    throw new ApiError(500, err?.message || err || "Request failed");
  }
}

export const api = {
  login: async (_username: string, _password: string) => {
    // Local app has no login requirements; return mock token
    return { token: "mock-desktop-token" };
  },

  listServers: () => call<Server[]>(App.ListServers).then((s) => s ?? []),
  getServer: (id: string) => call<Server>(App.GetServer, id),
  createServer: (body: CreateServerInput) =>
    call<Server>(
      App.CreateServer,
      body.name,
      body.type,
      body.version,
      body.loader_version || "",
      body.memory_mb || 2048,
    ),
  startServer: (id: string) => call<Server>(App.StartServer, id),
  stopServer: (id: string) => call<void>(App.StopServer, id),
  restartServer: (id: string) => call<void>(App.RestartServer, id),
  deleteServer: (id: string) => call<void>(App.DeleteServer, id),

  getMetrics: (id: string, limit = 120) =>
    call<ServerMetric[]>(App.GetServerMetrics, id, limit).then((m) => m ?? []),
  getStorage: (id: string) =>
    call<{ bytes: number; available: boolean }>(App.GetServerStorage, id),

  listMods: (id: string) => call<ModsResponse>(App.ListMods, id),
  deleteMod: (id: string, name: string) => call<void>(App.DeleteMod, id, name),
  uploadMods: async (id: string, files: File[]): Promise<ModsResponse> => {
    // In Wails, get the absolute host path of each file and install it directly
    const paths = files.map((f: any) => f.path).filter((p) => p !== undefined && p !== "");
    if (paths.length > 0) {
      await call(App.InstallMods, id, paths);
    }
    return call<ModsResponse>(App.ListMods, id);
  },
  updateServerSettings: (
    id: string,
    body: { difficulty: string; online_mode: boolean; motd: string; memory_mb: number },
  ) =>
    call<Server>(
      App.UpdateServerSettings,
      id,
      body.difficulty,
      body.online_mode,
      body.motd,
      body.memory_mb,
    ),
  getSystem: () => call<{ ram_total_bytes: number; cpu_cores: number }>(App.GetSystemInfo),

  updateBackupSchedule: (
    id: string,
    body: { enabled: boolean; interval_minutes: number; keep: number },
  ) => call<Server>(App.UpdateBackupSchedule, id, body.enabled, body.interval_minutes, body.keep),

  iconUrl: (id: string) => `/api/servers/${id}/icon`,
  uploadIcon: async (id: string, file: File): Promise<void> => {
    const path = (file as any).path;
    if (path) {
      await call(App.UploadServerIcon, id, path);
    }
  },

  listBackups: (id: string) => call<Backup[]>(App.ListBackups, id).then((b) => b ?? []),
  createBackup: (id: string) => call<Backup>(App.CreateBackup, id),
  restoreBackup: (backupId: string) => call<unknown>(App.RestoreBackup, backupId),

  getTunnels: (id: string) => call<ServerTunnel[]>(App.GetTunnelStatus, id).then((t) => t ?? []),
  detachTunnel: (id: string, proto?: "tcp" | "udp") =>
    call<void>(App.DetachTunnel, id, proto || ""),
  createTunnel: (id: string, kind: "java" | "bedrock") =>
    call<ServerTunnel>(App.CreateTunnel, id, kind),
  rescanTunnel: (id: string) => call<ServerTunnel[]>(App.RescanTunnel, id).then((t) => t ?? []),
  listTunnels: () => call<ServerTunnel[]>(App.ListTunnels).then((t) => t ?? []),

  listAccounts: () => call<PlayitAccount[]>(App.ListPlayitAccounts).then((a) => a ?? []),
  createAccount: (name: string, secret_key: string) =>
    call<PlayitAccount>(App.CreatePlayitAccount, name, secret_key),
  deleteAccount: (id: string) => call<void>(App.DeletePlayitAccount, id),
  startPlayitClaim: () => call<{ claim_url: string }>(App.StartPlayitClaim),

  getSettings: () =>
    call<{ servers_path: string; backups_within_server: string }>(App.GetSettings),
  updateSettings: (body: { servers_path?: string; backups_within_server?: string }) =>
    call<void>(App.UpdateSettings, body.servers_path || "", body.backups_within_server || ""),
  importServer: (body: {
    name: string;
    type: ServerType;
    version: string;
    loader_version?: string;
    memory_mb?: number;
    cpu_cores?: number;
    host_path: string;
  }) =>
    call<Server>(
      App.ImportServerFromPath,
      body.name,
      body.type,
      body.version,
      body.loader_version || "",
      body.memory_mb || 2048,
      body.host_path,
    ),
  selectFolder: () => call<string>(App.SelectFolder),
};
