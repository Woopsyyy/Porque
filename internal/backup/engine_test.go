package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAndValidateArchive(t *testing.T) {
	// Create a temp server directory structure
	srcDir := t.TempDir()
	
	// Write dummy files
	os.MkdirAll(filepath.Join(srcDir, "world"), 0755)
	os.WriteFile(filepath.Join(srcDir, "server.properties"), []byte("motd=hi"), 0644)
	os.WriteFile(filepath.Join(srcDir, "world", "level.dat"), []byte("x"), 0644)

	// Exclude backups dir (must not be in backup)
	os.MkdirAll(filepath.Join(srcDir, "backups"), 0755)
	os.WriteFile(filepath.Join(srcDir, "backups", "old.tar.gz"), []byte("skip me"), 0644)

	outArchive := filepath.Join(t.TempDir(), "backup.tar.gz")
	
	// Create the archive
	sha, err := createArchiveFromDir(srcDir, outArchive)
	if err != nil {
		t.Fatalf("createArchiveFromDir: %v", err)
	}
	if len(sha) != 64 {
		t.Errorf("sha length = %d, want 64", len(sha))
	}

	// Validate the archive
	if err := validateArchive(outArchive); err != nil {
		t.Errorf("validateArchive on good archive: %v", err)
	}

	// Verify the hash is consistent
	got, _ := sha256File(outArchive)
	if got != sha {
		t.Errorf("sha256File = %s, want %s", got, sha)
	}

	// Try extracting to a clean directory
	destDir := t.TempDir()
	if err := extractArchiveToDir(outArchive, destDir); err != nil {
		t.Fatalf("extractArchiveToDir: %v", err)
	}

	// Check recovered files
	propData, err := os.ReadFile(filepath.Join(destDir, "server.properties"))
	if err != nil || string(propData) != "motd=hi" {
		t.Errorf("server.properties recover mismatch: got %q, err %v", string(propData), err)
	}

	levelData, err := os.ReadFile(filepath.Join(destDir, "world", "level.dat"))
	if err != nil || string(levelData) != "x" {
		t.Errorf("world/level.dat recover mismatch: got %q, err %v", string(levelData), err)
	}

	// Ensure the backups directory was skipped
	if _, err := os.Stat(filepath.Join(destDir, "backups")); !os.IsNotExist(err) {
		t.Error("expected backups/ directory to be skipped, but it exists")
	}
}

func TestValidateArchive_CorruptGzip(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "corrupt.tar.gz")
	if err := os.WriteFile(bad, []byte("this is definitely not gzip"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(bad); err == nil {
		t.Fatal("expected error for corrupt gzip, got nil")
	}
}

func TestValidateArchive_TruncatedTar(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "heavy.dat"), []byte(strings.Repeat("A", 8192)), 0644)
	
	outArchive := filepath.Join(t.TempDir(), "trunc.tar.gz")
	if _, err := createArchiveFromDir(srcDir, outArchive); err != nil {
		t.Fatal(err)
	}

	// Truncate the file mid-stream
	data, _ := os.ReadFile(outArchive)
	if err := os.WriteFile(outArchive, data[:len(data)/2], 0644); err != nil {
		t.Fatal(err)
	}

	if err := validateArchive(outArchive); err == nil {
		t.Fatal("expected error for truncated archive, got nil")
	}
}

func TestValidateArchive_Empty(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.tar.gz")
	if err := os.WriteFile(empty, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchive(empty); err == nil {
		t.Fatal("expected error for empty archive, got nil")
	}
}
