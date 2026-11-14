package notify

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mtsous/devices-api/internal/device"
)

const keepAliveInterval = 15 * time.Second

// Handler streams state changes to clients as Server-Sent Events.
type Handler struct {
	broker *Broker
	logger *slog.Logger
}

func NewHandler(broker *Broker, logger *slog.Logger) *Handler {
	return &Handler{broker: broker, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /devices/events", h.events)
}

type stateChangeEvent struct {
	DeviceID      string       `json:"device_id"`
	PreviousState device.State `json:"previous_state"`
	NewState      device.State `json:"new_state"`
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	// The stream is long-lived; a server-wide write timeout must not cut it.
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		h.logger.ErrorContext(r.Context(), "event stream not supported", "error", err)
		return
	}

	changes, unsubscribe := h.broker.Subscribe()
	defer unsubscribe()

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case change, ok := <-changes:
			if !ok {
				return
			}
			if err := writeEvent(w, change); err != nil {
				return
			}
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

func writeEvent(w http.ResponseWriter, change device.StateChange) error {
	data, err := json.Marshal(stateChangeEvent{
		DeviceID:      change.DeviceID.String(),
		PreviousState: change.Previous,
		NewState:      change.Current,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: state-changed\ndata: %s\n\n", data)
	return err
}
