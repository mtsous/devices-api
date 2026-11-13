package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

	page    *Page
	listErr error

	listCalled bool
	lastFilter Filter
	lastPage   PageRequest

	updated   *Device
	updateErr error

	updateCalled bool
	lastUpdateID uuid.UUID
	lastUpdate   UpdateParams

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

func (f *fakeService) List(_ context.Context, filter Filter, page PageRequest) (*Page, error) {
	f.listCalled = true
	f.lastFilter = filter
	f.lastPage = page
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.page, nil
}

func (f *fakeService) Update(_ context.Context, id uuid.UUID, params UpdateParams) (*Device, error) {
	f.updateCalled = true
	f.lastUpdateID = id
	f.lastUpdate = params
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updated, nil
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

func formatParams(p UpdateParams) string {
	show := func(v any, isNil bool) string {
		if isNil {
			return "<nil>"
		}
		return fmt.Sprintf("%q", v)
	}
	return fmt.Sprintf("{Name:%s Brand:%s State:%s}",
		show(deref(p.Name), p.Name == nil),
		show(deref(p.Brand), p.Brand == nil),
		show(deref(p.State), p.State == nil))
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
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

func TestHandlerUpdate(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	updatedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	updated := &Device{
		ID:        id,
		Name:      "Galaxy",
		Brand:     "Samsung",
		State:     StateInactive,
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: &updatedAt,
	}

	tests := []struct {
		name      string
		id        string
		body      string
		updateErr error

		wantStatus int
		wantParams *UpdateParams
		wantError  string
	}{
		{
			name:       "updates only name",
			id:         id.String(),
			body:       `{"name":"Galaxy"}`,
			wantStatus: http.StatusOK,
			wantParams: &UpdateParams{Name: new("Galaxy")},
		},
		{
			name:       "updates only brand",
			id:         id.String(),
			body:       `{"brand":"Samsung"}`,
			wantStatus: http.StatusOK,
			wantParams: &UpdateParams{Brand: new("Samsung")},
		},
		{
			name:       "updates only state",
			id:         id.String(),
			body:       `{"state":"available"}`,
			wantStatus: http.StatusOK,
			wantParams: &UpdateParams{State: new(StateAvailable)},
		},
		{
			name:       "updates all fields",
			id:         id.String(),
			body:       `{"name":"Galaxy","brand":"Samsung","state":"inactive"}`,
			wantStatus: http.StatusOK,
			wantParams: &UpdateParams{Name: new("Galaxy"), Brand: new("Samsung"), State: new(StateInactive)},
		},
		{
			name:       "passes empty string as sent",
			id:         id.String(),
			body:       `{"brand":""}`,
			updateErr:  fmt.Errorf("%w: brand is required", ErrInvalidInput),
			wantStatus: http.StatusBadRequest,
			wantParams: &UpdateParams{Brand: new("")},
			wantError:  "invalid device input: brand is required",
		},
		{
			name:       "rejects malformed id",
			id:         "not-a-uuid",
			body:       `{"name":"Galaxy"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid device id",
		},
		{
			name:       "rejects malformed json",
			id:         id.String(),
			body:       `{"name":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "rejects created_at",
			id:         id.String(),
			body:       `{"created_at":"2020-01-01T00:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "maps not found to 404",
			id:         id.String(),
			body:       `{"name":"Galaxy"}`,
			updateErr:  fmt.Errorf("update device %s: %w", id, ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantParams: &UpdateParams{Name: new("Galaxy")},
			wantError:  "device not found",
		},
		{
			name:       "maps in use to 409",
			id:         id.String(),
			body:       `{"brand":"Samsung"}`,
			updateErr:  fmt.Errorf("update device %s: %w", id, ErrInUse),
			wantStatus: http.StatusConflict,
			wantParams: &UpdateParams{Brand: new("Samsung")},
			wantError:  "device is in use",
		},
		{
			name:       "hides unexpected errors",
			id:         id.String(),
			body:       `{"name":"Galaxy"}`,
			updateErr:  errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantParams: &UpdateParams{Name: new("Galaxy")},
			wantError:  "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{updated: updated, updateErr: tc.updateErr}
			req := httptest.NewRequest(http.MethodPatch, "/devices/"+tc.id, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			newTestMux(svc).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			if tc.wantParams == nil {
				if svc.updateCalled {
					t.Fatal("service Update called, want request rejected first")
				}
			} else {
				if svc.lastUpdateID != id {
					t.Fatalf("service Update id = %s, want %s", svc.lastUpdateID, id)
				}
				if got, want := formatParams(svc.lastUpdate), formatParams(*tc.wantParams); got != want {
					t.Fatalf("service Update params = %s, want %s", got, want)
				}
			}

			if tc.wantError != "" {
				var body errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Error != tc.wantError {
					t.Fatalf("error = %q, want %q", body.Error, tc.wantError)
				}
				return
			}

			var got map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			want := map[string]any{
				"id":         id.String(),
				"name":       "Galaxy",
				"brand":      "Samsung",
				"state":      "inactive",
				"created_at": "2026-01-01T12:00:00Z",
				"updated_at": "2026-03-01T09:00:00Z",
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

func TestHandlerList(t *testing.T) {
	first := &Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "iPhone",
		Brand:     "Apple",
		State:     StateAvailable,
		CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	second := &Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "Pixel",
		Brand:     "Google",
		State:     StateInUse,
		CreatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
	}
	cursor := uuid.Must(uuid.NewV7())

	tests := []struct {
		name    string
		query   string
		page    *Page
		listErr error

		wantStatus     int
		wantFilter     Filter
		wantPage       PageRequest
		wantCalled     bool
		wantIDs        []string
		wantNextCursor string // empty means the field must be absent
		wantError      string
	}{
		{
			name:       "lists devices without filters",
			page:       &Page{Devices: []*Device{first, second}},
			wantStatus: http.StatusOK,
			wantCalled: true,
			wantIDs:    []string{first.ID.String(), second.ID.String()},
		},
		{
			name:       "passes brand filter",
			query:      "?brand=Apple",
			page:       &Page{Devices: []*Device{first}},
			wantStatus: http.StatusOK,
			wantFilter: Filter{Brand: "Apple"},
			wantCalled: true,
			wantIDs:    []string{first.ID.String()},
		},
		{
			name:       "passes state filter",
			query:      "?state=in-use",
			page:       &Page{Devices: []*Device{second}},
			wantStatus: http.StatusOK,
			wantFilter: Filter{State: StateInUse},
			wantCalled: true,
			wantIDs:    []string{second.ID.String()},
		},
		{
			name:           "passes cursor and limit and returns next cursor",
			query:          "?cursor=" + cursor.String() + "&limit=1",
			page:           &Page{Devices: []*Device{first}, Cursor: first.ID},
			wantStatus:     http.StatusOK,
			wantPage:       PageRequest{Size: 1, Cursor: cursor},
			wantCalled:     true,
			wantIDs:        []string{first.ID.String()},
			wantNextCursor: first.ID.String(),
		},
		{
			name:       "returns empty list",
			page:       &Page{Devices: []*Device{}},
			wantStatus: http.StatusOK,
			wantCalled: true,
			wantIDs:    []string{},
		},
		{
			name:       "rejects malformed cursor",
			query:      "?cursor=not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid cursor",
		},
		{
			name:       "rejects non-numeric limit",
			query:      "?limit=ten",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid limit",
		},
		{
			name:       "maps invalid filter to bad request",
			query:      "?brand=Apple&state=in-use",
			listErr:    fmt.Errorf("%w: filter by brand or state, not both", ErrInvalidInput),
			wantStatus: http.StatusBadRequest,
			wantFilter: Filter{Brand: "Apple", State: StateInUse},
			wantCalled: true,
			wantError:  "invalid device input: filter by brand or state, not both",
		},
		{
			name:       "hides unexpected errors",
			listErr:    errors.New("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
			wantError:  "internal server error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{page: tc.page, listErr: tc.listErr}
			req := httptest.NewRequest(http.MethodGet, "/devices"+tc.query, nil)
			rec := httptest.NewRecorder()

			newTestMux(svc).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			if svc.listCalled != tc.wantCalled {
				t.Fatalf("service List called = %v, want %v", svc.listCalled, tc.wantCalled)
			}
			if tc.wantCalled {
				if svc.lastFilter != tc.wantFilter {
					t.Fatalf("service List filter = %+v, want %+v", svc.lastFilter, tc.wantFilter)
				}
				if svc.lastPage != tc.wantPage {
					t.Fatalf("service List page = %+v, want %+v", svc.lastPage, tc.wantPage)
				}
			}

			if tc.wantError != "" {
				var body errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode error body: %v", err)
				}
				if body.Error != tc.wantError {
					t.Fatalf("error = %q, want %q", body.Error, tc.wantError)
				}
				return
			}

			var got map[string]json.RawMessage
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			var devices []deviceResponse
			if err := json.Unmarshal(got["devices"], &devices); err != nil {
				t.Fatalf("decode devices %s: %v", got["devices"], err)
			}
			if devices == nil {
				t.Fatalf("devices = %s, want a JSON array", got["devices"])
			}
			gotIDs := make([]string, len(devices))
			for i, d := range devices {
				gotIDs[i] = d.ID
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Fatalf("device ids = %v, want %v", gotIDs, tc.wantIDs)
			}

			rawCursor, hasCursor := got["next_cursor"]
			if tc.wantNextCursor == "" {
				if hasCursor {
					t.Fatalf("next_cursor = %s, want it absent", rawCursor)
				}
				return
			}
			var nextCursor string
			if err := json.Unmarshal(rawCursor, &nextCursor); err != nil {
				t.Fatalf("decode next_cursor %s: %v", rawCursor, err)
			}
			if nextCursor != tc.wantNextCursor {
				t.Fatalf("next_cursor = %q, want %q", nextCursor, tc.wantNextCursor)
			}
		})
	}
}
