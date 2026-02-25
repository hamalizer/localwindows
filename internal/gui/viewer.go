package gui

import (
	"fmt"
	"image"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"localwindows/internal/client"
	"localwindows/internal/discovery"
	"localwindows/internal/input"
	"localwindows/internal/protocol"
	"localwindows/internal/transfer"
)

// Fyne key name -> our input.Key mapping.
var fyneKeyMap = map[fyne.KeyName]input.Key{
	fyne.KeyA: input.KeyA, fyne.KeyB: input.KeyB, fyne.KeyC: input.KeyC,
	fyne.KeyD: input.KeyD, fyne.KeyE: input.KeyE, fyne.KeyF: input.KeyF,
	fyne.KeyG: input.KeyG, fyne.KeyH: input.KeyH, fyne.KeyI: input.KeyI,
	fyne.KeyJ: input.KeyJ, fyne.KeyK: input.KeyK, fyne.KeyL: input.KeyL,
	fyne.KeyM: input.KeyM, fyne.KeyN: input.KeyN, fyne.KeyO: input.KeyO,
	fyne.KeyP: input.KeyP, fyne.KeyQ: input.KeyQ, fyne.KeyR: input.KeyR,
	fyne.KeyS: input.KeyS, fyne.KeyT: input.KeyT, fyne.KeyU: input.KeyU,
	fyne.KeyV: input.KeyV, fyne.KeyW: input.KeyW, fyne.KeyX: input.KeyX,
	fyne.KeyY: input.KeyY, fyne.KeyZ: input.KeyZ,
	fyne.Key0: input.Key0, fyne.Key1: input.Key1, fyne.Key2: input.Key2,
	fyne.Key3: input.Key3, fyne.Key4: input.Key4, fyne.Key5: input.Key5,
	fyne.Key6: input.Key6, fyne.Key7: input.Key7, fyne.Key8: input.Key8,
	fyne.Key9: input.Key9,
	fyne.KeyF1: input.KeyF1, fyne.KeyF2: input.KeyF2, fyne.KeyF3: input.KeyF3,
	fyne.KeyF4: input.KeyF4, fyne.KeyF5: input.KeyF5, fyne.KeyF6: input.KeyF6,
	fyne.KeyF7: input.KeyF7, fyne.KeyF8: input.KeyF8, fyne.KeyF9: input.KeyF9,
	fyne.KeyF10: input.KeyF10, fyne.KeyF11: input.KeyF11, fyne.KeyF12: input.KeyF12,
	fyne.KeyReturn: input.KeyReturn, fyne.KeyTab: input.KeyTab,
	fyne.KeyBackspace: input.KeyBackspace, fyne.KeyDelete: input.KeyDelete,
	fyne.KeyEscape: input.KeyEscape, fyne.KeySpace: input.KeySpace,
	fyne.KeyLeft: input.KeyLeft, fyne.KeyRight: input.KeyRight,
	fyne.KeyUp: input.KeyUp, fyne.KeyDown: input.KeyDown,
	fyne.KeyHome: input.KeyHome, fyne.KeyEnd: input.KeyEnd,
	fyne.KeyPageUp: input.KeyPageUp, fyne.KeyPageDown: input.KeyPageDown,
	fyne.KeyInsert: input.KeyInsert,
}

// Preferences keys.
const (
	prefLastHost = "last_host"
	prefLastPort = "last_port"
	prefLastAuth = "last_auth"
)

