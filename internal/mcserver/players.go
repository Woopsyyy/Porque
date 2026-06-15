package mcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/woopsy/porque/internal/apperr"
	"github.com/woopsy/porque/internal/db"
	"github.com/woopsy/porque/internal/rcon"
)

// bedrockPrefix is Floodgate's default username prefix for Bedrock players who
// join a Java server through Geyser. Names starting with it are Bedrock.
const bedrockPrefix = "."

// Player describes a player for the UI player panel.
type Player struct {
	Name    string `json:"name"`
	Edition string `json:"edition"` // "java" or "bedrock"
	UUID    string `json:"uuid"`
}

// editionFor classifies a player name as Java or Bedrock by the Geyser prefix.
func editionFor(name string) string {
	if strings.HasPrefix(name, bedrockPrefix) {
		return "bedrock"
	}
	return "java"
}

// ListOnlinePlayers returns the players currently connected to a running server
// via the RCON `list` command. Returns an empty slice if the server is stopped.
func (c *Controller) ListOnlinePlayers(ctx context.Context, serverID uuid.UUID) ([]Player, error) {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if srv.State != db.StateRunning {
		return []Player{}, nil
	}

	rc := rcon.New(rconAddrOf(srv), srv.RconPassword)
	out, err := rc.ListPlayers(ctx)
	if err != nil {
		return []Player{}, nil // server may be mid-start; treat as nobody online
	}

	players := make([]Player, 0)
	for _, name := range parseOnlineNames(out) {
		players = append(players, Player{Name: name, Edition: editionFor(name)})
	}
	return players, nil
}

// onlineHeaderRe matches the standard `list` header in both vanilla/Paper
// ("There are 1 of a max of 50 players online:") and slash ("1/50") forms,
// capturing everything after the colon as the comma-separated name list.
var onlineHeaderRe = regexp.MustCompile(`(?i)players online:?\s*(.*)`)

// colorCodeRe matches Minecraft section-sign color/format codes (§a, §l, …).
var colorCodeRe = regexp.MustCompile("§.")

func stripColorCodes(s string) string {
	return colorCodeRe.ReplaceAllString(s, "")
}

// parseOnlineNames extracts player names from `list` output, e.g.
// "There are 2 of a max of 20 players online: Steve, .BedrockGuy". It is
// hardened against color codes and doubled/echoed RCON responses (which
// previously leaked the header into the username).
func parseOnlineNames(out string) []string {
	out = stripColorCodes(out)
	m := onlineHeaderRe.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	return splitPlayerNames(m[1])
}

// splitPlayerNames cleans a comma-separated name tail: it defensively cuts at
// any secondary "there are " (a doubled header), then trims and drops empties.
func splitPlayerNames(rest string) []string {
	if i := strings.Index(strings.ToLower(rest), "there are "); i >= 0 {
		rest = rest[:i]
	}
	var names []string
	for _, p := range strings.Split(rest, ",") {
		if n := strings.TrimSpace(p); n != "" {
			names = append(names, n)
		}
	}
	return names
}

type whitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// GetWhitelist returns the server's whitelisted players. While running it asks
// the server over RCON; while stopped it reads whitelist.json.
func (c *Controller) GetWhitelist(ctx context.Context, serverID uuid.UUID) ([]Player, error) {
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	if srv.State == db.StateRunning {
		rc := rcon.New(rconAddrOf(srv), srv.RconPassword)
		out, err := rc.Run(ctx, "whitelist", "list")
		if err == nil {
			players := make([]Player, 0)
			for _, name := range parseWhitelistNames(out) {
				players = append(players, Player{Name: name, Edition: editionFor(name)})
			}
			return players, nil
		}
		// Fall through to reading the file on RCON failure.
	}

	entries, err := readWhitelistFile(srv.VolumeName)
	if err != nil {
		return []Player{}, nil
	}
	players := make([]Player, 0, len(entries))
	for _, e := range entries {
		players = append(players, Player{Name: e.Name, Edition: editionFor(e.Name), UUID: e.UUID})
	}
	return players, nil
}

