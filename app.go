package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/woopsy/porque/internal/autostart"
	"github.com/woopsy/porque/internal/backup"
	"github.com/woopsy/porque/internal/db"
	"github.com/woopsy/porque/internal/mcserver"
	"github.com/woopsy/porque/internal/playit"
	"github.com/woopsy/porque/internal/rcon"
	"github.com/woopsy/porque/internal/systray"
	"github.com/woopsy/porque/internal/updater"
	"github.com/woopsy/porque/internal/worker"
)

//go:embed assets/mascot_256.png
var mascotBytes []byte

type App struct {
	ctx     context.Context
	store   *db.Store
	life    *mcserver.Controller
	tunnels *playit.Manager
	worker  *worker.Worker
	backups *backup.Service

	version string

	activeStreams   map[string]context.CancelFunc
	activeStreamsMu sync.Mutex

	allowClose bool
}

func NewApp(version string) *App {
	if version == "" {
		version = "dev"
	}
	return &App{
		version:       version,
		activeStreams: make(map[string]context.CancelFunc),
	}
}

type WailsPublisher struct {
	ctx context.Context
}

func (wp *WailsPublisher) PublishStatus(topic string, payload any) {
	if wp.ctx != nil {
		wailsRuntime.EventsEmit(wp.ctx, "topic:"+topic, payload)
		log.Printf("[WailsPublisher] Emitted event topic:%s\n", topic)
	} else {
		log.Printf("[WailsPublisher] Context is NIL, cannot emit event topic:%s\n", topic)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		userConfigDir = "."
	}
	appDir := filepath.Join(userConfigDir, "porque")
	_ = os.MkdirAll(appDir, 0755)

	dbPath := filepath.Join(appDir, "porque.db")

	// Write dbPath to a local debug log file to verify what database is used in dev
	_ = os.WriteFile("porque_db_path.log", []byte(dbPath), 0644)

	conn, err := db.Connect(dbPath)
	if err != nil {
		wailsRuntime.LogErrorf(ctx, "failed to connect to db: %v", err)
		return
	}

	if err := db.Migrate(conn); err != nil {
		wailsRuntime.LogErrorf(ctx, "failed to migrate db: %v", err)
		return
	}

	a.store = db.NewStore(conn)

	serversDir := filepath.Join(appDir, "servers")
	_ = os.MkdirAll(serversDir, 0755)
	if _, err := a.store.GetSetting(context.Background(), "servers_path"); err != nil {
		_ = a.store.SetSetting(context.Background(), "servers_path", serversDir)
	}
	if _, err := a.store.GetSetting(context.Background(), "backups_within_server"); err != nil {
		_ = a.store.SetSetting(context.Background(), "backups_within_server", "true")
	}

	pub := &WailsPublisher{ctx: ctx}

	a.life = mcserver.NewController(a.store, pub)
	a.tunnels = playit.NewManager(a.store, playit.NewHTTPClient(), pub, appDir)
	a.backups = backup.NewService(a.store, a.life, appDir, 5)
	a.life.SetBackupService(a.backups)

	wConfig := worker.Config{
		MetricsInterval: 10 * time.Second,
		StartupGrace:    60 * time.Second,
		HeartbeatMisses: 3,
		MaxRestarts:     5,
		RestartWindow:   10 * time.Minute,
		RestartBackoff:  2 * time.Second,
	}
	a.worker = worker.New(a.store, a.life, a.backups, pub, wConfig)
	go a.worker.Run(context.Background())

	// Start Windows system tray icon
	systray.Start(mascotBytes, a.Show, a.Quit, a)

	// Check GitHub for a newer release and self-update in the background.
	go a.autoUpdate()
}

// autoUpdate checks GitHub Releases on startup and, if a newer version exists,
// downloads it and swaps the running binary in place. It never blocks startup
// and is skipped for dev builds.
func (a *App) autoUpdate() {
	if a.version == "dev" {
		return
	}
	ctx := context.Background()
	rel, err := updater.Latest(ctx)
	if err != nil {
		wailsRuntime.LogInfof(a.ctx, "update check failed: %v", err)
		return
	}
	if !updater.IsNewer(rel.TagName, a.version) {
		return
	}

	wailsRuntime.EventsEmit(a.ctx, "update:available", map[string]string{"version": rel.TagName})
	if err := updater.DownloadAndApply(ctx, rel); err != nil {
		wailsRuntime.LogErrorf(a.ctx, "auto-update failed: %v", err)
		wailsRuntime.EventsEmit(a.ctx, "update:failed", map[string]string{
			"version": rel.TagName,
			"url":     rel.HTMLURL,
		})
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "update:ready", map[string]string{"version": rel.TagName})
}

// GetAppVersion returns the running application version.
func (a *App) GetAppVersion() string {
	return a.version
}

