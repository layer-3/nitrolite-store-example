package webui

import (
	"io/fs"
	"net/http"
	"strings"

	webassets "github.com/layer-3/nitrolite-go-example/web"
)

// New returns the embedded web surface handler.
func New() (http.Handler, error) {
	sub, err := fs.Sub(webassets.FS, ".")
	if err != nil {
		return nil, err
	}

	fileserver := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case trimmed == "":
			servePage(w, sub, "index.html")
			return
		case trimmed == "/reference":
			servePage(w, sub, "reference.html")
			return
		case trimmed == "/advanced":
			servePage(w, sub, "advanced.html")
			return
		case strings.HasPrefix(trimmed, "/pay/"):
			servePage(w, sub, "pay.html")
			return
		}
		fileserver.ServeHTTP(w, r)
	}), nil
}

func servePage(w http.ResponseWriter, sub fs.FS, name string) {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		http.Error(w, "missing page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
