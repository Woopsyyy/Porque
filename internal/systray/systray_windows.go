//go:build windows

package systray

import (
	"github.com/getlantern/systray"
)

var (
	iconBytes []byte
	onShow    func()
	onQuit    func()
)

// Start initializes the system tray menu.
func Start(icon []byte, showCb func(), quitCb func()) {
	iconBytes = icon
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