func (a *App) showViewerScreen() {
	a.mainWindow.SetTitle("LocalWindows - Viewer Mode")
	a.mainWindow.Resize(fyne.NewSize(550, 520))

	prefs := a.fyneApp.Preferences()

	// -- Connection form --
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("Host IP address")
	if saved := prefs.String(prefLastHost); saved != "" {
		hostEntry.SetText(saved)
	}

	portEntry := widget.NewEntry()
	portEntry.SetText(prefs.StringWithFallback(prefLastPort, "19283"))

	credEntry := widget.NewPasswordEntry()
	credEntry.SetPlaceHolder("Password or PIN")

	authModeSelect := widget.NewSelect([]string{"Password", "PIN"}, nil)
	authModeSelect.SetSelected(prefs.StringWithFallback(prefLastAuth, "Password"))

	// -- Discovery list --
	discoveredHosts := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {},
	)

	var hostsMu sync.Mutex
	var hosts []discovery.Host

	refreshList := func() {
		discoveredHosts.Length = func() int {
			hostsMu.Lock()
			defer hostsMu.Unlock()
			return len(hosts)
		}
		discoveredHosts.UpdateItem = func(id widget.ListItemID, obj fyne.CanvasObject) {
			hostsMu.Lock()
			defer hostsMu.Unlock()
			if id < len(hosts) {
				h := hosts[id]
				authStr := "Password"
				if h.AuthMode == protocol.AuthPIN {
					authStr = "PIN"
				}
				obj.(*widget.Label).SetText(fmt.Sprintf("%s  (%s:%d) [%s]", h.Name, h.IP, h.Port, authStr))
			}
		}
		discoveredHosts.Refresh()
	}

	discoveredHosts.OnSelected = func(id widget.ListItemID) {
		hostsMu.Lock()
		if id < len(hosts) {
			h := hosts[id]
			hostEntry.SetText(h.IP)
			portEntry.SetText(strconv.Itoa(h.Port))
			if h.AuthMode == protocol.AuthPIN {
				authModeSelect.SetSelected("PIN")
			} else {
				authModeSelect.SetSelected("Password")
			}
		}
		hostsMu.Unlock()
	}

	// Start discovery listener.
	listener := discovery.NewListener(func(h discovery.Host) {
		hostsMu.Lock()
		// Deduplicate.
		for _, existing := range hosts {
			if existing.IP == h.IP && existing.Port == h.Port {
				hostsMu.Unlock()
				return
			}
		}
		hosts = append(hosts, h)
		hostsMu.Unlock()
		refreshList()
	})
	if err := listener.Start(); err != nil {
		log.Printf("discovery listener failed: %v", err)
	}

	statusLabel := widget.NewLabel("Status: Disconnected")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	var connectBtn *widget.Button
	var cl *client.Client

	connectBtn = widget.NewButton("Connect", func() {
		if cl != nil && cl.IsConnected() {
			cl.Disconnect()
			cl = nil
			statusLabel.SetText("Status: Disconnected")
			connectBtn.SetText("Connect")
			connectBtn.Importance = widget.HighImportance
			return
		}

		host := hostEntry.Text
		if host == "" {
			dialog.ShowError(fmt.Errorf("enter a host address"), a.mainWindow)
			return
		}
		port, err := strconv.Atoi(portEntry.Text)
		if err != nil || port < 1 || port > 65535 {
			dialog.ShowError(fmt.Errorf("invalid port"), a.mainWindow)
			return
		}
		if credEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("enter password or PIN"), a.mainWindow)
			return
		}

		var authMode protocol.AuthMode
		if authModeSelect.Selected == "PIN" {
			authMode = protocol.AuthPIN
		}

		// Save connection preferences.
		prefs.SetString(prefLastHost, host)
		prefs.SetString(prefLastPort, portEntry.Text)
		prefs.SetString(prefLastAuth, authModeSelect.Selected)

		statusLabel.SetText("Status: Connecting...")
		connectBtn.Disable()

		go func() {
			cl = client.New()
			_, err := cl.Connect(client.ConnectConfig{
				Host:       host,
				Port:       port,
				AuthMode:   authMode,
				Credential: credEntry.Text,
			})
			if err != nil {
				cl = nil
				statusLabel.SetText("Status: Disconnected")
				connectBtn.Enable()
				dialog.ShowError(fmt.Errorf("connection failed: %v", err), a.mainWindow)
				return
			}

			listener.Stop()
			connectBtn.Enable()
			a.showRemoteDesktop(cl)
		}()
	})
	connectBtn.Importance = widget.HighImportance

	backBtn := widget.NewButton("Back", func() {
		listener.Stop()
		if cl != nil && cl.IsConnected() {
			cl.Disconnect()
		}
		a.showModeSelector()
	})

	// -- Layout --
	discoveryLabel := widget.NewLabel("Discovered Hosts on LAN:")
	discoveryLabel.TextStyle = fyne.TextStyle{Bold: true}

	connForm := container.NewVBox(
		container.NewGridWithColumns(2,
			widget.NewLabel("Host:"), hostEntry,
			widget.NewLabel("Port:"), portEntry,
			widget.NewLabel("Auth:"), authModeSelect,
			widget.NewLabel("Credential:"), credEntry,
		),
	)

	content := container.NewVBox(
		connForm,
		widget.NewSeparator(),
		discoveryLabel,
		container.NewGridWrap(fyne.NewSize(500, 120), discoveredHosts),
		widget.NewSeparator(),
		statusLabel,
		layout.NewSpacer(),
		container.NewGridWithColumns(2, backBtn, connectBtn),
	)

	a.mainWindow.SetContent(container.NewPadded(content))
}

