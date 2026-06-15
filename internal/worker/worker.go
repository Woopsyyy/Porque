package worker

import (
	"context"
	"fmt"
	"log"
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

	"github.com/woopsy/porque/internal/backup"
	"github.com/woopsy/porque/internal/db"
	"github.com/woopsy/porque/internal/mcserver"
	"github.com/woopsy/porque/internal/rcon"
)

// Publisher fans out status/metric events.
type Publisher interface {
	PublishStatus(topic string, payload any)
}

// Config tunes the worker loops.
type Config struct {
	MetricsInterval time.Duration
	StartupGrace    time.Duration
	HeartbeatMisses int
	MaxRestarts     int
	RestartWindow   time.Duration
	RestartBackoff  time.Duration
}

// Worker coordinates recovery, heartbeat, metrics, and scheduled backups.
type Worker struct {
	store   *db.Store
	life    *mcserver.Controller
	backups *backup.Service
	pub     Publisher
	cfg     Config

	mu                  sync.Mutex
	restarts            map[uuid.UUID][]time.Time
	misses              map[uuid.UUID]int
	healing             map[uuid.UUID]bool
	gamerulesConfigured map[uuid.UUID]bool
}

// New constructs a Worker. backups may be nil (scheduled backups disabled).
func New(store *db.Store, life *mcserver.Controller, backups *backup.Service, pub Publisher, cfg Config) *Worker {
	return &Worker{
		store: store, life: life, backups: backups, pub: pub, cfg: cfg,
		restarts:            map[uuid.UUID][]time.Time{},
		misses:              map[uuid.UUID]int{},
		healing:             map[uuid.UUID]bool{},
		gamerulesConfigured: map[uuid.UUID]bool{},
	}
}

// Run starts the heartbeat/metrics scheduler, blocking until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	go w.logPruner(ctx)
	go w.backupScheduler(ctx)

	t := time.NewTicker(w.cfg.MetricsInterval)
	defer t.Stop()
	log.Printf("worker loops running (interval=%s)", w.cfg.MetricsInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick(ctx)
		}
	}
}

// logPruner deletes logs older than 24 hours every 5 minutes.
func (w *Worker) logPruner(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	// Run once immediately on start
	_ = w.store.PruneAppLogs(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.store.PruneAppLogs(ctx); err != nil {
				log.Printf("worker: failed to prune expired app logs: %v", err)
			}
		}
	}
}

// backupScheduler runs due scheduled backups once a minute.
func (w *Worker) backupScheduler(ctx context.Context) {
	if w.backups == nil {
		return
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			due, err := w.store.DueBackups(ctx)
			if err != nil {
				continue
			}
			for i := range due {
				srv := due[i]
				if !isBackupDue(&srv) {
					continue
				}
				if _, err := w.backups.Create(ctx, srv.ID); err != nil {
					log.Printf("worker: scheduled backup for %s failed: %v", srv.Name, err)
					continue
				}
				_ = w.store.MarkBackupRun(ctx, srv.ID)
			}
		}
	}
}

func isBackupDue(srv *db.Server) bool {
	if srv.BackupLastRun == nil {
		return true
	}
	var nextRun time.Time
	val := srv.BackupIntervalValue
	if val <= 0 {
		val = 1
	}
	switch srv.BackupIntervalUnit {
	case "hour":
		nextRun = srv.BackupLastRun.Add(time.Duration(val) * time.Hour)
	case "week":
		nextRun = srv.BackupLastRun.Add(time.Duration(val) * 7 * 24 * time.Hour)
	case "month":
		nextRun = srv.BackupLastRun.AddDate(0, val, 0)
	default:
		nextRun = srv.BackupLastRun.Add(time.Duration(val) * time.Hour)
	}
	return time.Now().After(nextRun)
}

func (w *Worker) doRecover(ctx context.Context, id uuid.UUID, eventType, details string) {
	defer w.endHealing(id)

	if !w.allowRestart(id) {
		_ = w.store.InsertRecoveryEvent(ctx, id, eventType, "mark_corrupted", "restart limit exceeded")
		w.setState(ctx, id, db.StateCorrupted, "restart limit exceeded; manual intervention required")
		return
	}
	_ = w.store.InsertRecoveryEvent(ctx, id, eventType, "restart_process", details)

	// Exponential backoff based on recent restart count.
	w.mu.Lock()
	attempts := len(w.restarts[id])
	w.mu.Unlock()
	backoff := w.cfg.RestartBackoff * time.Duration(1<<min(attempts, 5))
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return
	}

	if _, err := w.life.Start(ctx, id); err != nil {
		log.Printf("worker: recovery start failed for %s: %v", id, err)
		_ = w.store.InsertRecoveryEvent(ctx, id, eventType, "restart_failed", err.Error())
	}
}

