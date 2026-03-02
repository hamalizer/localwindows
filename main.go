package main

import (
	"localwindows/internal/gui"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	app := gui.NewApp()
	app.Run()
}
