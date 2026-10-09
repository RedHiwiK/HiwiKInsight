// Package dashboard serves the static dashboard frontend (/dashboard/*).
//
// The static files themselves are served without authentication; all data is loaded
// from the query API, which enforces its own access control.
package dashboard

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

type Handler struct {
	static fs.FS // root of the build output (contains index.html); nil means the frontend is not built
}

func New(dist fs.FS) *Handler {
	h := &Handler{}
	if sub, err := fs.Sub(dist, "dist/app"); err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			h.static = sub
		}
	}
	return h
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /dashboard/{path...}", h.serveStatic)
}

// serveStatic caches hashed assets aggressively; every other path falls back to index.html (client-side routing).
func (h *Handler) serveStatic(w http.ResponseWriter, r *http.Request) {
	if h.static == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Dashboard not built: run `make web` (or cd web && pnpm install && pnpm build), then rebuild the server"))
		return
	}
	p := path.Clean(r.PathValue("path"))
	if strings.HasPrefix(p, "assets/") {
		if _, err := fs.Stat(h.static, p); err == nil {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeFileFS(w, r, h.static, p)
			return
		}
		http.NotFound(w, r)
		return
	}
	if p != "." && p != "index.html" {
		if st, err := fs.Stat(h.static, p); err == nil && !st.IsDir() { // root-level files such as favicon
			http.ServeFileFS(w, r, h.static, p)
			return
		}
	}
	b, err := fs.ReadFile(h.static, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Write(b)
}
