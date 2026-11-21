package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/fonts/*.woff2
var fontFS embed.FS

// fontHandler serves the self-hosted subsets so the UI works offline.
func fontHandler() http.Handler {
	sub, err := fs.Sub(fontFS, "web/fonts")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/fonts/", http.FileServer(http.FS(sub)))
}
