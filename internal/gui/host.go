package gui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"localwindows/internal/auth"
	"localwindows/internal/discovery"
	"localwindows/internal/protocol"
	"localwindows/internal/server"
)

func (a *App) showHostScreen() {
	a.mainWindow.SetTitle("LocalWindows - Host Mode")
	a.mainWindow.Resize(fyne.NewSize(550, 500))

	cfg := server.DefaultConfig()
	authMgr := auth.NewManager(protocol.AuthPassword)

	// -- Auth mode selector --
	authModeLabel := widget.NewLabel("Authentication Mode:")
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Enter password")
	passwordEntry.SetText("localwindows")

	pinDisplay := widget.NewLabel("")
	pinDisplay.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	pinDisplay.Hide()

	generatePINBtn := widget.NewButton("Generate New PIN", nil)
	generatePINBtn.Hide()

	passwordRow := container.NewVBox(passwordEntry)
	pinRow := container.NewVBox(pinDisplay, generatePINBtn)

	authModeSelect := widget.NewSelect([]string{"Password", "PIN"}, func(val string) {
		switch val {
		case "Password":
			authMgr.SetMode(protocol.AuthPassword)
			passwordRow.Show()
			pinRow.Hide()
		case "PIN":
			authMgr.SetMode(protocol.AuthPIN)
			passwordRow.Hide()
			pinRow.Show()
			pin, err := authMgr.GeneratePIN()
			if err != nil {
				dialog.ShowError(err, a.mainWindow)
				return
			}
			pinDisplay.SetText(fmt.Sprintf("PIN: %s", pin))
		}
	})
	authModeSelect.SetSelected("Password")

	generatePINBtn.OnTapped = func() {
		pin, err := authMgr.GeneratePIN()
		if err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}
		pinDisplay.SetText(fmt.Sprintf("PIN: %s", pin))
	}

	// -- Port --
	portEntry := widget.NewEntry()
	portEntry.SetText(strconv.Itoa(cfg.Port))

	var srv *server.Server
	var startBtn *widget.Button

	// -- Quality / FPS --
	qualitySlider := widget.NewSlider(10, 100)
	qualitySlider.SetValue(float64(cfg.Quality))
	qualityLabel := widget.NewLabel(fmt.Sprintf("Quality: %d%%", cfg.Quality))
	qualitySlider.OnChanged = func(v float64) {
		qualityLabel.SetText(fmt.Sprintf("Quality: %d%%", int(v)))
		if srv != nil && srv.IsRunning() {
			srv.SetQuality(int(v))
		}
	}

	fpsSlider := widget.NewSlider(1, 60)
	fpsSlider.SetValue(float64(cfg.MaxFPS))
	fpsLabel := widget.NewLabel(fmt.Sprintf("Max FPS: %d", cfg.MaxFPS))
	fpsSlider.OnChanged = func(v float64) {
		fpsLabel.SetText(fmt.Sprintf("Max FPS: %d", int(v)))
		if srv != nil && srv.IsRunning() {
			srv.SetMaxFPS(int(v))
		}
	}

	// -- Receive directory --
	recvDirLabel := widget.NewLabel(fmt.Sprintf("Receive dir: %s", cfg.ReceiveDir))
	recvDirLabel.Wrapping = fyne.TextWrapWord
	recvDirLabel.TextStyle = fyne.TextStyle{Italic: true}

	// -- Status area --
	statusLabel := widget.NewLabel("Status: Stopped")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	ipLabel := widget.NewLabel(fmt.Sprintf("Local IP: %s", discovery.GetLocalIP()))
	clientsLabel := widget.NewLabel("Connected clients: 0")

	startBtn = widget.NewButton("Start Sharing", func() {
		if srv != nil && srv.IsRunning() {
			// Stop
			srv.Stop()
			srv = nil
			a.activeServer = nil
			statusLabel.SetText("Status: Stopped")
			clientsLabel.SetText("Connected clients: 0")
			startBtn.SetText("Start Sharing")
			startBtn.Importance = widget.HighImportance
			return
		}

		// Read config from UI.
		port, err := strconv.Atoi(portEntry.Text)
		if err != nil || port < 1 || port > 65535 {
			dialog.ShowError(fmt.Errorf("invalid port number"), a.mainWindow)
			return
		}

		if authMgr.Mode() == protocol.AuthPassword {
			if passwordEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("password cannot be empty"), a.mainWindow)
				return
			}
			authMgr.SetPassword(passwordEntry.Text)
		}

		cfg.Port = port
		cfg.Quality = int(qualitySlider.Value)
		cfg.MaxFPS = int(fpsSlider.Value)

		srv = server.New(authMgr, cfg)
		a.activeServer = srv
		srv.OnClientConnect = func(addr string) {
			clientsLabel.SetText(fmt.Sprintf("Connected clients: %d", srv.ClientCount()))
		}
		srv.OnClientDisconnect = func(addr string) {
			clientsLabel.SetText(fmt.Sprintf("Connected clients: %d", srv.ClientCount()))
		}
		srv.OnError = func(err error) {
			// Log but don't crash.
		}
		srv.OnFileReceived = func(name string, size int64) {
			dialog.ShowInformation("File Received",
				fmt.Sprintf("Received: %s (%s)", name, formatSize(size)),
				a.mainWindow)
		}
		srv.OnClipboardReceived = func(text string) {
			a.mainWindow.Clipboard().SetContent(text)
		}

		if err := srv.Start(); err != nil {
			dialog.ShowError(fmt.Errorf("failed to start: %v", err), a.mainWindow)
			return
		}

		statusLabel.SetText(fmt.Sprintf("Status: Sharing on port %d", port))
		startBtn.SetText("Stop Sharing")
		startBtn.Importance = widget.DangerImportance
	})
	startBtn.Importance = widget.HighImportance

	// -- Back button --
	backBtn := widget.NewButton("Back", func() {
		if srv != nil && srv.IsRunning() {
			dialog.ShowConfirm("Stop Sharing?",
				"Stopping will disconnect all viewers.", func(ok bool) {
					if ok {
						srv.Stop()
						a.activeServer = nil
						a.showModeSelector()
					}
				}, a.mainWindow)
			return
		}
		a.showModeSelector()
	})

	// -- Layout --
	settingsForm := container.NewVBox(
		authModeLabel,
		authModeSelect,
		passwordRow,
		pinRow,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Port:"), portEntry,
		),
		widget.NewSeparator(),
		qualityLabel, qualitySlider,
		fpsLabel, fpsSlider,
	)

	statusBox := container.NewVBox(
		widget.NewSeparator(),
		statusLabel,
		ipLabel,
		clientsLabel,
		recvDirLabel,
	)

	content := container.NewVBox(
		settingsForm,
		statusBox,
		layout.NewSpacer(),
		container.NewGridWithColumns(2, backBtn, startBtn),
	)

	a.mainWindow.SetContent(container.NewPadded(content))
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
