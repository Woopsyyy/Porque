//go:build windows

package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// Set configures the auto-start behavior in the Windows Registry.
func Set(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
		// Wrap path in quotes to handle potential spaces in the directory structure
		val := fmt.Sprintf(`"%s" --minimized`, exePath)
		err = k.SetStringValue("Porque", val)
		if err != nil {
			return fmt.Errorf("set registry value: %w", err)
		}
	} else {
		// Ignore error in case the value doesn't exist
		_ = k.DeleteValue("Porque")
	}
	return nil
}
