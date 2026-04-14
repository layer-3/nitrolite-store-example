package webui

import (
	"io/fs"
	"net/http"

	webassets "github.com/layer-3/nitrolite-go-example/web"
)

// New returns the embedded web console handler.
func New() (http.Handler, error) {
	sub, err := fs.Sub(webassets.FS, ".")
	if err != nil {
		return nil, err
	}

	fileserver := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "missing index", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
		fileserver.ServeHTTP(w, r)
	}), nil
}
