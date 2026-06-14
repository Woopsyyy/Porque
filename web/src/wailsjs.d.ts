declare module "*/wailsjs/go/main/App" {
  export function ListServers(): Promise<any>;
  export function GetServer(id: string): Promise<any>;
  export function CreateServer(name: string, type: string, version: string, loaderVersion: string, memoryMB: number): Promise<any>;
  export function StartServer(id: string): Promise<any>;
  export function StopServer(id: string): Promise<any>;
  export function RestartServer(id: string): Promise<any>;
  export function DeleteServer(id: string): Promise<any>;
  export function GetServerMetrics(id: string, limit: number): Promise<any>;
  export function GetServerStorage(id: string): Promise<any>;
  export function ListMods(id: string): Promise<any>;
  export function DeleteMod(id: string, name: string): Promise<any>;
  export function InstallMods(id: string, paths: string[]): Promise<any>;
  export function UpdateServerSettings(id: string, difficulty: string, onlineMode: boolean, motd: string, memoryMB: number): Promise<any>;
  export function GetSystemInfo(): Promise<any>;
  export function UpdateBackupSchedule(id: string, enabled: boolean, intervalMinutes: number, keep: number): Promise<any>;
  export function UploadServerIcon(id: string, path: string): Promise<any>;
  export function ListBackups(id: string): Promise<any>;
  export function CreateBackup(id: string): Promise<any>;
  export function RestoreBackup(id: string): Promise<any>;
  export function GetTunnelStatus(id: string): Promise<any>;
  export function DetachTunnel(id: string, proto: string): Promise<any>;
  export function CreateTunnel(id: string, kind: string): Promise<any>;
  export function RescanTunnel(id: string): Promise<any>;
  export function ListTunnels(): Promise<any>;
  export function ListPlayitAccounts(): Promise<any>;
  export function CreatePlayitAccount(name: string, secretKey: string): Promise<any>;
  export function DeletePlayitAccount(id: string): Promise<any>;
  export function StartPlayitClaim(): Promise<any>;
  export function GetSettings(): Promise<any>;
  export function UpdateSettings(serversPath: string, backupsWithinServer: string): Promise<any>;
  export function ImportServerFromPath(name: string, type: string, version: string, loaderVersion: string, memoryMB: number, hostPath: string): Promise<any>;
  export function SelectFolder(): Promise<any>;
  export function StartStreamingLogs(serverIDStr: string): Promise<void>;
  export function StopStreamingLogs(serverIDStr: string): Promise<void>;
  export function StartStreamingPlayitLogs(serverIDStr: string): Promise<void>;
  export function StopStreamingPlayitLogs(serverIDStr: string): Promise<void>;
}

declare module "*/wailsjs/runtime" {
  export function EventsOn(eventName: string, callback: (data: any) => void): void;
  export function EventsOff(eventName: string): void;
}