// allowRestart reports whether a restart attempt is within the rate limit.
func (w *Worker) allowRestart(id uuid.UUID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := time.Now().Add(-w.cfg.RestartWindow)
	kept := w.restarts[id][:0]
	for _, t := range w.restarts[id] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= w.cfg.MaxRestarts {
		w.restarts[id] = kept
		return false
	}
	w.restarts[id] = append(kept, time.Now())
	return true
}

func (w *Worker) beginHealing(id uuid.UUID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.healing[id] {
		return false
	}
	w.healing[id] = true
	return true
}

func (w *Worker) endHealing(id uuid.UUID) {
	w.mu.Lock()
	delete(w.healing, id)
	w.mu.Unlock()
}

// ----- heartbeat + metrics -------------------------------------------------

func (w *Worker) tick(ctx context.Context) {
	servers, err := w.store.RunningServers(ctx)
	if err != nil {
		log.Printf("worker: list running servers: %v", err)
		return
	}
	var wg sync.WaitGroup
	for i := range servers {
		srv := servers[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.sample(ctx, &srv)
		}()
	}
	wg.Wait()
}

func (w *Worker) sample(ctx context.Context, srv *db.Server) {
	inst, err := w.store.LatestInstance(ctx, srv.ID)
	if err != nil || inst == nil || inst.ContainerID == nil || *inst.ContainerID == "" {
		return
	}
	pidStr := *inst.ContainerID
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return
	}

	running := isProcessAlive(pid)
	inGrace := inst.StartedAt != nil && time.Since(*inst.StartedAt) < w.cfg.StartupGrace

	if !running && !inGrace {
		w.mu.Lock()
		delete(w.gamerulesConfigured, srv.ID)
		w.mu.Unlock()
		w.triggerHeartbeatRecovery(ctx, srv.ID, "process not running")
		return
	}

	// Health check via RCON list players
	rc := rcon.New(fmt.Sprintf("127.0.0.1:%d", srv.RconPort), srv.RconPassword)
	players, maxPlayers := 0, 0
	latencyVal := 0
	latencyPtr := &latencyVal
	*latencyPtr = -1

	out, rconErr := rc.ListPlayers(ctx)
	if rconErr == nil {
		players, maxPlayers, _ = rcon.ParsePlayerList(out)
		*latencyPtr = 10 // reasonable default latency

		// Run gamerules once to disable noisy command feedback in game chat and logs
		w.mu.Lock()
		configured := w.gamerulesConfigured[srv.ID]
		w.mu.Unlock()

		if !configured {
			log.Printf("worker: configuring gamerules for server %s (disabling command feedback spam)\n", srv.Name)
			_, _ = rc.Run(ctx, "gamerule sendCommandFeedback false")
			_, _ = rc.Run(ctx, "gamerule broadcastConsoleToOps false")
			_, _ = rc.Run(ctx, "gamerule logAdminCommands false")

			w.mu.Lock()
			w.gamerulesConfigured[srv.ID] = true
			w.mu.Unlock()
		}
	} else if !inGrace {
		w.mu.Lock()
		w.misses[srv.ID]++
		miss := w.misses[srv.ID]
		w.mu.Unlock()
		if miss >= w.cfg.HeartbeatMisses {
			w.triggerHeartbeatRecovery(ctx, srv.ID, "unhealthy heartbeat")
		}
		return
	}

	w.mu.Lock()
	w.misses[srv.ID] = 0
	w.mu.Unlock()

	// CPU and Memory stats
	cpu, _ := getCPUUsage(pid)
	mem, _ := getMemoryUsage(pid)

	// Storage usage
	var storage int64
	size, err := dirSize(srv.VolumeName)
	if err == nil {
		storage = size
	}

	_ = w.store.InsertMetric(ctx, srv.ID, cpu, mem, players, maxPlayers, latencyPtr, storage)
	w.pub.PublishStatus(srv.ID.String(), map[string]any{
		"type":          "metrics",
		"server_id":     srv.ID.String(),
		"cpu_pct":       cpu,
		"mem_bytes":     mem,
		"player_count":  players,
		"max_players":   maxPlayers,
		"latency_ms":    latencyVal,
		"storage_bytes": storage,
		"at":            time.Now().UTC(),
	})
}

func (w *Worker) triggerHeartbeatRecovery(ctx context.Context, id uuid.UUID, reason string) {
	if !w.beginHealing(id) {
		return
	}
	w.mu.Lock()
	w.misses[id] = 0
	w.mu.Unlock()
	w.setState(ctx, id, db.StateRecovering, reason)
	go w.doRecover(ctx, id, "stuck_heartbeat", reason)
}

