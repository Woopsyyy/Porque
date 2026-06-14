package playit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
)

// StatusPublisher fans out tunnel status changes (ws.Hub satisfies it).
type StatusPublisher interface {
	PublishStatus(topic string, payload any)
}

type nopPublisher struct{}

func (nopPublisher) PublishStatus(string, any) {}

// Manager attaches/detaches Playit agent processes to servers and tracks them.
type Manager struct {
	store      *db.Store
	client     PlayitClient
	pub        StatusPublisher
	appDataDir string

	mu        sync.RWMutex
	claiming  bool
	claimCode string
	claimURL  string
}

// NewManager wires a tunnel manager. client and pub may be nil.
func NewManager(store *db.Store, client PlayitClient, pub StatusPublisher, appDataDir string) *Manager {
	if client == nil {
		client = StubClient{}
	}
	if pub == nil {
		pub = nopPublisher{}
	}
	return &Manager{store: store, client: client, pub: pub, appDataDir: appDataDir}
}

func sidecarName(serverName string) string { return "mc-playit-" + slugify(serverName) }

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

// statusTopic is the ws topic for a server's tunnel events.
func statusTopic(serverID uuid.UUID) string { return "playit:" + serverID.String() }

// Attach launches a Playit agent process for a running server using the given
// account's secret key.
func (m *Manager) Attach(ctx context.Context, serverID, accountID uuid.UUID) (*db.ServerTunnel, error) {
	srv, err := m.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	inst, err := m.store.LatestInstance(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv.State != db.StateRunning || inst == nil || inst.ContainerID == nil {
		return nil, apperr.BadState("server must be running to attach a tunnel")
	}
	if existing, _ := m.store.ActiveServerTunnel(ctx, serverID); existing != nil {
		return nil, apperr.Conflict("server already has an active tunnel")
	}
	account, err := m.store.GetPlayitAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	playitPath, err := DownloadPlayitAgent(m.appDataDir)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("failed to download playit agent: %w", err))
	}

	cmd := exec.Command(playitPath, "run", "--secret", account.SecretKey)
	cmd.Dir = srv.VolumeName

	logFile, err := os.OpenFile(filepath.Join(srv.VolumeName, "playit.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return nil, apperr.Internal(fmt.Errorf("failed to start playit agent: %w", err))
	}
	if logFile != nil {
		logFile.Close()
	}

	pidStr := strconv.Itoa(cmd.Process.Pid)

	// Monitor the playit process in the background
	go func() {
		_ = cmd.Wait()
	}()

	t := &db.ServerTunnel{
		ServerID:           serverID,
		PlayitAccountID:    &accountID,
		SidecarContainerID: &pidStr,
		Status:             db.TunnelStarting,
		Active:             true,
	}
	if err := m.store.CreateServerTunnel(ctx, t); err != nil {
		// Kill the process if database insert fails
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, err
	}
	m.publish(serverID, t)
	return t, nil
}

// Detach stops a server's tunnel process and deactivates the record.
func (m *Manager) Detach(ctx context.Context, serverID uuid.UUID) error {
	if t, _ := m.store.ActiveServerTunnel(ctx, serverID); t != nil {
		if t.SidecarContainerID != nil && *t.SidecarContainerID != "" {
			pid, err := strconv.Atoi(*t.SidecarContainerID)
			if err == nil {
				p, err := os.FindProcess(pid)
				if err == nil {
					_ = p.Kill()
					_, _ = p.Wait()
				}
			}
		}
	}
	if err := m.store.DeactivateServerTunnels(ctx, serverID); err != nil {
		return err
	}
	m.pub.PublishStatus(statusTopic(serverID), map[string]any{
		"server_id": serverID.String(),
		"status":    db.TunnelDisconnected,
		"at":        time.Now().UTC(),
	})
	return nil
}

// DetachProto removes one protocol's tunnel; the agent process is stopped only
// when no protocols remain attached.
func (m *Manager) DetachProto(ctx context.Context, serverID uuid.UUID, proto string) error {
	if err := m.store.DeactivateServerTunnelByProto(ctx, serverID, proto); err != nil {
		return err
	}
	remaining, _ := m.store.ActiveServerTunnels(ctx, serverID)
	if len(remaining) == 0 {
		_ = m.Detach(ctx, serverID)
	} else {
		m.pub.PublishStatus(statusTopic(serverID), map[string]any{
			"server_id": serverID.String(),
			"status":    db.TunnelDisconnected,
			"proto":     proto,
			"at":        time.Now().UTC(),
		})
	}
	return nil
}

// Status returns the active tunnels for a server (one per protocol).
func (m *Manager) Status(ctx context.Context, serverID uuid.UUID) ([]db.ServerTunnel, error) {
	if _, err := m.store.GetServer(ctx, serverID); err != nil {
		return nil, err
	}
	return m.store.ActiveServerTunnels(ctx, serverID)
}

