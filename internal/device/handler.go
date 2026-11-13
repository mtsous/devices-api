package device

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

var errTrailingData = errors.New("request body must contain a single JSON object")

type deviceService interface {
	Create(ctx context.Context, device Device) (*Device, error)
}

type Handler struct {
	service deviceService
	logger  *slog.Logger
}

func NewHandler(service deviceService, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /devices", h.create)
}

type createDeviceRequest struct {
	Name  string `json:"name"`
	Brand string `json:"brand"`
	State State  `json:"state"`
}

type deviceResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Brand     string     `json:"brand"`
	State     State      `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func toDeviceResponse(d *Device) deviceResponse {
	return deviceResponse{
		ID:        d.ID.String(),
		Name:      d.Name,
		Brand:     d.Brand,
		State:     d.State,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := h.service.Create(r.Context(), Device{
		Name:  req.Name,
		Brand: req.Brand,
		State: req.State,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Location", "/devices/"+created.ID.String())
	h.writeJSON(w, r, http.StatusCreated, toDeviceResponse(created))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// Reject trailing data such as a second JSON object.
	if decoder.More() {
		return errTrailingData
	}
	return nil
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrInvalidState):
		h.writeError(w, r, http.StatusBadRequest, err.Error())
	default:
		h.logger.ErrorContext(r.Context(), "device request failed", "error", err)
		h.writeError(w, r, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.writeJSON(w, r, status, errorResponse{Error: message})
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.ErrorContext(r.Context(), "write response", "error", err)
	}
}