// setState persists a state change and broadcasts it.
func (w *Worker) setState(ctx context.Context, id uuid.UUID, state db.ServerState, message string) {
	_ = w.store.UpdateServerState(ctx, id, state)
	_ = w.store.InsertStateEvent(ctx, id, "", state, message)
	w.pub.PublishStatus(id.String(), map[string]any{
		"type":      "recovery",
		"server_id": id.String(),
		"state":     state,
		"message":   message,
		"at":        time.Now().UTC(),
	})
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func isProcessAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		openProcess := kernel32.NewProc("OpenProcess")
		// PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
		handle, _, _ := openProcess.Call(0x1000, 0, uintptr(pid))
		if handle == 0 {
			return false
		}
		defer syscall.CloseHandle(syscall.Handle(handle))

		getExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
		var exitCode uint32
		r, _, _ := getExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&exitCode)))
		if r == 0 {
			return false
		}
		const STILL_ACTIVE = 259
		return exitCode == STILL_ACTIVE
	} else {
		p, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		err = p.Signal(syscall.Signal(0))
		return err == nil
	}
}

func getMemoryUsage(pid int) (int64, error) {
	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		openProcess := kernel32.NewProc("OpenProcess")
		// PROCESS_QUERY_INFORMATION = 0x0400, PROCESS_VM_READ = 0x0010
		handle, _, _ := openProcess.Call(0x0400|0x0010, 0, uintptr(pid))
		if handle == 0 {
			return 0, fmt.Errorf("failed to open process")
		}
		defer syscall.CloseHandle(syscall.Handle(handle))

		psapi := syscall.NewLazyDLL("psapi.dll")
		getProcessMemoryInfo := psapi.NewProc("GetProcessMemoryInfo")
		var counters struct {
			CB                         uint32
			PageFaultCount             uint32
			PeakWorkingSetSize         uintptr
			WorkingSetSize             uintptr
			QuotaPeakWorkingSetSize    uintptr
			QuotaWorkingSetSize        uintptr
			QuotaPeakPagedPoolUsage    uintptr
			QuotaPagedPoolUsage        uintptr
			PeakPagefileUsage          uintptr
			PagefileUsage              uintptr
		}
		counters.CB = uint32(unsafe.Sizeof(counters))
		r, _, _ := getProcessMemoryInfo.Call(handle, uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB))
		if r == 0 {
			return 0, fmt.Errorf("failed to get memory info")
		}
		return int64(counters.WorkingSetSize), nil
	} else {
		cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
		out, err := cmd.Output()
		if err != nil {
			return 0, err
		}
		kbStr := strings.TrimSpace(string(out))
		kb, err := strconv.ParseInt(kbStr, 10, 64)
		if err != nil {
			return 0, err
		}
		return kb * 1024, nil
	}
}

type cpuTracker struct {
	mu          sync.Mutex
	lastTime    map[int]time.Time
	lastCPUTime map[int]int64
}

var tracker = &cpuTracker{
	lastTime:    make(map[int]time.Time),
	lastCPUTime: make(map[int]int64),
}

func getCPUUsage(pid int) (float64, error) {
	var cpuTime int64

	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		openProcess := kernel32.NewProc("OpenProcess")
		handle, _, _ := openProcess.Call(0x0400, 0, uintptr(pid))
		if handle == 0 {
			return 0, fmt.Errorf("failed to open process")
		}
		defer syscall.CloseHandle(syscall.Handle(handle))

		getProcessTimes := kernel32.NewProc("GetProcessTimes")
		var creationTime, exitTime, kernelTime, userTime struct {
			LowDateTime  uint32
			HighDateTime uint32
		}
		r, _, _ := getProcessTimes.Call(
			handle,
			uintptr(unsafe.Pointer(&creationTime)),
			uintptr(unsafe.Pointer(&exitTime)),
			uintptr(unsafe.Pointer(&kernelTime)),
			uintptr(unsafe.Pointer(&userTime)),
		)
		if r == 0 {
			return 0, fmt.Errorf("failed to get process times")
		}
		kTime := (int64(kernelTime.HighDateTime) << 32) + int64(kernelTime.LowDateTime)
		uTime := (int64(userTime.HighDateTime) << 32) + int64(userTime.LowDateTime)
		cpuTime = (kTime + uTime) * 100
	} else {
		cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=")
		out, err := cmd.Output()
		if err != nil {
			return 0, err
		}
		pctStr := strings.TrimSpace(string(out))
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			return 0, err
		}
		return pct, nil
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	now := time.Now()
	prevTime, ok1 := tracker.lastTime[pid]
	prevCPUTime, ok2 := tracker.lastCPUTime[pid]

	tracker.lastTime[pid] = now
	tracker.lastCPUTime[pid] = cpuTime

	if !ok1 || !ok2 {
		return 0, nil
	}

	timeDelta := now.Sub(prevTime).Nanoseconds()
	cpuDelta := cpuTime - prevCPUTime

	if timeDelta <= 0 {
		return 0, nil
	}

	pct := (float64(cpuDelta) / float64(timeDelta)) * 100.0
	pct = pct / float64(runtime.NumCPU())
	if pct < 0 {
		pct = 0
	}
	return pct, nil
}
