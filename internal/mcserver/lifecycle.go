package mcserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
	"github.com/woopsy/porque/internal/rcon"
)

// StatusPublisher receives lifecycle state changes for real-time fan-out.
type StatusPublisher interface {
	PublishStatus(serverID string, payload any)
}

// nopPublisher is the default when no hub is wired.
type nopPublisher struct{}

func (nopPublisher) PublishStatus(string, any) {}

// Controller drives server lifecycle, persisting state and broadcasting changes.
type Controller struct {
	store *db.Store
	pub   StatusPublisher
}

// NewController wires a lifecycle controller. pub may be nil.
func NewController(store *db.Store, pub StatusPublisher) *Controller {
	if pub == nil {
		pub = nopPublisher{}
	}
	return &Controller{store: store, pub: pub}
}

// CreateParams are the user-supplied inputs to create a server.
type CreateParams struct {
	Name          string
	Type          db.ServerType
	Version       string
	LoaderVersion string
	MemoryMB      int
	CPUCores      float64
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

// Create validates inputs, provisions a local directory, and records a new server.
func (c *Controller) Create(ctx context.Context, p CreateParams) (*db.Server, error) {
	if len(strings.TrimSpace(p.Name)) < 1 || len(p.Name) > 64 {
		return nil, apperr.Validation("name must be between 1 and 64 characters")
	}
	if !validType(p.Type) {
		return nil, apperr.Validation("server_type must be one of VANILLA, PAPER, FABRIC, FORGE")
	}
	if p.Version == "" {
		return nil, apperr.Validation("version is required")
	}
	if p.MemoryMB == 0 {
		p.MemoryMB = 2048
	}
	if p.MemoryMB < 512 {
		return nil, apperr.Validation("memory_mb must be at least 512")
	}

	rconPw, err := randomSecret(16)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	// Determine local directory path
	var volName string
	serversPath, errSrv := c.store.GetSetting(ctx, "servers_path")
	if errSrv == nil && serversPath != "" {
		volName = filepath.Join(serversPath, slugify(p.Name))
	} else {
		// Fallback to local user home folder or current directory if not set
		userHome, err := os.UserHomeDir()
		if err == nil {
			volName = filepath.Join(userHome, ".porque", "servers", slugify(p.Name))
		} else {
			volName = filepath.Join(".", "servers", slugify(p.Name))
		}
	}

	srv := &db.Server{
		Name:         p.Name,
		ServerType:   p.Type,
		Version:      p.Version,
		Image:        "native",
		MemoryMB:     p.MemoryMB,
		CPUCores:     1.0,
		RconPassword: rconPw,
		VolumeName:   volName,
		State:        db.StateCreating,
	}
	if p.LoaderVersion != "" {
		srv.LoaderVersion = &p.LoaderVersion
	}

	if err := c.store.CreateServer(ctx, srv); err != nil {
		return nil, err
	}

	// Create local server folder
	if err := os.MkdirAll(srv.VolumeName, 0o755); err != nil {
		c.transition(ctx, srv, db.StateCorrupted, "folder creation failed: "+err.Error())
		return nil, apperr.Internal(err)
	}

	c.transition(ctx, srv, db.StateStopped, "created")
	return srv, nil
}

// Start downloads the server jar, configures it, and spawns the Java process.
func (c *Controller) Start(ctx context.Context, id uuid.UUID) (*db.Server, error) {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return nil, err
	}
	switch srv.State {
	case db.StateRunning, db.StateStarting:
		return nil, apperr.BadState("server is already running")
	}

	c.transition(ctx, srv, db.StateStarting, "starting")

	// Ensure the server directory exists
	if err := os.MkdirAll(srv.VolumeName, 0o755); err != nil {
		c.transition(ctx, srv, db.StateCrashed, "failed to create directory: "+err.Error())
		return nil, apperr.Internal(err)
	}

	// Download server jar if not present
	jarName, err := downloadServerJar(srv.VolumeName, string(srv.ServerType), srv.Version)
	if err != nil {
		c.transition(ctx, srv, db.StateCrashed, "jar download failed: "+err.Error())
		return nil, apperr.Internal(err)
	}

	// Write eula.txt and server.properties
	if err := writeEula(srv.VolumeName); err != nil {
		c.transition(ctx, srv, db.StateCrashed, "eula write failed: "+err.Error())
		return nil, apperr.Internal(err)
	}
	if err := writeServerProperties(srv.VolumeName, srv); err != nil {
		c.transition(ctx, srv, db.StateCrashed, "properties write failed: "+err.Error())
		return nil, apperr.Internal(err)
	}

	// Start the local Java process: java -Xms<mem>M -Xmx<mem>M -jar <jarName> nogui
	cmd := exec.Command("java",
		fmt.Sprintf("-Xms%dM", srv.MemoryMB),
		fmt.Sprintf("-Xmx%dM", srv.MemoryMB),
		"-jar", jarName,
		"nogui",
	)
	cmd.Dir = srv.VolumeName

	// Open or create log file
	logFile, err := os.OpenFile(filepath.Join(srv.VolumeName, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		c.transition(ctx, srv, db.StateCrashed, "failed to open log file: "+err.Error())
		return nil, apperr.Internal(err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		c.transition(ctx, srv, db.StateCrashed, "java process failed to start: "+err.Error())
		return nil, apperr.Internal(err)
	}

	// Close write handle in Go, as the OS now owns the stdout/stderr file descriptor
	logFile.Close()

	pidStr := strconv.Itoa(cmd.Process.Pid)
	now := time.Now()
	mcPort := 25565
	rconPort := 25575

	inst := &db.ServerInstance{
		ServerID:     srv.ID,
		ContainerID:  &pidStr,
		MCHostPort:   &mcPort,
		RconHostPort: &rconPort,
		StartedAt:    &now,
	}
	if err := c.store.CreateInstance(ctx, inst); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}

	// Monitor the process in a separate goroutine
	go func(s *db.Server, instanceID uuid.UUID) {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				exitCode = -1
			}
		}

		_ = c.store.MarkInstanceStopped(context.Background(), instanceID, &exitCode)

		if exitCode == 0 || exitCode == 130 { // 130 is SIGINT/Ctrl+C
			c.transition(context.Background(), s, db.StateStopped, "stopped")
		} else {
			c.transition(context.Background(), s, db.StateCrashed, fmt.Sprintf("exited with code %d", exitCode))
		}
	}(srv, inst.ID)

	c.transition(ctx, srv, db.StateRunning, fmt.Sprintf("running natively on port %d (PID %s)", mcPort, pidStr))
	return srv, nil
}