// CheckForUpdates manually checks for a newer release (for a Settings button).
func (a *App) CheckForUpdates() (map[string]any, error) {
	rel, err := updater.Latest(a.ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"current":   a.version,
		"latest":    rel.TagName,
		"available": a.version != "dev" && updater.IsNewer(rel.TagName, a.version),
		"url":       rel.HTMLURL,
	}, nil
}

// ApplyUpdate downloads and applies the latest release on demand.
func (a *App) ApplyUpdate() error {
	rel, err := updater.Latest(a.ctx)
	if err != nil {
		return err
	}
	if !updater.IsNewer(rel.TagName, a.version) {
		return fmt.Errorf("already up to date")
	}
	if err := updater.DownloadAndApply(a.ctx, rel); err != nil {
		return err
	}
	wailsRuntime.EventsEmit(a.ctx, "update:ready", map[string]string{"version": rel.TagName})
	return nil
}

// RestartApp relaunches the (now-updated) executable and exits the current one.
func (a *App) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Hard-exit so the close-to-tray handler doesn't keep us alive.
	os.Exit(0)
	return nil
}

func (a *App) GetSystemInfo() map[string]any {
	return map[string]any{
		"ram_total_bytes": hostRAMBytes(),
		"cpu_cores":       runtime.NumCPU(),
	}
}

func (a *App) ListServers() ([]db.Server, error) {
	return a.store.ListServers(a.ctx)
}

func (a *App) ListAppLogs() ([]db.AppLog, error) {
	return a.store.ListAppLogs(a.ctx)
}

func (a *App) CreateServer(name string, serverType string, version string, loaderVersion string, memoryMB int) (*db.Server, error) {
	return a.life.Create(a.ctx, mcserver.CreateParams{
		Name:          name,
		Type:          db.ServerType(serverType),
		Version:       version,
		LoaderVersion: loaderVersion,
		MemoryMB:      memoryMB,
	})
}

func (a *App) GetServer(idStr string) (*db.Server, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return a.store.GetServer(a.ctx, id)
}

func (a *App) StartServer(idStr string) (*db.Server, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return a.life.Start(a.ctx, id)
}

func (a *App) StopServer(idStr string) (map[string]string, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	if err := a.life.Stop(a.ctx, id); err != nil {
		return nil, err
	}
	_ = a.tunnels.Detach(a.ctx, id)
	return map[string]string{"status": "stopped"}, nil
}

func (a *App) RestartServer(idStr string) (map[string]string, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	if err := a.life.Restart(a.ctx, id); err != nil {
		return nil, err
	}
	return map[string]string{"status": "restarted"}, nil
}

func (a *App) DeleteServer(idStr string) error {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return err
	}
	srv, err := a.store.GetServer(a.ctx, id)
	if err != nil {
		return err
	}
	_ = a.tunnels.Detach(a.ctx, id)
	if err := a.life.Delete(a.ctx, id); err != nil {
		return err
	}
	a.backups.PurgeServer(srv.Name)
	return nil
}

// DeleteServerRecord removes the server from Porque (DB + backups) WITHOUT deleting
// the server directory from disk. The files stay where they are.
func (a *App) DeleteServerRecord(idStr string) error {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return err
	}
	srv, err := a.store.GetServer(a.ctx, id)
	if err != nil {
		return err
	}
	_ = a.tunnels.Detach(a.ctx, id)
	if err := a.life.DeleteRecord(a.ctx, id); err != nil {
		return err
	}
	a.backups.PurgeServer(srv.Name)
	return nil
}


func (a *App) GetServerMetrics(idStr string, limit int) ([]db.ServerMetric, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return a.store.ListMetrics(a.ctx, id, limit)
}

func (a *App) GetServerStorage(idStr string) (map[string]any, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	bytes, available, err := a.life.Storage(a.ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"bytes": bytes, "available": available}, nil
}

func (a *App) UpdateServerSettings(idStr string, difficulty string, onlineMode bool, motd string, memoryMB int) (*db.Server, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	switch difficulty {
	case "peaceful", "easy", "normal", "hard":
	default:
		difficulty = "normal"
	}
	if memoryMB < 512 {
		return nil, fmt.Errorf("memory_mb must be at least 512")
	}
	motd = strings.TrimSpace(motd)
	if motd == "" {
		motd = "A Minecraft Server"
	}
	cpu := mcserver.AutoCPUCores(memoryMB)
	if err := a.store.UpdateServerConfig(a.ctx, id, difficulty, onlineMode, motd, memoryMB, cpu); err != nil {
		return nil, err
	}
	return a.store.GetServer(a.ctx, id)
}

// SelectFolder opens the OS-native directory picker via the Wails runtime. An
// empty result with no error means the user cancelled. We intentionally do NOT
// shell out to PowerShell here — the old COM/WinForms fallback tripped Windows
// Defender and execution-policy warnings.
func (a *App) SelectFolder() (string, error) {
	res, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Minecraft Server Directory",
	})
	if err != nil {
		wailsRuntime.LogErrorf(a.ctx, "OpenDirectoryDialog failed: %v", err)
		return "", err
	}
	return res, nil
}

