package api

import (
	"embed"
	"net/http"
)

//go:embed assets/updock-logo.png
var logoFS embed.FS

func (s *Server) handleLogo(w http.ResponseWriter, _ *http.Request) {
	data, err := logoFS.ReadFile("assets/updock-logo.png")
	if err != nil {
		http.Error(w, "logo not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