// Stop gracefully stops the running server.
func (c *Controller) Stop(ctx context.Context, id uuid.UUID) error {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return err
	}
	inst, err := c.store.LatestInstance(ctx, srv.ID)
	if err != nil {
		return err
	}
	if inst == nil || inst.ContainerID == nil || *inst.ContainerID == "" {
		return apperr.BadState("server has no running process")
	}

	pid, err := strconv.Atoi(*inst.ContainerID)
	if err != nil {
		return apperr.Internal(err)
	}

	c.transition(ctx, srv, db.StateStopping, "stopping")

	// Try graceful stop via RCON first
	stoppedGracefully := false
	rc := rcon.New("127.0.0.1:25575", srv.RconPassword)
	if _, err := rc.Run(ctx, "stop"); err == nil {
		// Wait for process to exit up to 30 seconds
		for i := 0; i < 60; i++ {
			if !isProcessAlive(pid) {
				stoppedGracefully = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Force kill if RCON failed or timed out
	if !stoppedGracefully && isProcessAlive(pid) {
		p, err := os.FindProcess(pid)
		if err == nil {
			_ = p.Kill()
			_, _ = p.Wait()
		}
	}

	// Exit code is nil on manual stops
	_ = c.store.MarkInstanceStopped(ctx, inst.ID, nil)
	c.transition(ctx, srv, db.StateStopped, "stopped")
	return nil
}

// EnsureStopped stops and kills any running process for the server. Safe to call when already stopped.
func (c *Controller) EnsureStopped(ctx context.Context, id uuid.UUID) error {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return err
	}
	if inst, _ := c.store.LatestInstance(ctx, srv.ID); inst != nil && inst.ContainerID != nil && *inst.ContainerID != "" {
		pid, err := strconv.Atoi(*inst.ContainerID)
		if err == nil && isProcessAlive(pid) {
			rc := rcon.New("127.0.0.1:25575", srv.RconPassword)
			_, _ = rc.Run(ctx, "stop")
			time.Sleep(2 * time.Second)

			if isProcessAlive(pid) {
				p, err := os.FindProcess(pid)
				if err == nil {
					_ = p.Kill()
					_, _ = p.Wait()
				}
			}
			_ = c.store.MarkInstanceStopped(ctx, inst.ID, nil)
		}
	}
	if srv.State != db.StateStopped {
		c.transition(ctx, srv, db.StateStopped, "stopped for restore")
	}
	return nil
}

// Restart stops and restarts the server process.
func (c *Controller) Restart(ctx context.Context, id uuid.UUID) error {
	if err := c.Stop(ctx, id); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	_, err := c.Start(ctx, id)
	return err
}

// Delete stops/removes the process and server directory, then deletes the server record.
func (c *Controller) Delete(ctx context.Context, id uuid.UUID) error {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return err
	}
	_ = c.EnsureStopped(ctx, id)

	// Remove server folder
	_ = os.RemoveAll(srv.VolumeName)

	return c.store.DeleteServer(ctx, srv.ID)
}