// SelectFile opens the OS-native file picker via the Wails runtime. An empty
// result with no error means the user cancelled.
func (a *App) SelectFile() (string, error) {
	res, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select File",
	})
	if err != nil {
		wailsRuntime.LogErrorf(a.ctx, "OpenFileDialog failed: %v", err)
		return "", err
	}
	return res, nil
}

func (a *App) DetectServerDirectory(hostPath string) (map[string]string, error) {
	t, ver := a.life.DetectServerType(a.ctx, hostPath)
	return map[string]string{
		"type":    string(t),
		"version": ver,
	}, nil
}

func (a *App) ImportServerFromPath(name string, serverType string, version string, loaderVersion string, memoryMB int, hostPath string) (*db.Server, error) {
	st := db.ServerType(serverType)
	if hostPath != "" {
		if t, ver := a.life.DetectServerType(a.ctx, hostPath); t != "" {
			st = t
			if ver != "" && (version == "" || version == "latest") {
				version = ver
			}
		}
	}
	if st == "" {
		st = db.TypeVanilla
	}
	if version == "" {
		version = "latest"
	}
	if memoryMB <= 0 {
		memoryMB = 2048
	}
	return a.life.ImportFromHostPath(a.ctx, mcserver.CreateParams{
		Name:          name,
		Type:          st,
		Version:       version,
		LoaderVersion: loaderVersion,
		MemoryMB:      memoryMB,
	}, hostPath)
}

func (a *App) ImportServerFromZip(name string, serverType string, version string, loaderVersion string, memoryMB int, zipPath string) (*db.Server, error) {
	st := db.ServerType(serverType)
	if st == "" {
		st = db.TypeVanilla
	}
	if version == "" {
		version = "latest"
	}
	if memoryMB <= 0 {
		memoryMB = 2048
	}

	f, err := os.Open(zipPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return a.life.ImportFromZip(a.ctx, mcserver.ImportParams{
		Name:          name,
		Type:          st,
		Version:       version,
		LoaderVersion: loaderVersion,
		MemoryMB:      memoryMB,
	}, f, fi.Size())
}

func (a *App) ListBackups(serverID string) ([]db.Backup, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	return a.backups.List(a.ctx, id)
}

func (a *App) CreateBackup(serverID string) (*db.Backup, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := a.store.GetServer(a.ctx, id)
	if err != nil {
		return nil, err
	}

	wasRunning := srv.State == db.StateRunning || srv.State == db.StateStarting

	if wasRunning {
		// Stop the server
		if err := a.life.Stop(a.ctx, id); err != nil {
			return nil, fmt.Errorf("failed to stop server before backup: %w", err)
		}
	}

	b, err := a.backups.Create(a.ctx, id)
	if err != nil {
		if wasRunning {
			_, _ = a.life.Start(a.ctx, id)
		}
		return nil, err
	}

	if wasRunning {
		// Restart the server
		if _, err := a.life.Start(a.ctx, id); err != nil {
			return b, fmt.Errorf("backup completed but failed to restart server: %w", err)
		}
	}

	return b, nil
}

func (a *App) RestoreBackup(backupIDStr string) (map[string]string, error) {
	backupID, err := uuid.Parse(backupIDStr)
	if err != nil {
		return nil, err
	}
	b, err := a.store.GetBackup(a.ctx, backupID)
	if err != nil {
		return nil, err
	}
	if err := a.backups.Restore(a.ctx, b.ServerID, backupID); err != nil {
		return nil, err
	}
	return map[string]string{"status": "restored", "state": "stopped"}, nil
}

// DeleteBackup removes a backup's archive file and record.
func (a *App) DeleteBackup(backupIDStr string) error {
	backupID, err := uuid.Parse(backupIDStr)
	if err != nil {
		return err
	}
	return a.backups.Delete(a.ctx, backupID)
}

func (a *App) UpdateBackupSchedule(serverID string, enabled bool, intervalValue int, intervalUnit string, keep int) (*db.Server, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	if intervalValue < 1 {
		intervalValue = 1
	}
	switch intervalUnit {
	case "hour", "week", "month":
	default:
		intervalUnit = "hour"
	}
	if keep < 1 {
		keep = 5
	}
	if err := a.store.UpdateBackupSchedule(a.ctx, id, enabled, intervalValue, intervalUnit, keep); err != nil {
		return nil, err
	}
	return a.store.GetServer(a.ctx, id)
}

func (a *App) GetSettings() (map[string]string, error) {
	serversPath, _ := a.store.GetSetting(a.ctx, "servers_path")
	backupsWithin, _ := a.store.GetSetting(a.ctx, "backups_within_server")
	runOnBoot, _ := a.store.GetSetting(a.ctx, "run_on_boot")
	closeToMin, _ := a.store.GetSetting(a.ctx, "close_to_minimize")

	if runOnBoot == "" {
		runOnBoot = "false"
	}
	if closeToMin == "" {
		closeToMin = "false"
	}

	return map[string]string{
		"servers_path":          serversPath,
		"backups_within_server": backupsWithin,
		"run_on_boot":           runOnBoot,
		"close_to_minimize":     closeToMin,
	}, nil
}

func (a *App) UpdateSettings(serversPath string, backupsWithinServer string, runOnBoot string, closeToMinimize string) error {
	if err := a.store.SetSetting(a.ctx, "servers_path", serversPath); err != nil {
		return err
	}
	if err := a.store.SetSetting(a.ctx, "backups_within_server", backupsWithinServer); err != nil {
		return err
	}
	if err := a.store.SetSetting(a.ctx, "run_on_boot", runOnBoot); err != nil {
		return err
	}
	if err := a.store.SetSetting(a.ctx, "close_to_minimize", closeToMinimize); err != nil {
		return err
	}

	// Update Windows autostart configuration
	bootEnabled := runOnBoot == "true"
	if err := autostart.Set(bootEnabled); err != nil {
		wailsRuntime.LogErrorf(a.ctx, "failed to configure autostart: %v", err)
	}

	return nil
}

// OnBeforeClose intercepts the window close event. If close_to_minimize is enabled, it hides the window instead.
func (a *App) OnBeforeClose(ctx context.Context) bool {
	if a.allowClose {
		return false // Allow the app to quit
	}
	closeToMin, _ := a.store.GetSetting(a.ctx, "close_to_minimize")
	if closeToMin == "true" {
		wailsRuntime.WindowHide(ctx)
		return true // Prevent closing, hide window instead
	}
	return false // Allow standard exit
}

// Show makes the Wails window visible. Called from the system tray.
func (a *App) Show() {
	if a.ctx != nil {
		wailsRuntime.WindowShow(a.ctx)
	}
}

// Quit initiates a full application shutdown.
func (a *App) Quit() {
	a.allowClose = true
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
}

func (a *App) ListMods(serverID string) (map[string]any, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	mods, folder, err := a.life.ListMods(a.ctx, id)
	if err != nil {
		return nil, err
	}
	if mods == nil {
		mods = []mcserver.ModInfo{}
	}
	return map[string]any{"folder": folder, "mods": mods}, nil
}

func (a *App) InstallMods(serverID string, filePaths []string) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	var files []mcserver.ModFile
	for _, fp := range filePaths {
		files = append(files, mcserver.ModFile{
			Name: filepath.Base(fp),
			Path: fp,
		})
	}
	return a.life.UploadMods(a.ctx, id, files)
}

