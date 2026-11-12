package health

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(mux)

	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
	}{
		{name: "GET returns ok", method: http.MethodGet, wantStatus: http.StatusOK, wantBody: "ok"},
		{name: "POST is not allowed", method: http.MethodPost, wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/healthz", nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantBody == "" {
				return
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("content-type: got %q", got)
			}
			if rec.Body.String() != tc.wantBody {
				t.Fatalf("body: got %q, want %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}
