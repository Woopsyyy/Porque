package mcserver

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/woopsy/porque/internal/db"
)

func writeEula(dir string) error {
	path := filepath.Join(dir, "eula.txt")
	return os.WriteFile(path, []byte("eula=true\n"), 0o644)
}

func writeServerProperties(dir string, srv *db.Server) error {
	path := filepath.Join(dir, "server.properties")
	
	// Read existing properties if they exist, to preserve other keys
	props := make(map[string]string)
	if _, err := os.Stat(path); err == nil {
		file, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
			file.Close()
		}
	}

	// Override critical keys managed by Porque
	props["enable-rcon"] = "true"
	props["rcon.port"] = "25575"
	props["rcon.password"] = srv.RconPassword
	props["server-port"] = "25565"
	if srv.OnlineMode {
		props["online-mode"] = "true"
	} else {
		props["online-mode"] = "false"
	}
	props["difficulty"] = srv.Difficulty
	props["motd"] = srv.MOTD
	props["broadcast-rcon-to-ops"] = "false"

	// Write properties back
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("# Minecraft server properties managed by Porque\n")
	for k, v := range props {
		if _, err := writer.WriteString(fmt.Sprintf("%s=%s\n", k, v)); err != nil {
			return err
		}
	}
	return writer.Flush()
}
