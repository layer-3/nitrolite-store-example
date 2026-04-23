package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// New returns the embedded frontend handler.
func New() (http.Handler, error) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := path.Clean(r.URL.Path)
		if cleanPath == "." || cleanPath == "/" {
			serveIndex(w, sub)
			return
		}
		if cleanPath == "/reference" || cleanPath == "/advanced" {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(cleanPath, "/api/") {
			http.NotFound(w, r)
			return
		}

		trimmed := strings.TrimPrefix(cleanPath, "/")
		if _, err := fs.Stat(sub, trimmed); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		serveIndex(w, sub)
	}), nil
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "missing frontend build", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