// Storage returns the on-disk size (bytes) of the server's directory read natively.
func (c *Controller) Storage(ctx context.Context, id uuid.UUID) (bytes int64, available bool, err error) {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return 0, false, err
	}

	size, err := dirSize(srv.VolumeName)
	if err != nil {
		return 0, false, nil
	}
	return size, true, nil
}

// FollowSidecarLogs streams the Playit agent's logs from the server directory.
func (c *Controller) FollowSidecarLogs(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return nil, err
	}
	t, err := c.store.ActiveServerTunnel(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil || t.SidecarContainerID == nil || *t.SidecarContainerID == "" {
		return nil, apperr.BadState("no active tunnel")
	}
	logPath := filepath.Join(srv.VolumeName, "playit.log")
	return newTailReader(logPath)
}

// FollowLogs returns the live log stream of the server.log file.
func (c *Controller) FollowLogs(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	srv, err := c.store.GetServer(ctx, id)
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join(srv.VolumeName, "server.log")
	return newTailReader(logPath)
}

// transition persists a state change, appends an audit event, and broadcasts.
func (c *Controller) transition(ctx context.Context, srv *db.Server, to db.ServerState, message string) {
	from := srv.State
	_ = c.store.UpdateServerState(ctx, srv.ID, to)
	_ = c.store.InsertStateEvent(ctx, srv.ID, from, to, message)
	srv.State = to
	c.pub.PublishStatus(srv.ID.String(), map[string]any{
		"server_id": srv.ID.String(),
		"state":     to,
		"message":   message,
		"at":        time.Now().UTC(),
	})
}

func validType(t db.ServerType) bool {
	switch t {
	case db.TypeVanilla, db.TypePaper, db.TypeFabric, db.TypeForge:
		return true
	}
	return false
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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

type tailReader struct {
	file   *os.File
	ctx    context.Context
	cancel context.CancelFunc
}

func (t *tailReader) Read(p []byte) (int, error) {
	for {
		select {
		case <-t.ctx.Done():
			return 0, io.EOF
		default:
			n, err := t.file.Read(p)
			if err == io.EOF {
				if n > 0 {
					return n, nil
				}
				time.Sleep(250 * time.Millisecond)
				continue
			}
			return n, err
		}
	}
}

func (t *tailReader) Close() error {
	t.cancel()
	return t.file.Close()
}

func newTailReader(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err == nil && fi.Size() > 50000 {
		_, _ = f.Seek(-50000, io.SeekEnd)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &tailReader{file: f, ctx: ctx, cancel: cancel}, nil
}
