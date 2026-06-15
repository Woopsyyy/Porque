//go:build !windows

package systray

import "github.com/woopsy/porque/internal/db"

type ServerManager interface {
	ListServers() ([]db.Server, error)
	StartServer(id string) (*db.Server, error)
	StopServer(id string) (map[string]string, error)
	RestartServer(id string) (map[string]string, error)
}

// Start is a stub implementation for non-Windows platforms.
func Start(icon []byte, showCb func(), quitCb func(), mgr ServerManager) {
}