func (a *App) DeleteMod(serverID string, name string) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	return a.life.DeleteMod(a.ctx, id, name)
}

// ListOnlinePlayers returns the players currently connected to a running server.
func (a *App) ListOnlinePlayers(serverID string) ([]mcserver.Player, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	return a.life.ListOnlinePlayers(a.ctx, id)
}

// GetWhitelist returns the server's whitelisted players.
func (a *App) GetWhitelist(serverID string) ([]mcserver.Player, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	return a.life.GetWhitelist(a.ctx, id)
}

// AddToWhitelist whitelists a player (Java or Bedrock auto-detected).
func (a *App) AddToWhitelist(serverID, name string) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	return a.life.AddToWhitelist(a.ctx, id, name)
}

// RemoveFromWhitelist removes a player from the whitelist.
func (a *App) RemoveFromWhitelist(serverID, name string) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	return a.life.RemoveFromWhitelist(a.ctx, id, name)
}

// GetServerRuntime reports how long the current server process has been running
// (distinct from the application's own uptime). Returns running=false and no
// timestamps when the server is not currently running.
func (a *App) GetServerRuntime(serverID string) (map[string]any, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := a.store.GetServer(a.ctx, id)
	if err != nil {
		return nil, err
	}
	res := map[string]any{"running": srv.State == db.StateRunning}
	if srv.State != db.StateRunning {
		return res, nil
	}
	inst, err := a.store.LatestInstance(a.ctx, id)
	if err == nil && inst != nil && inst.StartedAt != nil && inst.StoppedAt == nil {
		res["started_at"] = inst.StartedAt.UTC().Format(time.RFC3339)
		res["uptime_seconds"] = int(time.Since(*inst.StartedAt).Seconds())
	}
	return res, nil
}



func (a *App) CreatePlayitAccount(name, secretKey string) (*db.PlayitAccount, error) {
	return a.store.CreatePlayitAccount(a.ctx, name, secretKey)
}

