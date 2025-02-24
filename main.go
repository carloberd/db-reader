package main

import (
	"log"

	"fyne.io/fyne/v2/app"

	"github.com/carloberd/db-reader/ui"
)

func main() {
	// Create and initialize the application
	a := app.New()
	inspector := ui.NewDBInspector(a)

	// Show the UI
	err := inspector.Show()
	if err != nil {
		log.Fatalf("Error launching application: %v", err)
	}
}