// ListActive returns all active tunnels across servers.
func (m *Manager) ListActive(ctx context.Context) ([]db.ServerTunnel, error) {
	return m.store.ListActiveTunnels(ctx)
}

func (m *Manager) publish(serverID uuid.UUID, t *db.ServerTunnel) {
	m.pub.PublishStatus(statusTopic(serverID), map[string]any{
		"server_id":      serverID.String(),
		"status":         t.Status,
		"public_address": t.PublicAddress,
		"at":             time.Now().UTC(),
	})
}

// EnsureBundledAccount triggers a background goroutine to claim a Playit.gg agent
// if no accounts exist in the store and a claim is not already active.
func (m *Manager) EnsureBundledAccount(ctx context.Context) {
	m.mu.Lock()
	if m.claiming {
		m.mu.Unlock()
		return
	}
	accts, err := m.store.ListPlayitAccounts(ctx)
	if err != nil || len(accts) > 0 {
		m.mu.Unlock()
		return
	}

	m.claiming = true
	m.mu.Unlock()

	go func() {
		claimCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		code, url, err := m.client.StartClaim(claimCtx)
		if err != nil {
			m.mu.Lock()
			m.claiming = false
			m.mu.Unlock()
			return
		}

		m.mu.Lock()
		m.claimCode = code
		m.claimURL = url
		m.mu.Unlock()

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-claimCtx.Done():
				m.mu.Lock()
				m.claiming = false
				m.claimCode = ""
				m.claimURL = ""
				m.mu.Unlock()
				return
			case <-ticker.C:
				secret, err := m.client.PollClaim(claimCtx, code)
				if err != nil {
					m.mu.Lock()
					m.claiming = false
					m.claimCode = ""
					m.claimURL = ""
					m.mu.Unlock()
					return
				}
				if secret != "" {
					acct, err := m.store.CreatePlayitAccount(context.Background(), "Bundled Account", secret)
					if err == nil {
						_ = m.SyncTunnels(context.Background(), acct.ID)
					}
					m.mu.Lock()
					m.claiming = false
					m.claimCode = ""
					m.claimURL = ""
					m.mu.Unlock()
					return
				}
			}
		}
	}()
}

// GetClaimStatus returns whether a claim is active, and if so, the code and url.
func (m *Manager) GetClaimStatus() (claiming bool, code, url string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.claiming, m.claimCode, m.claimURL
}

// SyncTunnels lists all servers, fetches playit.gg tunnels for the connected account,
// detaches any active old tunnels, and attaches new ones using the new account credentials.
func (m *Manager) SyncTunnels(ctx context.Context, accountID uuid.UUID) error {
	account, err := m.store.GetPlayitAccount(ctx, accountID)
	if err != nil {
		return err
	}

	tunnels, err := m.client.ListTunnels(ctx, account.SecretKey)
	if err != nil {
		return err
	}

	var playitMcTunnels []Tunnel
	for _, t := range tunnels {
		if t.Proto == "tcp" || t.Proto == "minecraft-java" {
			playitMcTunnels = append(playitMcTunnels, t)
		}
	}

	if len(playitMcTunnels) == 0 {
		agentID, err := m.client.GetAgentID(ctx, account.SecretKey)
		if err == nil && agentID != "" {
			newT, err := m.client.CreateTunnel(ctx, account.SecretKey, agentID, "tcp", 25565)
			if err == nil && newT != nil {
				playitMcTunnels = append(playitMcTunnels, *newT)
			}
		}
	}

	servers, err := m.store.ListServers(ctx)
	if err != nil {
		return err
	}

	assignedTunnels := make(map[string]bool)

	for _, srv := range servers {
		activeT, _ := m.store.ActiveServerTunnel(ctx, srv.ID)
		if activeT != nil || srv.State == db.StateRunning {
			var matchedTunnel *Tunnel
			for _, pt := range playitMcTunnels {
				if !assignedTunnels[pt.ID] && (strings.Contains(strings.ToLower(pt.Name), strings.ToLower(srv.Name)) || strings.Contains(strings.ToLower(srv.Name), strings.ToLower(pt.Name))) {
					matchedTunnel = &pt
					break
				}
			}
			if matchedTunnel == nil {
				for _, pt := range playitMcTunnels {
					if !assignedTunnels[pt.ID] {
						matchedTunnel = &pt
						break
					}
				}
			}

			var publicAddr string
			if matchedTunnel != nil {
				assignedTunnels[matchedTunnel.ID] = true
				publicAddr = matchedTunnel.PublicAddress
			}

			if activeT != nil {
				_ = m.Detach(ctx, srv.ID)
			}

			if srv.State == db.StateRunning {
				newT, err := m.Attach(ctx, srv.ID, accountID)
				if err == nil && newT != nil {
					status := db.TunnelConnected
					if publicAddr == "" {
						status = db.TunnelStarting
					}
					var addrPtr *string
					if publicAddr != "" {
						addrPtr = &publicAddr
					}
					_ = m.store.UpdateServerTunnel(ctx, newT.ID, status, addrPtr)
				}
			}
		}
	}
	return nil
}