func (a *App) ListPlayitAccounts() ([]any, error) {
	accts, err := a.store.ListPlayitAccounts(a.ctx)
	if err != nil {
		return nil, err
	}
	if len(accts) == 0 {
		a.tunnels.EnsureBundledAccount(a.ctx)
		claiming, _, claimURL := a.tunnels.GetClaimStatus()
		if claiming || claimURL != "" {
			mockAcct := map[string]any{
				"id":         "00000000-0000-0000-0000-000000000000",
				"name":       "Minecraft (linking...)",
				"status":     "claiming",
				"claim_url":  claimURL,
				"created_at": time.Now().UTC().Format(time.RFC3339),
				"updated_at": time.Now().UTC().Format(time.RFC3339),
			}
			return []any{mockAcct}, nil
		}
	}
	var res []any
	for _, act := range accts {
		res = append(res, act)
	}
	return res, nil
}

func (a *App) DeletePlayitAccount(idStr string) error {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return err
	}
	tunnels, err := a.tunnels.ListActive(a.ctx)
	if err == nil {
		for _, t := range tunnels {
			if t.PlayitAccountID != nil && *t.PlayitAccountID == id {
				_ = a.tunnels.Detach(a.ctx, t.ServerID)
			}
		}
	}
	return a.store.DeletePlayitAccount(a.ctx, id)
}

func (a *App) ListTunnels() ([]db.ServerTunnel, error) {
	accts, err := a.store.ListPlayitAccounts(a.ctx)
	if err == nil && len(accts) > 0 {
		playitTunnels, err := a.tunnels.ListTunnelsFromAPI(a.ctx, accts[0].SecretKey)
		if err == nil {
			// Build proto→address map and a flat address list for legacy rows.
			byProto := map[string]string{}
			var anyAddresses []string
			for _, pt := range playitTunnels {
				if pt.PublicAddress == "" {
					continue
				}
				if _, ok := byProto[pt.Proto]; !ok {
					byProto[pt.Proto] = pt.PublicAddress
				}
				anyAddresses = append(anyAddresses, pt.PublicAddress)
			}

			activeTs, err := a.store.ListActiveTunnels(a.ctx)
			if err == nil {
				for _, act := range activeTs {
					if act.PublicAddress != nil && *act.PublicAddress != "" {
						continue // already has address, skip
					}
					var addr string
					if act.Proto == "" {
						// Legacy row (AttachTunnel path) — use first available address.
						if len(anyAddresses) > 0 {
							addr = anyAddresses[0]
						}
					} else if a, ok := byProto[act.Proto]; ok {
						addr = a
					}
					if addr != "" {
						status := db.TunnelConnected
						_ = a.store.UpdateServerTunnel(a.ctx, act.ID, status, &addr)
					}
				}
			}
		}
	}

	return a.tunnels.ListActive(a.ctx)
}


func (a *App) AttachTunnel(serverIDStr string, accountIDStr string) (*db.ServerTunnel, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return nil, err
	}
	return a.tunnels.Attach(a.ctx, serverID, accountID)
}

func (a *App) GetTunnelStatus(serverIDStr string) ([]db.ServerTunnel, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	tunnels, err := a.tunnels.Status(a.ctx, serverID)
	if err != nil {
		return nil, err
	}
	if tunnels == nil {
		return []db.ServerTunnel{}, nil
	}
	return tunnels, nil
}

func (a *App) SendServerCommand(serverIDStr string, command string) (string, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return "", err
	}
	srv, err := a.store.GetServer(a.ctx, serverID)
	if err != nil {
		return "", err
	}
	if srv.State != db.StateRunning {
		return "", fmt.Errorf("server is not running")
	}
	port := srv.RconPort
	if port == 0 {
		port = 25575
	}
	inst, err := a.store.LatestInstance(a.ctx, serverID)
	if err == nil && inst != nil && inst.RconHostPort != nil {
		port = *inst.RconHostPort
	}
	rc := rcon.New(fmt.Sprintf("127.0.0.1:%d", port), srv.RconPassword)
	return rc.Run(a.ctx, command)
}

func (a *App) CreateTunnel(serverIDStr string, kind string) (*db.ServerTunnel, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "java" && kind != "bedrock" {
		kind = "java"
	}
	return a.tunnels.CreateTunnelForServer(a.ctx, serverID, kind)
}

func (a *App) RescanTunnel(serverIDStr string) ([]db.ServerTunnel, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	return a.tunnels.Rescan(a.ctx, serverID)
}

func (a *App) DetachTunnel(serverIDStr string, proto string) error {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return err
	}
	if proto == "tcp" || proto == "udp" {
		return a.tunnels.DetachProto(a.ctx, serverID, proto)
	}
	return a.tunnels.Detach(a.ctx, serverID)
}

