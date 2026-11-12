package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRegistrar struct {
	pattern string
}

func (f fakeRegistrar) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(f.pattern, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
}

func TestNew(t *testing.T) {
	r := New(
		fakeRegistrar{pattern: "GET /a"},
		fakeRegistrar{pattern: "GET /b"},
	)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "first registrar route", method: http.MethodGet, path: "/a", wantStatus: http.StatusTeapot},
		{name: "second registrar route", method: http.MethodGet, path: "/b", wantStatus: http.StatusTeapot},
		{name: "wrong method", method: http.MethodPost, path: "/a", wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/unknown", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
