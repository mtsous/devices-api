package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeService struct {
	created   *Device
	createErr error

	createCalled bool
	lastCreate   Device
}

func (f *fakeService) Create(_ context.Context, device Device) (*Device, error) {
	f.createCalled = true
	f.lastCreate = device
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

func newTestMux(svc deviceService) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(svc, discardLogger).RegisterRoutes(mux)
	return mux
}

func TestHandlerCreate(t *testing.T) {
	created := &Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "iPhone",
		Brand:     "Apple",
		State:     StateAvailable,
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name      string
		body      string
		createErr error

		wantStatus      int
		wantInput       *Device // what the handler passes to the service; nil means not called
		wantErrContains string
	}{
		{
			name:       "creates device",
			body:       `{"name":"iPhone","brand":"Apple","state":"available"}`,
			wantStatus: http.StatusCreated,
			wantInput:  &Device{Name: "iPhone", Brand: "Apple", State: StateAvailable},
		},
		{
			name:       "passes missing state as empty",
			body:       `{"name":"iPhone","brand":"Apple"}`,
			wantStatus: http.StatusCreated,
			wantInput:  &Device{Name: "iPhone", Brand: "Apple"},
		},
		{
			name:            "rejects malformed json",
			body:            `{"name":`,
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid request body",
		},
		{
			name:            "rejects unknown fields",
			body:            `{"name":"iPhone","brand":"Apple","created_at":"2020-01-01T00:00:00Z"}`,
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid request body",
		},
		{
			name:            "rejects multiple json objects",
			body:            `{"name":"iPhone","brand":"Apple"}{"name":"Pixel"}`,
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid request body",
		},
		{
			name:            "rejects empty body",
			body:            ``,
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid request body",
		},
		{
			name:            "maps invalid input to bad request",
			body:            `{"name":"","brand":"Apple"}`,
			createErr:       fmt.Errorf("%w: name is required", ErrInvalidInput),
			wantStatus:      http.StatusBadRequest,
			wantInput:       &Device{Brand: "Apple"},
			wantErrContains: "name is required",
		},
		{
			name:            "maps invalid state to bad request",
			body:            `{"name":"iPhone","brand":"Apple","state":"broken"}`,
			createErr:       fmt.Errorf("%w: %q", ErrInvalidState, "broken"),
			wantStatus:      http.StatusBadRequest,
			wantInput:       &Device{Name: "iPhone", Brand: "Apple", State: "broken"},
			wantErrContains: "invalid device state",
		},
		{
			name:            "hides unexpected errors",
			body:            `{"name":"iPhone","brand":"Apple"}`,
			createErr:       errors.New("connection refused"),
			wantStatus:      http.StatusInternalServerError,
			wantInput:       &Device{Name: "iPhone", Brand: "Apple"},
			wantErrContains: "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{created: created, createErr: tc.createErr}
			req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			newTestMux(svc).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			if tc.wantInput == nil {
				if svc.createCalled {
					t.Fatal("service Create called, want request rejected first")
				}
			} else {
				assertDeviceEqual(t, svc.lastCreate, *tc.wantInput)
			}

			if tc.wantErrContains != "" {
				var body errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if !strings.Contains(body.Error, tc.wantErrContains) {
					t.Fatalf("error = %q, want it to contain %q", body.Error, tc.wantErrContains)
				}

				errIsConnectionRefused := strings.Contains(body.Error, "connection refused")
				if tc.wantStatus == http.StatusInternalServerError && errIsConnectionRefused {
					t.Fatalf("error = %q leaks internal details", body.Error)
				}
				return
			}

			if got, want := rec.Header().Get("Location"), "/devices/"+created.ID.String(); got != want {
				t.Fatalf("Location = %q, want %q", got, want)
			}
			var got map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			want := map[string]any{
				"id":         created.ID.String(),
				"name":       "iPhone",
				"brand":      "Apple",
				"state":      "available",
				"created_at": "2026-01-01T12:00:00Z",
			}
			if len(got) != len(want) {
				t.Fatalf("response = %v, want %v", got, want)
			}
			for k, v := range want {
				if got[k] != v {
					t.Fatalf("response[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
