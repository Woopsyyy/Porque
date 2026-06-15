//go:build windows

package systray

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/energye/systray"
	"github.com/woopsy/porque/internal/db"
)

type ServerManager interface {
	ListServers() ([]db.Server, error)
	StartServer(id string) (*db.Server, error)
	StopServer(id string) (map[string]string, error)
	RestartServer(id string) (map[string]string, error)
}

var (
	iconBytes []byte
	onShow    func()
	onQuit    func()
	manager   ServerManager

	mActive     *systray.MenuItem
	mStart      *systray.MenuItem
	mStop       *systray.MenuItem
	mRestart    *systray.MenuItem
	mChange     *systray.MenuItem
	changeSlots []*systray.MenuItem

	mu             sync.Mutex
	activeServerID string
	slotServerIDs  [10]string
)

// Start initializes the system tray menu.
func Start(icon []byte, showCb func(), quitCb func(), mgr ServerManager) {
	// If the provided icon is a PNG, wrap it in a standard ICO header for Windows compatibility
	if isPNG(icon) {
		iconBytes = pngToIco(icon)
	} else {
		iconBytes = icon
	}
	
	onShow = showCb
	onQuit = quitCb
	manager = mgr

	go func() {
		systray.Run(onReady, nil)
	}()
}

func onReady() {
	systray.SetIcon(iconBytes)
	systray.SetTitle("Porque")
	systray.SetTooltip("Porque Minecraft Server Manager")

	mShow := systray.AddMenuItem("Show Dashboard", "Restore the window")
	systray.AddSeparator()

	// Active server label
	mActive = systray.AddMenuItem("Active Server: None", "")
	mActive.Disable()

	// Control actions
	mStart = systray.AddMenuItem("Start Server", "Start the active server")
	mStop = systray.AddMenuItem("Stop Server", "Stop the active server")
	mRestart = systray.AddMenuItem("Restart Server", "Restart the active server")

	// Change Server submenu
	mChange = systray.AddMenuItem("Change Server", "Select the active server to manage")
	changeSlots = make([]*systray.MenuItem, 10)
	for i := 0; i < 10; i++ {
		slotItem := mChange.AddSubMenuItem(fmt.Sprintf("Slot %d", i), "")
		changeSlots[i] = slotItem
		slotItem.Hide()

		// Wire up the click callback for the slot
		idx := i
		slotItem.Click(func() {
			mu.Lock()
			targetID := slotServerIDs[idx]
			if targetID != "" {
				activeServerID = targetID
			}
			mu.Unlock()
			updateTrayMenu()
		})
	}

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit the application")

	// Set click events
	mStart.Click(func() {
		mu.Lock()
		id := activeServerID
		mu.Unlock()
		if id != "" && manager != nil {
			go func() {
				_, _ = manager.StartServer(id)
				updateTrayMenu()
			}()
		}
	})

	mStop.Click(func() {
		mu.Lock()
		id := activeServerID
		mu.Unlock()
		if id != "" && manager != nil {
			go func() {
				_, _ = manager.StopServer(id)
				updateTrayMenu()
			}()
		}
	})

	mRestart.Click(func() {
		mu.Lock()
		id := activeServerID
		mu.Unlock()
		if id != "" && manager != nil {
			go func() {
				_, _ = manager.RestartServer(id)
				updateTrayMenu()
			}()
		}
	})

	mShow.Click(func() {
		if onShow != nil {
			onShow()
		}
	})

	mQuit.Click(func() {
		if onQuit != nil {
			onQuit()
		}
		systray.Quit()
	})

	// Set left-click action to directly show the window
	systray.SetOnClick(func(menu systray.IMenu) {
		if onShow != nil {
			onShow()
		}
	})

	// Set right-click action to display the context menu
	systray.SetOnRClick(func(menu systray.IMenu) {
		menu.ShowMenu()
	})

	// Start a background loop to update the menu state every 2 seconds
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			updateTrayMenu()
		}
	}()
}

