//go:build !release

package cmd

import (
	"net/http"
	"os"
)

// GetFrontendFileSystem returns the filesystem for serving frontend files
func GetFrontendFileSystem() http.FileSystem {
	// In debug mode, serve from local filesystem
	if _, err := os.Stat("frontend/web/dist"); err == nil {
		return http.Dir("frontend/web/dist")
	}
	// Return nil if no filesystem available in debug mode
	return nil
}

// GetTemplateHTML returns the HTML template for standalone output
func GetTemplateHTML() ([]byte, error) {
	return os.ReadFile("frontend/res/template.html")
}
