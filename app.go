package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
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
	"github.com/woopsy/porque/internal/worker"
)

type App struct {
	ctx     context.Context
	store   *db.Store
	life    *mcserver.Controller
	tunnels *playit.Manager
	worker  *worker.Worker
	backups *backup.Service

	activeStreams   map[string]context.CancelFunc
	activeStreamsMu sync.Mutex

	allowClose bool
}

func NewApp() *App {
	return &App{
		activeStreams: make(map[string]context.CancelFunc),
	}
}

type WailsPublisher struct {
	ctx context.Context
}

func (wp *WailsPublisher) PublishStatus(topic string, payload any) {
	if wp.ctx != nil {
		wailsRuntime.EventsEmit(wp.ctx, "topic:"+topic, payload)
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

func (a *App) SelectFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Minecraft Server Directory",
	})
}

func (a *App) SelectFile() (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select File",
	})
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
	return a.backups.Create(a.ctx, id)
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

func (a *App) UpdateBackupSchedule(serverID string, enabled bool, intervalMinutes int, keep int) (*db.Server, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	if intervalMinutes < 5 {
		intervalMinutes = 360
	}
	if keep < 1 {
		keep = 5
	}
	if err := a.store.UpdateBackupSchedule(a.ctx, id, enabled, intervalMinutes, keep); err != nil {
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
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		files = append(files, mcserver.ModFile{
			Name: filepath.Base(fp),
			Data: data,
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
				"name":       "Bundled Playit Account (linking...)",
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
			activeTs, err := a.store.ListActiveTunnels(a.ctx)
			if err == nil {
				for _, act := range activeTs {
					srv, err := a.store.GetServer(a.ctx, act.ServerID)
					if err == nil {
						var matched *playit.Tunnel
						for _, pt := range playitTunnels {
							if strings.Contains(strings.ToLower(pt.Name), strings.ToLower(srv.Name)) ||
								strings.Contains(strings.ToLower(srv.Name), strings.ToLower(pt.Name)) {
								matched = &pt
								break
							}
						}
						if matched == nil && len(playitTunnels) > 0 {
							matched = &playitTunnels[0]
						}
						if matched != nil {
							status := db.TunnelConnected
							_ = a.store.UpdateServerTunnel(a.ctx, act.ID, status, &matched.PublicAddress)
						}
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

func (a *App) StartStreamingPlayitLogs(serverIDStr string) {
	a.activeStreamsMu.Lock()
	if cancel, exists := a.activeStreams["playit:"+serverIDStr]; exists {
		cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.activeStreams["playit:"+serverIDStr] = cancel
	a.activeStreamsMu.Unlock()

	go func() {
		id, err := uuid.Parse(serverIDStr)
		if err != nil {
			return
		}
		rc, err := a.life.FollowSidecarLogs(ctx, id)
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
					wailsRuntime.EventsEmit(a.ctx, "logs:playit:"+serverIDStr, string(buf[:n]))
				}
				if err != nil {
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
