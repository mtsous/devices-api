package docs

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		path            string
		wantContentType string
		wantContains    string
	}{
		{path: "/openapi.yml", wantContentType: "application/yaml", wantContains: "openapi: 3.1.0"},
		{path: "/docs", wantContentType: "text/html; charset=utf-8", wantContains: `url: "/openapi.yml"`},
	}

	mux := http.NewServeMux()
	NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(mux)

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != tc.wantContentType {
				t.Fatalf("Content-Type = %q, want %q", got, tc.wantContentType)
			}
			if !strings.Contains(rec.Body.String(), tc.wantContains) {
				t.Fatalf("body does not contain %q", tc.wantContains)
			}
		})
	}
}