func (a *App) StartPlayitClaim() (map[string]string, error) {
	a.tunnels.EnsureBundledAccount(a.ctx)
	var claimURL string
	for i := 0; i < 20; i++ {
		_, _, url := a.tunnels.GetClaimStatus()
		if url != "" {
			claimURL = url
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if claimURL == "" {
		return nil, fmt.Errorf("claim URL not generated in time")
	}
	return map[string]string{"claim_url": claimURL}, nil
}

func (a *App) StartStreamingLogs(serverIDStr string) {
	a.activeStreamsMu.Lock()
	if cancel, exists := a.activeStreams[serverIDStr]; exists {
		cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.activeStreams[serverIDStr] = cancel
	a.activeStreamsMu.Unlock()

	go func() {
		id, err := uuid.Parse(serverIDStr)
		if err != nil {
			return
		}
		rc, err := a.life.FollowLogs(ctx, id)
		if err != nil {
			return
		}
		defer rc.Close()

		buf := make([]byte, 2048)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := rc.Read(buf)
				if n > 0 {
					wailsRuntime.EventsEmit(a.ctx, "logs:"+serverIDStr, string(buf[:n]))
				}
				if err != nil {
					return
				}
			}
		}
	}()
}

func (a *App) StopStreamingLogs(serverIDStr string) {
	a.activeStreamsMu.Lock()
	if cancel, exists := a.activeStreams[serverIDStr]; exists {
		cancel()
		delete(a.activeStreams, serverIDStr)
	}
	a.activeStreamsMu.Unlock()
}

func (a *App) GetServerLogs(serverIDStr string) (string, error) {
	id, err := uuid.Parse(serverIDStr)
	if err != nil {
		return "", err
	}
	srv, err := a.store.GetServer(a.ctx, id)
	if err != nil {
		return "", err
	}
	logPath := filepath.Join(srv.VolumeName, "server.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "No log file found.", nil
		}
		return "", err
	}
	const maxBytes = 500 * 1024
	if len(data) > maxBytes {
		return string(data[len(data)-maxBytes:]), nil
	}
	return string(data), nil
}

func (a *App) StartStreamingPlayitLogs(serverIDStr string) {
	log.Printf("[StartStreamingPlayitLogs] Requested for server ID: %s", serverIDStr)
	a.activeStreamsMu.Lock()
	if cancel, exists := a.activeStreams["playit:"+serverIDStr]; exists {
		log.Printf("[StartStreamingPlayitLogs] Cancelling existing stream for %s", serverIDStr)
		cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.activeStreams["playit:"+serverIDStr] = cancel
	a.activeStreamsMu.Unlock()

	go func() {
		id, err := uuid.Parse(serverIDStr)
		if err != nil {
			log.Printf("[StartStreamingPlayitLogs] Failed to parse UUID %s: %v", serverIDStr, err)
			return
		}
		rc, err := a.life.FollowSidecarLogs(ctx, id)
		if err != nil {
			log.Printf("[StartStreamingPlayitLogs] FollowSidecarLogs failed for %s: %v", serverIDStr, err)
			return
		}
		log.Printf("[StartStreamingPlayitLogs] FollowSidecarLogs started successfully for %s", serverIDStr)
		defer rc.Close()

		buf := make([]byte, 2048)
		for {
			select {
			case <-ctx.Done():
				log.Printf("[StartStreamingPlayitLogs] Context cancelled for %s", serverIDStr)
				return
			default:
				n, err := rc.Read(buf)
				if n > 0 {
					log.Printf("[StartStreamingPlayitLogs] Read %d bytes, emitting event logs:playit:%s", n, serverIDStr)
					wailsRuntime.EventsEmit(a.ctx, "logs:playit:"+serverIDStr, string(buf[:n]))
				}
				if err != nil {
					log.Printf("[StartStreamingPlayitLogs] Read error or EOF for %s: %v", serverIDStr, err)
					return
				}
			}
		}
	}()
}

func (a *App) StopStreamingPlayitLogs(serverIDStr string) {
	a.activeStreamsMu.Lock()
	if cancel, exists := a.activeStreams["playit:"+serverIDStr]; exists {
		cancel()
		delete(a.activeStreams, "playit:"+serverIDStr)
	}
	a.activeStreamsMu.Unlock()
}

func (a *App) GetServerIcon(idStr string) (string, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return "", err
	}
	data, err := a.life.GetIcon(a.ctx, id)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) UploadServerIcon(idStr string, filePath string) error {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return a.life.SetIcon(a.ctx, id, data)
}

func hostRAMBytes() int64 {
	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")
		var stat struct {
			Length            uint32
			MemoryLoad        uint32
			TotalPhys         uint64
			AvailPhys         uint64
			TotalPage         uint64
			AvailPage         uint64
			TotalVirtual      uint64
			AvailVirtual      uint64
			AvailExtendedPhys uint64
		}
		stat.Length = uint32(unsafe.Sizeof(stat))
		r, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&stat)))
		if r == 0 {
			return 0
		}
		return int64(stat.TotalPhys)
	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("sysctl", "-n", "hw.memsize")
		out, err := cmd.Output()
		if err == nil {
			bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			if err == nil {
				return bytes
			}
		}
	} else {
		f, err := os.Open("/proc/meminfo")
		if err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, _ := strconv.ParseInt(fields[1], 10, 64)
						return kb * 1024
					}
				}
			}
		}
	}
	return 0
}

