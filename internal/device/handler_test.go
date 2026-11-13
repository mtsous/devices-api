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

	device *Device
	getErr error

	getCalled bool
	lastGetID uuid.UUID

	deleteErr error

	deleteCalled bool
	lastDeleteID uuid.UUID
}

func (f *fakeService) Create(_ context.Context, device Device) (*Device, error) {
	f.createCalled = true
	f.lastCreate = device
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

func (f *fakeService) Get(_ context.Context, id uuid.UUID) (*Device, error) {
	f.getCalled = true
	f.lastGetID = id
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.device, nil
}

func (f *fakeService) Delete(_ context.Context, id uuid.UUID) error {
	f.deleteCalled = true
	f.lastDeleteID = id
	return f.deleteErr
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

func TestHandlerGet(t *testing.T) {
	updatedAt := time.Date(2026, 2, 1, 8, 30, 0, 0, time.UTC)
	device := &Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "Pixel",
		Brand:     "Google",
		State:     StateInUse,
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: &updatedAt,
	}

	tests := []struct {
		name   string
		id     string
		getErr error

		wantStatus      int
		wantCalled      bool
		wantErrContains string
	}{
		{
			name:       "returns device",
			id:         device.ID.String(),
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:            "rejects malformed id",
			id:              "not-a-uuid",
			wantStatus:      http.StatusBadRequest,
			wantErrContains: "invalid device id",
		},
		{
			name:            "maps not found to 404",
			id:              device.ID.String(),
			getErr:          fmt.Errorf("get device %s: %w", device.ID, ErrNotFound),
			wantStatus:      http.StatusNotFound,
			wantCalled:      true,
			wantErrContains: "device not found",
		},
		{
			name:            "hides unexpected errors",
			id:              device.ID.String(),
			getErr:          errors.New("connection refused"),
			wantStatus:      http.StatusInternalServerError,
			wantCalled:      true,
			wantErrContains: "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{device: device, getErr: tc.getErr}
			req := httptest.NewRequest(http.MethodGet, "/devices/"+tc.id, nil)
			rec := httptest.NewRecorder()

			newTestMux(svc).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			if svc.getCalled != tc.wantCalled {
				t.Fatalf("service Get called = %v, want %v", svc.getCalled, tc.wantCalled)
			}
			if tc.wantCalled && svc.lastGetID != device.ID {
				t.Fatalf("service Get id = %s, want %s", svc.lastGetID, device.ID)
			}

			if tc.wantErrContains != "" {
				var body errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Error != tc.wantErrContains {
					t.Fatalf("error = %q, want %q", body.Error, tc.wantErrContains)
				}
				return
			}

			var got map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			want := map[string]any{
				"id":         device.ID.String(),
				"name":       "Pixel",
				"brand":      "Google",
				"state":      "in-use",
				"created_at": "2026-01-01T12:00:00Z",
				"updated_at": "2026-02-01T08:30:00Z",
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

func TestHandlerDelete(t *testing.T) {
	id := uuid.Must(uuid.NewV7())

	tests := []struct {
		name      string
		id        string
		deleteErr error

		wantStatus int
		wantCalled bool
		wantError  string
	}{
		{
			name:       "deletes device",
			id:         id.String(),
			wantStatus: http.StatusNoContent,
			wantCalled: true,
		},
		{
			name:       "rejects malformed id",
			id:         "not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid device id",
		},
		{
			name:       "maps not found to 404",
			id:         id.String(),
			deleteErr:  fmt.Errorf("delete device %s: %w", id, ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCalled: true,
			wantError:  "device not found",
		},
		{
			name:       "maps in use to 409",
			id:         id.String(),
			deleteErr:  fmt.Errorf("delete device %s: %w", id, ErrInUse),
			wantStatus: http.StatusConflict,
			wantCalled: true,
			wantError:  "device is in use",
		},
		{
			name:       "hides unexpected errors",
			id:         id.String(),
			deleteErr:  errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
			wantError:  "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{deleteErr: tc.deleteErr}
			req := httptest.NewRequest(http.MethodDelete, "/devices/"+tc.id, nil)
			rec := httptest.NewRecorder()

			newTestMux(svc).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if svc.deleteCalled != tc.wantCalled {
				t.Fatalf("service Delete called = %v, want %v", svc.deleteCalled, tc.wantCalled)
			}
			if tc.wantCalled && svc.lastDeleteID != id {
				t.Fatalf("service Delete id = %s, want %s", svc.lastDeleteID, id)
			}

			if tc.wantError == "" {
				if rec.Body.Len() != 0 {
					t.Fatalf("body = %q, want empty", rec.Body)
				}
				return
			}
			var body errorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Error != tc.wantError {
				t.Fatalf("error = %q, want %q", body.Error, tc.wantError)
			}
		})
	}
}