// ListTunnelsFromAPI lists tunnels directly from playit.gg API using the secret key.
func (m *Manager) ListTunnelsFromAPI(ctx context.Context, secretKey string) ([]Tunnel, error) {
	return m.client.ListTunnels(ctx, secretKey)
}

func (m *Manager) ensureSidecar(ctx context.Context, srv *db.Server, account *db.PlayitAccount) (string, error) {
	if existing, _ := m.store.ActiveServerTunnels(ctx, srv.ID); len(existing) > 0 {
		for _, t := range existing {
			if t.SidecarContainerID != nil && *t.SidecarContainerID != "" {
				return *t.SidecarContainerID, nil
			}
		}
	}

	playitPath, err := DownloadPlayitAgent(m.appDataDir)
	if err != nil {
		return "", apperr.Internal(fmt.Errorf("failed to download playit agent: %w", err))
	}

	cmd := exec.Command(playitPath, "run", "--secret", account.SecretKey)
	cmd.Dir = srv.VolumeName

	logFile, err := os.OpenFile(filepath.Join(srv.VolumeName, "playit.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return "", apperr.Internal(fmt.Errorf("failed to start playit agent: %w", err))
	}
	if logFile != nil {
		logFile.Close()
	}

	pidStr := strconv.Itoa(cmd.Process.Pid)

	go func() {
		_ = cmd.Wait()
	}()

	return pidStr, nil
}

// CreateTunnelForServer creates (or replaces) a server's Playit tunnel for the
// given kind: "java" => TCP/25565, "bedrock" => UDP/19132 (Geyser).
func (m *Manager) CreateTunnelForServer(ctx context.Context, serverID uuid.UUID, kind string) (*db.ServerTunnel, error) {
	proto, localPort := "tcp", 25565
	if kind == "bedrock" {
		proto, localPort = "udp", 19132
	}

	srv, err := m.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	inst, err := m.store.LatestInstance(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv.State != db.StateRunning || inst == nil || inst.ContainerID == nil {
		return nil, apperr.BadState("server must be running to create a tunnel")
	}

	accts, err := m.store.ListPlayitAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(accts) == 0 {
		m.EnsureBundledAccount(ctx)
		return nil, apperr.BadState("Linking your Playit.gg account — approve it in your browser, then try again.")
	}
	account := accts[0]

	if existing, _ := m.store.ActiveTunnelByProto(ctx, serverID, proto); existing != nil {
		_ = m.store.DeactivateServerTunnelByProto(ctx, serverID, proto)
	}

	cid, err := m.ensureSidecar(ctx, srv, &account)
	if err != nil {
		return nil, err
	}

	var addr *string
	status := db.TunnelStarting
	if agentID, aerr := m.client.GetAgentID(ctx, account.SecretKey); aerr == nil && agentID != "" {
		if t, terr := m.client.CreateTunnel(ctx, account.SecretKey, agentID, proto, localPort); terr == nil && t != nil && t.PublicAddress != "" {
			a := t.PublicAddress
			addr, status = &a, db.TunnelConnected
		}
	}

	row := &db.ServerTunnel{
		ServerID:           serverID,
		PlayitAccountID:    &account.ID,
		SidecarContainerID: &cid,
		PublicAddress:      addr,
		Proto:              proto,
		Status:             status,
		Active:             true,
	}
	if err := m.store.CreateServerTunnel(ctx, row); err != nil {
		return nil, err
	}
	m.publish(serverID, row)
	return row, nil
}

// Rescan re-queries the Playit API for each active tunnel's public address
// (matched by protocol) and updates the stored records.
func (m *Manager) Rescan(ctx context.Context, serverID uuid.UUID) ([]db.ServerTunnel, error) {
	active, err := m.store.ActiveServerTunnels(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, apperr.BadState("no active tunnel to rescan")
	}
	accts, err := m.store.ListPlayitAccounts(ctx)
	if err != nil || len(accts) == 0 {
		return active, nil
	}
	tunnels, err := m.client.ListTunnels(ctx, accts[0].SecretKey)
	if err != nil {
		return active, nil
	}

	byProto := map[string]string{}
	for _, t := range tunnels {
		if t.PublicAddress == "" {
			continue
		}
		p := t.Proto
		switch p {
		case "minecraft-java":
			p = "tcp"
		case "minecraft-bedrock":
			p = "udp"
		}
		if _, ok := byProto[p]; !ok {
			byProto[p] = t.PublicAddress
		}
	}

	for i := range active {
		if addr, ok := byProto[active[i].Proto]; ok && addr != "" {
			_ = m.store.UpdateServerTunnel(ctx, active[i].ID, db.TunnelConnected, &addr)
			active[i].PublicAddress = &addr
			active[i].Status = db.TunnelConnected
			m.publish(serverID, &active[i])
		}
	}
	return active, nil
}