type GeyserMetadata struct {
	GeyserBuild    int `json:"geyser_build"`
	FloodgateBuild int `json:"floodgate_build"`
}

type GeyserBuildResponse struct {
	Build int `json:"build"`
}

// CreateJavaAndBedrockTunnels creates both Java (TCP) and Bedrock (UDP) tunnels for the running server.
func (a *App) CreateJavaAndBedrockTunnels(serverIDStr string) ([]db.ServerTunnel, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	// Create Java tunnel
	_, err = a.tunnels.CreateTunnelForServer(a.ctx, serverID, "java")
	if err != nil {
		return nil, fmt.Errorf("failed to create Java tunnel: %w", err)
	}
	// Create Bedrock tunnel
	_, err = a.tunnels.CreateTunnelForServer(a.ctx, serverID, "bedrock")
	if err != nil {
		return nil, fmt.Errorf("failed to create Bedrock tunnel: %w", err)
	}
	return a.tunnels.Status(a.ctx, serverID)
}

// GetGeyserStatus gets the current status of Geyser and Floodgate on a server.
func (a *App) GetGeyserStatus(serverIDStr string) (map[string]any, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	srv, err := a.store.GetServer(a.ctx, serverID)
	if err != nil {
		return nil, err
	}

	folder, err := a.life.ModsFolder(a.ctx, serverID)
	if err != nil {
		return nil, err
	}

	targetDir := filepath.Join(srv.VolumeName, folder)
	geyserInstalled := false
	floodgateInstalled := false

	// Scan folder for jar files
	entries, _ := os.ReadDir(targetDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jar") {
			if strings.HasPrefix(name, "geyser-") {
				geyserInstalled = true
			}
			if strings.HasPrefix(name, "floodgate-") {
				floodgateInstalled = true
			}
		}
	}

	// Read metadata file if exists
	var meta GeyserMetadata
	metaPath := filepath.Join(srv.VolumeName, "geyser_metadata.json")
	if content, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(content, &meta)
	}

	// If the jars are not actually there, reset the metadata version
	if !geyserInstalled {
		meta.GeyserBuild = 0
	}
	if !floodgateInstalled {
		meta.FloodgateBuild = 0
	}

	// Query latest builds from GeyserMC API
	latestGeyserBuild := 0
	latestFloodgateBuild := 0

	client := &http.Client{Timeout: 2 * time.Second}
	if resp, err := client.Get("https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest"); err == nil {
		defer resp.Body.Close()
		var r GeyserBuildResponse
		if json.NewDecoder(resp.Body).Decode(&r) == nil {
			latestGeyserBuild = r.Build
		}
	}

	if resp, err := client.Get("https://download.geysermc.org/v2/projects/floodgate/versions/latest/builds/latest"); err == nil {
		defer resp.Body.Close()
		var r GeyserBuildResponse
		if json.NewDecoder(resp.Body).Decode(&r) == nil {
			latestFloodgateBuild = r.Build
		}
	}

	supportsGeyser := srv.ServerType == db.TypePaper || srv.ServerType == db.TypeFabric

	return map[string]any{
		"geyser_installed":       geyserInstalled,
		"geyser_build":           meta.GeyserBuild,
		"floodgate_installed":     floodgateInstalled,
		"floodgate_build":         meta.FloodgateBuild,
		"latest_geyser_build":    latestGeyserBuild,
		"latest_floodgate_build": latestFloodgateBuild,
		"server_type":            srv.ServerType,
		"supports_geyser":        supportsGeyser,
	}, nil
}