func (a *App) showRemoteDesktop(cl *client.Client) {
	rw, rh := cl.ScreenSize()
	a.mainWindow.SetTitle(fmt.Sprintf("LocalWindows - Remote Desktop (%dx%d)", rw, rh))

	// Frame counter for FPS display.
	var frameCount atomic.Int64
	fpsLabel := widget.NewLabel("FPS: --")
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			if !cl.IsConnected() {
				return
			}
			count := frameCount.Swap(0)
			fpsLabel.SetText(fmt.Sprintf("FPS: %d", count))
		}
	}()

	// Create the remote screen image.
	screenImg := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, rw, rh)))
	screenImg.FillMode = canvas.ImageFillContain
	screenImg.ScaleMode = canvas.ImageScaleFastest

	// Update the image on new frames.
	cl.OnFrameUpdate = func(frame *image.RGBA) {
		frameCount.Add(1)
		screenImg.Image = frame
		canvas.Refresh(screenImg)
	}

	// Handle clipboard from server.
	cl.OnClipboard = func(text string) {
		a.mainWindow.Clipboard().SetContent(text)
	}

	// Handle disconnect.
	cl.OnDisconnect = func(err error) {
		msg := "Disconnected"
		if err != nil {
			msg = fmt.Sprintf("Connection lost: %v", err)
		}
		dialog.ShowInformation("Disconnected", msg, a.mainWindow)
		a.mainWindow.SetFullScreen(false)
		a.showViewerScreen()
	}

	// File transfer manager.
	xferMgr := transfer.NewManager(".")
	xferMgr.SetCallbacks(
		func(id string, sent, total int64) {},
		func(id string, name string) {
			dialog.ShowInformation("Transfer Complete",
				fmt.Sprintf("Sent: %s", name), a.mainWindow)
		},
		func(id string, err error) {
			dialog.ShowError(fmt.Errorf("transfer error: %v", err), a.mainWindow)
		},
	)

	// -- Clipboard sync with cancellation --
	var clipStop chan struct{}
	var clipBtn *widget.Button
	clipBtn = widget.NewButton("Clip Sync: Off", func() {
		if clipStop != nil {
			// Stop existing goroutine.
			close(clipStop)
			clipStop = nil
			clipBtn.SetText("Clip Sync: Off")
		} else {
			// Start new goroutine.
			clipStop = make(chan struct{})
			clipBtn.SetText("Clip Sync: On")
			go syncClipboard(cl, a.mainWindow, clipStop)
		}
	})

	// -- Fullscreen toggle --
	isFullScreen := false
	var fullscreenBtn *widget.Button
	fullscreenBtn = widget.NewButton("Fullscreen", func() {
		isFullScreen = !isFullScreen
		a.mainWindow.SetFullScreen(isFullScreen)
		if isFullScreen {
			fullscreenBtn.SetText("Exit Fullscreen")
		} else {
			fullscreenBtn.SetText("Fullscreen")
		}
	})

	// -- Toolbar --
	ctrlAltDelBtn := widget.NewButton("Ctrl+Alt+Del", func() {
		cl.SendSpecialCombo(protocol.ComboCtrlAltDel)
	})

	altTabBtn := widget.NewButton("Alt+Tab", func() {
		cl.SendSpecialCombo(protocol.ComboAltTab)
	})

	winDBtn := widget.NewButton("Win+D", func() {
		cl.SendSpecialCombo(protocol.ComboWinD)
	})

	sendFileBtn := widget.NewButton("Send File", func() {
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			reader.Close()
			path := reader.URI().Path()
			_, ferr := xferMgr.SendFile(cl.Conn(), path)
			if ferr != nil {
				dialog.ShowError(ferr, a.mainWindow)
			}
		}, a.mainWindow)
		d.Show()
	})

	disconnectBtn := widget.NewButton("Disconnect", func() {
		a.mainWindow.SetFullScreen(false)
		cl.Disconnect()
		a.showViewerScreen()
	})
	disconnectBtn.Importance = widget.DangerImportance

	toolbar := container.NewHBox(
		ctrlAltDelBtn, altTabBtn, winDBtn,
		widget.NewSeparator(),
		clipBtn, fullscreenBtn,
		layout.NewSpacer(),
		fpsLabel,
		widget.NewSeparator(),
		sendFileBtn,
		disconnectBtn,
	)

	// -- Interactive remote screen --
	screen := newInteractiveScreen(screenImg, cl, rw, rh)

	content := container.NewBorder(
		toolbar, // top
		nil,     // bottom
		nil,     // left
		nil,     // right
		screen,
	)

	a.mainWindow.SetContent(content)
	a.mainWindow.Resize(fyne.NewSize(
		float32(min(rw, 1280)),
		float32(min(rh, 800))+50,
	))

	// Focus the interactive screen so it receives keyboard events.
	a.mainWindow.Canvas().Focus(screen)
}

// syncClipboard periodically sends the local clipboard to the remote host.
// It stops when the stop channel is closed or the client disconnects.
func syncClipboard(cl *client.Client, win fyne.Window, stop <-chan struct{}) {
	var lastClip string
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if !cl.IsConnected() {
				return
			}
			clip := win.Clipboard().Content()
			if clip != "" && clip != lastClip {
				lastClip = clip
				cl.SendClipboard(clip)
			}
		}
	}
}
