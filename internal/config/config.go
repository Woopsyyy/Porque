// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the API server and worker.
type Config struct {
	DatabaseURL      string // postgres DSN
	JWTSecret        string // signing secret for admin JWTs
	APIPort          string // HTTP listen port for the API server
	DockerHost       string // optional override for the Docker endpoint (DOCKER_HOST)
	DataRoot         string // optional host bind-mount root; empty => use named volumes
	MCDefaultImage   string // default Minecraft server image
	AdminUsername    string // optional: seed an admin user on first boot
	AdminPassword    string // optional: seed admin password
	BackupRoot       string // directory where backup archives are written
	BackupKeep       int    // number of backups to retain per server (rotation)
	HelperImage      string // lightweight image used for volume restore/extract
	PlayitAgentImage string // playit.gg agent image for tunnel sidecars

	// Worker tuning.
	MetricsInterval time.Duration // heartbeat + metrics sampling period
	StartupGrace    time.Duration // grace before heartbeat failures count
	HeartbeatMisses int           // consecutive unhealthy checks before recovery
	MaxRestarts     int           // max auto-restarts per window before 'corrupted'
	RestartWindow   time.Duration // sliding window for restart rate limiting
	RestartBackoff  time.Duration // base backoff before an auto-restart
}

// Load reads configuration from the environment, applying defaults where safe.
// JWT_SECRET is required and has no default.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:      getenv("DATABASE_URL", "postgres://porque:porque@localhost:5432/porque?sslmode=disable"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		APIPort:          getenv("API_PORT", "8080"),
		DockerHost:       os.Getenv("DOCKER_HOST"),
		DataRoot:         os.Getenv("PORQUE_DATA_ROOT"),
		MCDefaultImage:   getenv("MC_DEFAULT_IMAGE", "itzg/minecraft-server"),
		AdminUsername:    os.Getenv("ADMIN_USERNAME"),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
		BackupRoot:       getenv("BACKUP_ROOT", "/backups"),
		BackupKeep:       getenvInt("BACKUP_KEEP", 5),
		HelperImage:      getenv("HELPER_IMAGE", "alpine:3.20"),
		PlayitAgentImage: getenv("PLAYIT_AGENT_IMAGE", "ghcr.io/playit-cloud/playit-agent:latest"),

		MetricsInterval: getenvDuration("METRICS_INTERVAL", 10*time.Second),
		StartupGrace:    getenvDuration("STARTUP_GRACE", 120*time.Second),
		HeartbeatMisses: getenvInt("HEARTBEAT_MISSES", 3),
		MaxRestarts:     getenvInt("MAX_RESTARTS", 5),
		RestartWindow:   getenvDuration("RESTART_WINDOW", 10*time.Minute),
		RestartBackoff:  getenvDuration("RESTART_BACKOFF", 5*time.Second),
	}
	if c.JWTSecret == "" {
		return nil, fmt.Errorf("config: JWT_SECRET is required")
	}
	return c, nil
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
