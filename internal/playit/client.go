package playit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

const PlayitAPIBase = "https://api.playit.gg"

// Tunnel is tunnel metadata as surfaced by a Playit account.
type Tunnel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PublicAddress string `json:"public_address"`
	Proto         string `json:"proto"`
	LocalPort     int    `json:"local_port"`
	Status        string `json:"status"`
}

// PlayitClient interface details for both claiming and metadata tracking.
type PlayitClient interface {
	ListTunnels(ctx context.Context, secretKey string) ([]Tunnel, error)
	StartClaim(ctx context.Context) (code string, claimURL string, err error)
	PollClaim(ctx context.Context, code string) (string, error)
	GetAgentID(ctx context.Context, secretKey string) (string, error)
	CreateTunnel(ctx context.Context, secretKey, agentID, proto string, localPort int) (*Tunnel, error)
}

// StubClient is the default no-op client.
type StubClient struct{}

// ListTunnels always returns an empty set.
func (StubClient) ListTunnels(ctx context.Context, secretKey string) ([]Tunnel, error) {
	return nil, nil
}

// StartClaim returns a dummy code and url.
func (StubClient) StartClaim(ctx context.Context) (string, string, error) {
	return "", "", nil
}

// PollClaim returns nothing.
func (StubClient) PollClaim(ctx context.Context, code string) (string, error) {
	return "", nil
}

// GetAgentID returns a dummy agent ID.
func (StubClient) GetAgentID(ctx context.Context, secretKey string) (string, error) {
	return "", nil
}

// CreateTunnel returns a dummy tunnel.
func (StubClient) CreateTunnel(ctx context.Context, secretKey, agentID, proto string, localPort int) (*Tunnel, error) {
	return nil, nil
}

// HTTPPlayitClient implements PlayitClient using real HTTP API requests.
type HTTPPlayitClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPPlayitClient {
	return &HTTPPlayitClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type playitEnvelope struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

type runDataResponse struct {
	AgentID string            `json:"agent_id"`
	Tunnels []rawRunDataProto `json:"tunnels"`
}

type rawRunDataProto struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Proto          string `json:"proto"`
	PortType       string `json:"port_type"`
	TunnelType     string `json:"tunnel_type"`
	LocalPort      int    `json:"local_port"`
	LocalIP        string `json:"local_ip"`
	AssignedDomain string `json:"assigned_domain"`
	Port           struct {
		From int `json:"from"`
	} `json:"port"`
}

// formatPublicAddress builds the public address string for a tunnel.
// Java (minecraft-java / .joinmc.link) tunnels use SRV records and don't
// require a port suffix. Bedrock and other tunnel types use host:port.
// Returns empty string when the domain hasn't been assigned yet.
func formatPublicAddress(t rawRunDataProto) string {
	if t.AssignedDomain == "" {
		return ""
	}
	// minecraft-java tunnels use SRV records via joinmc.link – no port needed
	if t.TunnelType == "minecraft-java" {
		return t.AssignedDomain
	}
	if t.Port.From == 0 {
		return t.AssignedDomain
	}
	return fmt.Sprintf("%s:%d", t.AssignedDomain, t.Port.From)
}

func effectiveProto(t rawRunDataProto) string {
	if t.TunnelType == "minecraft-java" {
		return "tcp"
	}
	if t.TunnelType == "minecraft-bedrock" {
		return "udp"
	}
	if t.Proto != "" {
		return t.Proto
	}
	if t.PortType != "" {
		return t.PortType
	}
	return ""
}

// ListTunnels queries the run data for the agent key.
func (c *HTTPPlayitClient) ListTunnels(ctx context.Context, secretKey string) ([]Tunnel, error) {
	url := fmt.Sprintf("%s/agents/rundata", PlayitAPIBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("agent-key %s", secretKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playit rundata status: %d", resp.StatusCode)
	}

	var env playitEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}

	if env.Status != "success" {
		var errMsg string
		_ = json.Unmarshal(env.Data, &errMsg)
		return nil, fmt.Errorf("playit API error: %s", errMsg)
	}

	var rd runDataResponse
	if err := json.Unmarshal(env.Data, &rd); err != nil {
		return nil, err
	}

	var tunnels []Tunnel
	for _, t := range rd.Tunnels {
		tunnels = append(tunnels, Tunnel{
			ID:            t.ID,
			Name:          t.Name,
			PublicAddress: formatPublicAddress(t),
			Proto:         effectiveProto(t),
			LocalPort:     t.LocalPort,
			Status:        "connected",
		})
	}
	return tunnels, nil
}


