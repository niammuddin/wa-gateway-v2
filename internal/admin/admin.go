package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed ui/index.html ui/assets/*
var ui embed.FS

func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	if path == "" || path == "/" {
		path = "ui/index.html"
	} else if strings.HasPrefix(path, "/assets/") {
		path = "ui" + path
	} else {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(ui, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(path, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
