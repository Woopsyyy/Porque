package mcserver

import (
	"fmt"
	"net"

	"github.com/woopsy/porque/internal/db"
)

// Default ports used as the starting point for allocation and as fallbacks for
// servers created before per-server ports existed.
const (
	defaultGamePort = 25565
	defaultRconPort = 25575
)

// gamePortOf returns the server's allocated game port, falling back to the
// default for legacy rows that were never assigned one.
func gamePortOf(s *db.Server) int {
	if s.Port > 0 {
		return s.Port
	}
	return defaultGamePort
}

// rconPortOf returns the server's allocated RCON port, falling back to the
// default for legacy rows.
func rconPortOf(s *db.Server) int {
	if s.RconPort > 0 {
		return s.RconPort
	}
	return defaultRconPort
}

// rconAddrOf builds the loopback RCON dial address for a server.
func rconAddrOf(s *db.Server) string {
	return fmt.Sprintf("127.0.0.1:%d", rconPortOf(s))
}

// portFree reports whether a TCP port can be bound on loopback right now.
func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// AllocatePorts picks a free (game, rcon) port pair that does not collide with
// ports already reserved by other servers (used) nor with anything currently
// bound on the host. Game ports start at 25565 and RCON ports at 25575; the two
// ranges are advanced in lockstep so they stay easy to reason about.
func AllocatePorts(used map[int]bool) (gamePort, rconPort int, err error) {
	for g, r := defaultGamePort, defaultRconPort; g < 65000; g, r = g+1, r+1 {
		if used[g] || used[r] {
			continue
		}
		if !portFree(g) || !portFree(r) {
			continue
		}
		return g, r, nil
	}
	return 0, 0, fmt.Errorf("no free port pair available")
}