func updateTrayMenu() {
	if manager == nil {
		return
	}

	servers, err := manager.ListServers()
	if err != nil || len(servers) == 0 {
		mActive.SetTitle("Active Server: None")
		mActive.SetTooltip("")
		mStart.Disable()
		mStop.Disable()
		mRestart.Disable()
		mChange.Hide()
		return
	}

	mChange.Show()

	mu.Lock()
	defer mu.Unlock()

	// 1. Resolve activeServerID if empty or invalid
	found := false
	var activeSrv *db.Server
	for i := range servers {
		if servers[i].ID.String() == activeServerID {
			found = true
			activeSrv = &servers[i]
			break
		}
	}

	// If not found, default to first running/starting server, or first server
	if !found {
		for i := range servers {
			if servers[i].State == db.StateRunning || servers[i].State == db.StateStarting {
				activeServerID = servers[i].ID.String()
				activeSrv = &servers[i]
				found = true
				break
			}
		}
		if !found {
			activeServerID = servers[0].ID.String()
			activeSrv = &servers[0]
			found = true
		}
	}

	// 2. Update Active Server Title & Tooltip
	mActive.SetTitle(fmt.Sprintf("Active: %s (%s)", activeSrv.Name, strings.ToUpper(string(activeSrv.State))))
	mActive.SetTooltip(fmt.Sprintf("Type: %s | Version: %s", activeSrv.ServerType, activeSrv.Version))

	// 3. Update Start/Stop/Restart Actions status
	switch activeSrv.State {
	case db.StateStopped, db.StateCrashed, db.StateCorrupted:
		mStart.Enable()
		mStop.Disable()
		mRestart.Disable()
	case db.StateRunning:
		mStart.Disable()
		mStop.Enable()
		mRestart.Enable()
	case db.StateStarting:
		mStart.Disable()
		mStop.Enable()
		mRestart.Disable()
	case db.StateStopping:
		mStart.Disable()
		mStop.Disable()
		mRestart.Disable()
	default:
		mStart.Disable()
		mStop.Disable()
		mRestart.Disable()
	}

	// 4. Update Change Server Submenu Slots
	for i := 0; i < 10; i++ {
		if i < len(servers) {
			srv := servers[i]
			slotServerIDs[i] = srv.ID.String()

			prefix := "[ ]"
			if srv.ID.String() == activeServerID {
				prefix = "[x]"
			}

			// Show name and state in submenu slot
			changeSlots[i].SetTitle(fmt.Sprintf("%s %s (%s)", prefix, srv.Name, srv.State))
			changeSlots[i].Show()
		} else {
			slotServerIDs[i] = ""
			changeSlots[i].Hide()
		}
	}
}

func isPNG(data []byte) bool {
	return len(data) > 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
}

// pngToIco wraps a raw PNG image inside a single-entry Windows ICO container in memory
func pngToIco(pngBytes []byte) []byte {
	size := len(pngBytes)
	
	// Default to 256x256 (0, 0 in ICO) if we can't parse the PNG dimensions
	var width, height byte = 0, 0
	if len(pngBytes) >= 24 {
		// PNG IHDR width is at bytes 16-19, height at 20-23 (big-endian)
		w := int(pngBytes[16])<<24 | int(pngBytes[17])<<16 | int(pngBytes[18])<<8 | int(pngBytes[19])
		h := int(pngBytes[20])<<24 | int(pngBytes[21])<<16 | int(pngBytes[22])<<8 | int(pngBytes[23])
		if w < 256 {
			width = byte(w)
		}
		if h < 256 {
			height = byte(h)
		}
	}

	ico := make([]byte, 22+size)

	// Icon Header
	ico[0] = 0 // Reserved
	ico[1] = 0
	ico[2] = 1 // Type (1 = Icon)
	ico[3] = 0
	ico[4] = 1 // Count (1 image)
	ico[5] = 0

	// Directory Entry
	ico[6] = width   // Width (0 means 256px)
	ico[7] = height  // Height (0 means 256px)
	ico[8] = 0   // Color count (0 = no palette)
	ico[9] = 0   // Reserved
	ico[10] = 1  // Planes
	ico[11] = 0
	ico[12] = 32 // Bit count (32 bits)
	ico[13] = 0

	// Bytes size (4 bytes, little-endian)
	ico[14] = byte(size)
	ico[15] = byte(size >> 8)
	ico[16] = byte(size >> 16)
	ico[17] = byte(size >> 24)

	// Image offset (4 bytes, little-endian) - always 22
	ico[18] = 22
	ico[19] = 0
	ico[20] = 0
	ico[21] = 0

	// Image Data
	copy(ico[22:], pngBytes)

	return ico
}
