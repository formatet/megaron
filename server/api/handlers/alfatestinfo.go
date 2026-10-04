package handlers

import (
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
)

// AlfatestInfo renders the public Swedish invitation without querying game state.
// A standalone page preserves the language and behaviour of existing templates.
func (h *WebHandler) AlfatestInfo(w http.ResponseWriter, r *http.Request) {
	page, err := template.ParseFiles(filepath.Join(h.templateDir, "alfatestinfo.html"))
	if err != nil {
		slog.Error("alpha invitation template", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, nil); err != nil {
		slog.Error("alpha invitation render", "err", err)
	}
}
