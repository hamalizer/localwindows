package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const appID = "com.localwindows.app"

// App is the main GUI application.
type App struct {
	fyneApp    fyne.App
	mainWindow fyne.Window
}

// NewApp creates and returns the application.
func NewApp() *App {
	a := &App{
		fyneApp: app.NewWithID(appID),
	}
	a.fyneApp.Settings().SetTheme(theme.DarkTheme())
	return a
}

// Run shows the mode selection screen and starts the event loop.
func (a *App) Run() {
	a.mainWindow = a.fyneApp.NewWindow("LocalWindows - Remote Desktop")
	a.mainWindow.Resize(fyne.NewSize(500, 400))
	a.mainWindow.CenterOnScreen()
	a.showModeSelector()
	a.mainWindow.ShowAndRun()
}

func (a *App) showModeSelector() {
	title := widget.NewRichTextFromMarkdown("# LocalWindows\n\nLightweight LAN Remote Desktop")
	title.Wrapping = fyne.TextWrapWord

	hostBtn := widget.NewButton("Host (Share This Screen)", func() {
		a.showHostScreen()
	})
	hostBtn.Importance = widget.HighImportance

	viewerBtn := widget.NewButton("Viewer (Connect to Remote)", func() {
		a.showViewerScreen()
	})
	viewerBtn.Importance = widget.MediumImportance

	desc1 := widget.NewLabel("Allow others on your LAN to see and control this computer")
	desc1.Alignment = fyne.TextAlignCenter
	desc1.TextStyle = fyne.TextStyle{Italic: true}

	desc2 := widget.NewLabel("Connect to another computer on your LAN")
	desc2.Alignment = fyne.TextAlignCenter
	desc2.TextStyle = fyne.TextStyle{Italic: true}

	content := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(title),
		layout.NewSpacer(),
		container.NewVBox(
			hostBtn,
			desc1,
			widget.NewSeparator(),
			viewerBtn,
			desc2,
		),
		layout.NewSpacer(),
	)

	padded := container.NewPadded(content)
	a.mainWindow.SetContent(padded)
}
