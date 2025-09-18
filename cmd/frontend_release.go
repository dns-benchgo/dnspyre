//go:build release

package cmd

import (
	"embed"
	"io/fs"
	"net/http"
)

// Embed the built frontend files
//
//go:embed ../frontend/web/dist
var frontendFS embed.FS

// Embed the template file
//
//go:embed ../frontend/res/template.html
var templateHTML []byte

// GetFrontendFileSystem returns the embedded frontend filesystem
func GetFrontendFileSystem() http.FileSystem {
	// Get subdirectory from embedded filesystem
	sub, err := fs.Sub(frontendFS, "frontend/web/dist")
	if err != nil {
		return nil
	}
	return http.FS(sub)
}

// GetTemplateHTML returns the embedded HTML template
func GetTemplateHTML() ([]byte, error) {
	return templateHTML, nil
}
