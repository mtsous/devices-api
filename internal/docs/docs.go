package docs

import (
	_ "embed"
	"log/slog"
	"net/http"
)

var (
	//go:embed openapi.yml
	spec []byte

	//go:embed index.html
	page []byte
)

type Handler struct {
	logger *slog.Logger
}

func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yml", h.serve("application/yaml", spec))
	mux.HandleFunc("GET /docs", h.serve("text/html; charset=utf-8", page))
}

func (h *Handler) serve(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		if _, err := w.Write(body); err != nil {
			h.logger.ErrorContext(r.Context(), "write docs response", "path", r.URL.Path, "error", err)
		}
	}
}