// StartClaim starts the claiming process by submitting a random 5-byte code.
func (c *HTTPPlayitClient) StartClaim(ctx context.Context) (string, string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	code := hex.EncodeToString(b)

	payload, _ := json.Marshal(map[string]string{
		"code":       code,
		"agent_type": "self-managed",
		"version":    "porque-backend 1.0",
	})

	url := fmt.Sprintf("%s/claim/setup", PlayitAPIBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var env playitEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", "", err
	}

	claimURL := fmt.Sprintf("https://playit.gg/claim/%s", code)
	return code, claimURL, nil
}

// PollClaim polls /claim/setup and exchanges it on success for the secret key.
func (c *HTTPPlayitClient) PollClaim(ctx context.Context, code string) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"code":       code,
		"agent_type": "self-managed",
		"version":    "porque-backend 1.0",
	})

	url := fmt.Sprintf("%s/claim/setup", PlayitAPIBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var env playitEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", err
	}

	var status string
	_ = json.Unmarshal(env.Data, &status)

	if status == "UserRejected" {
		return "", fmt.Errorf("user rejected the agent claim")
	}

	if status != "UserAccepted" {
		return "", nil // Still polling
	}

	// Approved! Exchange for secret key
	exchPayload, _ := json.Marshal(map[string]string{"code": code})
	exchURL := fmt.Sprintf("%s/claim/exchange", PlayitAPIBase)
	
	reqExch, err := http.NewRequestWithContext(ctx, "POST", exchURL, bytes.NewReader(exchPayload))
	if err != nil {
		return "", err
	}
	reqExch.Header.Set("Content-Type", "application/json")

	respExch, err := c.client.Do(reqExch)
	if err != nil {
		return "", err
	}
	defer respExch.Body.Close()

	var envExch playitEnvelope
	if err := json.NewDecoder(respExch.Body).Decode(&envExch); err != nil {
		return "", err
	}

	if envExch.Status != "success" {
		var errMsg string
		_ = json.Unmarshal(envExch.Data, &errMsg)
		return "", fmt.Errorf("exchange failed: %s", errMsg)
	}

	dataStr := string(envExch.Data)
	re := regexp.MustCompile(`[a-f0-9]{64}`)
	match := re.FindString(dataStr)
	if match == "" {
		return "", fmt.Errorf("secret key not found in response data: %s", dataStr)
	}

	return match, nil
}

// GetAgentID queries the run data and returns the agent_id.
func (c *HTTPPlayitClient) GetAgentID(ctx context.Context, secretKey string) (string, error) {
	url := fmt.Sprintf("%s/agents/rundata", PlayitAPIBase)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("agent-key %s", secretKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("playit GetAgentID status: %d", resp.StatusCode)
	}

	var env playitEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", err
	}

	if env.Status != "success" {
		var errMsg string
		_ = json.Unmarshal(env.Data, &errMsg)
		return "", fmt.Errorf("playit API error: %s", errMsg)
	}

	var rd runDataResponse
	if err := json.Unmarshal(env.Data, &rd); err != nil {
		return "", err
	}

	return rd.AgentID, nil
}

// CreateTunnel creates a new tunnel on playit.gg. proto is "tcp" (Java) or
// "udp" (Bedrock); the tunnel name/type are derived from it.
func (c *HTTPPlayitClient) CreateTunnel(ctx context.Context, secretKey, agentID, proto string, localPort int) (*Tunnel, error) {
	name, tunnelType := "Minecraft Java", "minecraft-java"
	if proto == "udp" {
		name, tunnelType = "Minecraft Bedrock", "minecraft-bedrock"
	}
	url := fmt.Sprintf("%s/tunnels/create", PlayitAPIBase)
	body := map[string]any{
		"name":        name,
		"tunnel_type": tunnelType,
		"port_type":   proto,
		"port_count":  1,
		"origin": map[string]any{
			"type": "agent",
			"data": map[string]any{
				"agent_id":   agentID,
				"local_ip":   "127.0.0.1",
				"local_port": localPort,
			},
		},
		"enabled":          true,
		"alloc":            nil,
		"firewall_id":      nil,
		"proxy_protocol":   nil,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("agent-key %s", secretKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create tunnel status: %d", resp.StatusCode)
	}

	var env playitEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}

	if env.Status != "success" {
		var errMsg string
		_ = json.Unmarshal(env.Data, &errMsg)
		return nil, fmt.Errorf("playit API error: %s", errMsg)
	}

	var t rawRunDataProto
	if err := json.Unmarshal(env.Data, &t); err != nil {
		return nil, err
	}

	return &Tunnel{
		ID:            t.ID,
		Name:          t.Name,
		PublicAddress: formatPublicAddress(t),
		Proto:         effectiveProto(t),
		LocalPort:     t.LocalPort,
		Status:        "connected",
	}, nil
}
