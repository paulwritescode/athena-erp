package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

// Handler serves embedded Vite output and routes unknown paths to the SPA entry point.
func Handler() http.Handler {
	static, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend assets are unavailable", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(static))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		// Hashed asset files never change, but the HTML entry point names the current hashes.
		// Without revalidation a browser keeps showing an older build after a rebuild.
		if !strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if _, err := fs.Stat(static, name); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, static, "index.html")
	})
}
