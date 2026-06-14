//go:build windows

package systray

import (
	"bytes"

	"github.com/getlantern/systray"
)

var (
	iconBytes []byte
	onShow    func()
	onQuit    func()
)

// Start initializes the system tray menu.
func Start(icon []byte, showCb func(), quitCb func()) {
	// If the provided icon is a PNG, wrap it in a standard ICO header for Windows compatibility
	if isPNG(icon) {
		iconBytes = pngToIco(icon)
	} else {
		iconBytes = icon
	}
	
	onShow = showCb
	onQuit = quitCb

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
	mQuit := systray.AddMenuItem("Quit", "Exit the application")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if onShow != nil {
					onShow()
				}
			case <-mQuit.ClickedCh:
				if onQuit != nil {
					onQuit()
				}
				systray.Quit()
				return
			}
		}
	}()
}

func isPNG(data []byte) bool {
	return len(data) > 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
}

// pngToIco wraps a raw PNG image inside a single-entry Windows ICO container in memory
func pngToIco(pngBytes []byte) []byte {
	size := len(pngBytes)
	ico := make([]byte, 22+size)

	// Icon Header
	ico[0] = 0 // Reserved
	ico[1] = 0
	ico[2] = 1 // Type (1 = Icon)
	ico[3] = 0
	ico[4] = 1 // Count (1 image)
	ico[5] = 0

	// Directory Entry
	ico[6] = 0   // Width (0 means 256px)
	ico[7] = 0   // Height (0 means 256px)
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
