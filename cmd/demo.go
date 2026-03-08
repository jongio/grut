package cmd

import "github.com/jongio/grut/internal/demo"

// setupDemoProject delegates to the shared demo package so both the main
// binary and the screenshot binary can create identical demo projects.
func setupDemoProject() (string, func(), error) {
	return demo.SetupProject()
}
