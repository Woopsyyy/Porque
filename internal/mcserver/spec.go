// Package mcserver maps Porque server records onto itzg/minecraft-server
// containers and drives their lifecycle state machine.
package mcserver

import (
	"fmt"

	"github.com/woopsy/porque/internal/db"
)

// Internal container ports used by itzg/minecraft-server.
const (
	MCPort   = 25565 // game port
	RconPort = 25575 // RCON port
)

// heapMB sizes the JVM heap below the container memory limit, leaving headroom
// for JVM non-heap usage (metaspace, threads, GC, direct buffers). This is
// essential because Porque disables container swap (MemorySwap == Memory): a
// heap equal to the limit would OOM-kill the server.
func heapMB(memoryMB int) int {
	headroom := memoryMB / 4
	if headroom < 512 {
		headroom = 512
	}
	heap := memoryMB - headroom
	if heap < memoryMB/2 {
		heap = memoryMB / 2 // never give the JVM less than half the limit
	}
	if heap < 256 {
		heap = 256
	}
	return heap
}

func difficultyOr(d string) string {
	switch d {
	case "peaceful", "easy", "normal", "hard":
		return d
	default:
		return "normal"
	}
}

func motdOr(m string) string {
	if m == "" {
		return "A Minecraft Server"
	}
	return m
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// BuildEnv translates a server record into the environment variables the
// itzg/minecraft-server image consumes. itzg performs the actual jar download
// for the selected TYPE/VERSION, writes eula.txt, and runs RCON for us.
func BuildEnv(s *db.Server) []string {
	env := []string{
		"EULA=TRUE",
		"TYPE=" + string(s.ServerType),
		"VERSION=" + s.Version,
		fmt.Sprintf("MEMORY=%dM", heapMB(s.MemoryMB)),
		"ENABLE_RCON=true",
		"RCON_PASSWORD=" + s.RconPassword,
		fmt.Sprintf("RCON_PORT=%d", RconPort),
		fmt.Sprintf("SERVER_PORT=%d", MCPort),
		// itzg waits for SIGTERM and runs a graceful /stop, so `docker stop`
		// with a generous timeout gives us clean world saving without RCON.
		"STOP_DURATION=40",
		// Editable settings (itzg writes these into server.properties on start).
		"DIFFICULTY=" + difficultyOr(s.Difficulty),
		"ONLINE_MODE=" + boolStr(s.OnlineMode),
		"MOTD=" + motdOr(s.MOTD),
	}

	// Loader/installer version overrides per type.
	if s.LoaderVersion != nil && *s.LoaderVersion != "" {
		switch s.ServerType {
		case db.TypeFabric:
			env = append(env, "FABRIC_LOADER_VERSION="+*s.LoaderVersion)
		case db.TypeForge:
			env = append(env, "FORGE_VERSION="+*s.LoaderVersion)
		}
	}
	return env
}

// AutoCPUCores determines a default CPU core allocation based on memory size.
func AutoCPUCores(memoryMB int) float64 {
	cores := float64(memoryMB) / 2048.0
	if cores < 1.0 {
		return 1.0
	}
	if cores > 4.0 {
		return 4.0
	}
	return cores
}
