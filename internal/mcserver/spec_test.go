package mcserver

import (
	"strings"
	"testing"

	"github.com/woopsy/porque/internal/db"
)

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			m[k] = v
		}
	}
	return m
}

func TestBuildEnv_PaperBasics(t *testing.T) {
	s := &db.Server{ServerType: db.TypePaper, Version: "1.20.4", MemoryMB: 1024, RconPassword: "secret"}
	m := envMap(BuildEnv(s))

	checks := map[string]string{
		"EULA":          "TRUE",
		"TYPE":          "PAPER",
		"VERSION":       "1.20.4",
		"MEMORY":        "512M", // heap sized below the 1024M container limit
		"ENABLE_RCON":   "true",
		"RCON_PASSWORD": "secret",
		"RCON_PORT":     "25575",
		"SERVER_PORT":   "25565",
	}
	for k, want := range checks {
		if got := m[k]; got != want {
			t.Errorf("env %s = %q, want %q", k, got, want)
		}
	}
}

func TestHeapMB_LeavesHeadroom(t *testing.T) {
	cases := []struct{ limit, wantHeap int }{
		{512, 256},
		{1024, 512},
		{2048, 1536},
		{4096, 3072},
		{8192, 6144},
	}
	for _, c := range cases {
		if got := heapMB(c.limit); got != c.wantHeap {
			t.Errorf("heapMB(%d) = %d, want %d", c.limit, got, c.wantHeap)
		}
		if heapMB(c.limit) >= c.limit {
			t.Errorf("heapMB(%d) must be below the container limit", c.limit)
		}
	}
}

func TestBuildEnv_FabricLoaderVersion(t *testing.T) {
	v := "0.15.0"
	s := &db.Server{ServerType: db.TypeFabric, Version: "1.20.4", MemoryMB: 2048, LoaderVersion: &v}
	m := envMap(BuildEnv(s))
	if m["FABRIC_LOADER_VERSION"] != "0.15.0" {
		t.Errorf("FABRIC_LOADER_VERSION = %q, want 0.15.0", m["FABRIC_LOADER_VERSION"])
	}
	if _, ok := m["FORGE_VERSION"]; ok {
		t.Errorf("FORGE_VERSION should not be set for a Fabric server")
	}
}

func TestBuildEnv_ForgeLoaderVersion(t *testing.T) {
	v := "47.2.0"
	s := &db.Server{ServerType: db.TypeForge, Version: "1.20.1", MemoryMB: 4096, LoaderVersion: &v}
	m := envMap(BuildEnv(s))
	if m["FORGE_VERSION"] != "47.2.0" {
		t.Errorf("FORGE_VERSION = %q, want 47.2.0", m["FORGE_VERSION"])
	}
}

func TestBuildEnv_VanillaNoLoader(t *testing.T) {
	s := &db.Server{ServerType: db.TypeVanilla, Version: "1.20.4", MemoryMB: 1024}
	m := envMap(BuildEnv(s))
	for _, k := range []string{"FABRIC_LOADER_VERSION", "FORGE_VERSION"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s should not be set for a Vanilla server", k)
		}
	}
}