// whitelistHeaderRe matches `whitelist list` output, e.g.
// "There are 2 whitelisted players: Steve, Alex".
var whitelistHeaderRe = regexp.MustCompile(`(?i)whitelisted players:?\s*(.*)`)

// parseWhitelistNames parses `whitelist list` output, hardened the same way as
// parseOnlineNames.
func parseWhitelistNames(out string) []string {
	out = stripColorCodes(out)
	m := whitelistHeaderRe.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	return splitPlayerNames(m[1])
}

// AddToWhitelist whitelists a player. Running servers use RCON; stopped servers
// edit whitelist.json (resolving the Mojang UUID for Java players).
func (c *Controller) AddToWhitelist(ctx context.Context, serverID uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperr.Validation("username is required")
	}
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}

	if srv.State == db.StateRunning {
		rc := rcon.New(rconAddrOf(srv), srv.RconPassword)
		if _, err := rc.Run(ctx, "whitelist", "add", name); err != nil {
			return apperr.Internal(fmt.Errorf("whitelist add via RCON failed: %w", err))
		}
		_, _ = rc.Run(ctx, "whitelist", "reload")
		return nil
	}

	// Stopped: edit whitelist.json directly.
	if editionFor(name) == "bedrock" {
		return apperr.BadState("start the server to whitelist Bedrock players")
	}
	id, err := resolveMojangUUID(ctx, name)
	if err != nil {
		return apperr.Validation(fmt.Sprintf("could not find Java player %q", name))
	}
	entries, _ := readWhitelistFile(srv.VolumeName)
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return nil // already whitelisted
		}
	}
	entries = append(entries, whitelistEntry{UUID: id, Name: name})
	return writeWhitelistFile(srv.VolumeName, entries)
}

// RemoveFromWhitelist removes a player from the whitelist.
func (c *Controller) RemoveFromWhitelist(ctx context.Context, serverID uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return apperr.Validation("username is required")
	}
	srv, err := c.store.GetServer(ctx, serverID)
	if err != nil {
		return err
	}

	if srv.State == db.StateRunning {
		rc := rcon.New(rconAddrOf(srv), srv.RconPassword)
		if _, err := rc.Run(ctx, "whitelist", "remove", name); err != nil {
			return apperr.Internal(fmt.Errorf("whitelist remove via RCON failed: %w", err))
		}
		_, _ = rc.Run(ctx, "whitelist", "reload")
		return nil
	}

	entries, err := readWhitelistFile(srv.VolumeName)
	if err != nil {
		return nil
	}
	kept := entries[:0]
	for _, e := range entries {
		if !strings.EqualFold(e.Name, name) {
			kept = append(kept, e)
		}
	}
	return writeWhitelistFile(srv.VolumeName, kept)
}

func whitelistPath(volume string) string {
	return filepath.Join(volume, "whitelist.json")
}

func readWhitelistFile(volume string) ([]whitelistEntry, error) {
	data, err := os.ReadFile(whitelistPath(volume))
	if err != nil {
		return nil, err
	}
	var entries []whitelistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func writeWhitelistFile(volume string, entries []whitelistEntry) error {
	if entries == nil {
		entries = []whitelistEntry{}
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return apperr.Internal(err)
	}
	if err := os.WriteFile(whitelistPath(volume), data, 0o644); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// resolveMojangUUID looks up the dashed UUID for a Java username.
func resolveMojangUUID(ctx context.Context, name string) (string, error) {
	url := "https://api.mojang.com/users/profiles/minecraft/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mojang lookup returned %d", resp.StatusCode)
	}
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if len(body.ID) != 32 {
		return "", fmt.Errorf("unexpected uuid %q", body.ID)
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", body.ID[0:8], body.ID[8:12], body.ID[12:16], body.ID[16:20], body.ID[20:32]), nil
}