// InstallOrUpdateGeyser downloads the latest Geyser & Floodgate, and configures the port.
func (a *App) InstallOrUpdateGeyser(serverIDStr string) (map[string]any, error) {
	serverID, err := uuid.Parse(serverIDStr)
	if err != nil {
		return nil, err
	}
	srv, err := a.store.GetServer(a.ctx, serverID)
	if err != nil {
		return nil, err
	}

	if srv.ServerType != db.TypePaper && srv.ServerType != db.TypeFabric {
		return nil, fmt.Errorf("server type %s is not supported by Geyser", srv.ServerType)
	}

	// Stop server if running
	if srv.State == db.StateRunning || srv.State == db.StateStarting {
		_ = a.life.Stop(a.ctx, serverID)
		// Wait up to 30 seconds for it to be fully stopped
		for i := 0; i < 30; i++ {
			s, err := a.store.GetServer(a.ctx, serverID)
			if err == nil && s.State == db.StateStopped {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	folder, err := a.life.ModsFolder(a.ctx, serverID)
	if err != nil {
		return nil, err
	}

	targetDir := filepath.Join(srv.VolumeName, folder)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Clean out old geyser/floodgate jars
	entries, _ := os.ReadDir(targetDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jar") && (strings.HasPrefix(name, "geyser-") || strings.HasPrefix(name, "floodgate-")) {
			_ = os.Remove(filepath.Join(targetDir, entry.Name()))
		}
	}

	platform := "spigot"
	geyserJarName := "Geyser-Spigot.jar"
	floodgateJarName := "floodgate-spigot.jar"
	configPath := filepath.Join(srv.VolumeName, "plugins", "Geyser-Spigot", "config.yml")
	configDir := filepath.Join(srv.VolumeName, "plugins", "Geyser-Spigot")

	if srv.ServerType == db.TypeFabric {
		platform = "fabric"
		geyserJarName = "Geyser-Fabric.jar"
		floodgateJarName = "floodgate-fabric.jar"
		configPath = filepath.Join(srv.VolumeName, "config", "Geyser-Fabric", "config.yml")
		configDir = filepath.Join(srv.VolumeName, "config", "Geyser-Fabric")
	}

	// Fetch latest build numbers first
	client := &http.Client{Timeout: 15 * time.Second}
	var geyserBuild, floodgateBuild int

	if resp, err := client.Get("https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest"); err == nil {
		defer resp.Body.Close()
		var r GeyserBuildResponse
		_ = json.NewDecoder(resp.Body).Decode(&r)
		geyserBuild = r.Build
	}
	if resp, err := client.Get("https://download.geysermc.org/v2/projects/floodgate/versions/latest/builds/latest"); err == nil {
		defer resp.Body.Close()
		var r GeyserBuildResponse
		_ = json.NewDecoder(resp.Body).Decode(&r)
		floodgateBuild = r.Build
	}

	// Helper to download a URL to dest
	download := func(url, dest string) error {
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %s", resp.Status)
		}
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, resp.Body)
		return err
	}

	// Download Geyser
	geyserURL := fmt.Sprintf("https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/%s", platform)
	if err := download(geyserURL, filepath.Join(targetDir, geyserJarName)); err != nil {
		return nil, fmt.Errorf("failed to download Geyser: %w", err)
	}

	// Download Floodgate
	floodgatePlatform := "spigot"
	if srv.ServerType == db.TypeFabric {
		floodgatePlatform = "fabric"
	}
	floodgateURL := fmt.Sprintf("https://download.geysermc.org/v2/projects/floodgate/versions/latest/builds/latest/downloads/%s", floodgatePlatform)
	if err := download(floodgateURL, filepath.Join(targetDir, floodgateJarName)); err != nil {
		return nil, fmt.Errorf("failed to download Floodgate: %w", err)
	}

	// Create config dir
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}

	// Write default config.yml if not exists
	gamePort := srv.Port
	if gamePort == 0 {
		gamePort = 25565
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := fmt.Sprintf(`# Geyser Configuration
bedrock:
  address: 0.0.0.0
  port: 19132
remote:
  address: 127.0.0.1
  port: %d
  auth-type: floodgate
`, gamePort)
		_ = os.WriteFile(configPath, []byte(defaultConfig), 0o644)
	}

	// Update Geyser config so the bedrock listener and the remote (Java) port match this server.
	_ = updateGeyserConfig(configPath, 19132, gamePort)

	// Write metadata JSON
	meta := GeyserMetadata{
		GeyserBuild:    geyserBuild,
		FloodgateBuild: floodgateBuild,
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(srv.VolumeName, "geyser_metadata.json"), metaBytes, 0o644)

	return map[string]any{
		"success":         true,
		"geyser_build":    geyserBuild,
		"floodgate_build": floodgateBuild,
	}, nil
}

func updateGeyserConfig(path string, bedrockPort, remotePort int) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")

	inBedrockSection := false
	inRemoteSection := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section headers (without indentation)
		if strings.HasPrefix(line, "bedrock:") {
			inBedrockSection = true
			inRemoteSection = false
			continue
		}
		if strings.HasPrefix(line, "remote:") {
			inRemoteSection = true
			inBedrockSection = false
			continue
		}
		// If line starts with a non-space, we left the previous section
		if len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inBedrockSection = false
			inRemoteSection = false
		}

		if inBedrockSection {
			if strings.HasPrefix(trimmed, "port:") {
				idx := strings.Index(line, "port:")
				indent := line[:idx]
				lines[i] = fmt.Sprintf("%sport: %d", indent, bedrockPort)
			}
		}

		if inRemoteSection {
			if strings.HasPrefix(trimmed, "auth-type:") {
				idx := strings.Index(line, "auth-type:")
				indent := line[:idx]
				lines[i] = fmt.Sprintf("%sauth-type: floodgate", indent)
			}
			if strings.HasPrefix(trimmed, "port:") {
				idx := strings.Index(line, "port:")
				indent := line[:idx]
				lines[i] = fmt.Sprintf("%sport: %d", indent, remotePort)
			}
		}
	}

	newContent := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(newContent), 0o644)
}

