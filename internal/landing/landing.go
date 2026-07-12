package landing

import (
	"embed"
	"net/http"
)

//go:embed index.html
var files embed.FS

// Handler serves the public landing page.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := files.ReadFile("index.html")
	if err != nil {
		http.Error(w, "landing page unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(data)
}
