package device

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepository is an in-memory Repository for service tests. It only
// stores and returns devices; repository rules such as timestamps, soft
// delete, filtering and ordering are covered by the Postgres repository tests.
type fakeRepository struct {
	devices map[uuid.UUID]Device

	listResult []*Device

	createErr error
	getErr    error
	listErr   error
	updateErr error
	deleteErr error

	// Recorded calls.
	createCalled    bool
	listCalled      bool
	updateCalled    bool
	lastFilter      Filter
	lastPageRequest PageRequest
	deletedIDs      []uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{devices: map[uuid.UUID]Device{}}
}

func (f *fakeRepository) Create(_ context.Context, device Device) (*Device, error) {
	f.createCalled = true
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &device, nil
}

func (f *fakeRepository) Get(_ context.Context, id uuid.UUID) (*Device, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	device, ok := f.devices[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &device, nil
}

func (f *fakeRepository) List(_ context.Context, filter Filter, page PageRequest) ([]*Device, error) {
	f.listCalled = true
	f.lastFilter = filter
	f.lastPageRequest = page
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listResult) == 0 {
		return nil, nil // result query with no rows
	}
	return f.listResult[:min(page.Size, len(f.listResult))], nil
}

func (f *fakeRepository) Update(_ context.Context, id uuid.UUID, fn func(d *Device) error) (*Device, error) {
	f.updateCalled = true
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	device, ok := f.devices[id]
	if !ok {
		return nil, ErrNotFound
	}
	if err := fn(&device); err != nil {
		return nil, err
	}
	f.devices[id] = device
	return &device, nil
}

func (f *fakeRepository) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedIDs = append(f.deletedIDs, id)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.devices[id]; !ok {
		return ErrNotFound
	}
	delete(f.devices, id)
	return nil
}

// fakeNotifier records the state changes it is asked to deliver.
type fakeNotifier struct {
	changes []StateChange
}

func (f *fakeNotifier) Notify(_ context.Context, change StateChange) {
	f.changes = append(f.changes, change)
}

var (
	errFakeRepo   = errors.New("fake repository error")
	discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func newTestDevice(state State) Device {
	return Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "iPhone",
		Brand:     "Apple",
		State:     state,
		CreatedAt: time.Now().UTC(),
	}
}

func newTestDevices(n int) []*Device {
	devices := make([]*Device, n)
	for i := range devices {
		d := newTestDevice(StateAvailable)
		devices[i] = &d
	}
	return devices
}

func ids(devices []*Device) []uuid.UUID {
	out := make([]uuid.UUID, len(devices))
	for i, d := range devices {
		out[i] = d.ID
	}
	return out
}

func assertDeviceEqual(t *testing.T, got, want Device) {
	t.Helper()
	if got.ID != want.ID ||
		got.Name != want.Name ||
		got.Brand != want.Brand ||
		got.State != want.State ||
		!got.CreatedAt.Equal(want.CreatedAt) ||
		!equalTimePtr(got.UpdatedAt, want.UpdatedAt) ||
		!equalTimePtr(got.DeletedAt, want.DeletedAt) {
		t.Fatalf("device = %s, want %s", got, want)
	}
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func TestServiceCreate(t *testing.T) {
	tests := []struct {
		name      string
		input     Device
		createErr error
		want      Device
		wantErr   error
	}{
		{
			name:  "defaults state to inactive",
			input: Device{Name: "iPhone", Brand: "Apple"},
			want:  Device{Name: "iPhone", Brand: "Apple", State: StateInactive},
		},
		{
			name:  "keeps a valid state",
			input: Device{Name: "Pixel", Brand: "Google", State: StateInUse},
			want:  Device{Name: "Pixel", Brand: "Google", State: StateInUse},
		},
		{
			name:  "trims name and brand",
			input: Device{Name: "  Galaxy ", Brand: " Samsung  "},
			want:  Device{Name: "Galaxy", Brand: "Samsung", State: StateInactive},
		},
		{
			name:    "rejects empty name",
			input:   Device{Name: "  ", Brand: "Apple"},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "rejects empty brand",
			input:   Device{Name: "iPhone"},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "rejects invalid state",
			input:   Device{Name: "iPhone", Brand: "Apple", State: "broken"},
			wantErr: ErrInvalidState,
		},
		{
			name:      "returns repository error",
			input:     Device{Name: "iPhone", Brand: "Apple"},
			createErr: errFakeRepo,
			wantErr:   errFakeRepo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.createErr = tc.createErr

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			got, err := svc.Create(context.Background(), tc.input)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("Create() = %s, want nil on error", got)
				}
				if tc.createErr == nil && repo.createCalled {
					t.Fatal("repository Create() called, validation should fail first")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Create() returned a nil device")
			}

			if got.ID.Version() != 7 {
				t.Fatalf("ID version = %d, want 7", got.ID.Version())
			}
			want := tc.want
			want.ID = got.ID
			assertDeviceEqual(t, *got, want)
		})
	}
}

func TestServiceGet(t *testing.T) {
	existing := newTestDevice(StateAvailable)

	tests := []struct {
		name    string
		id      uuid.UUID
		getErr  error
		wantErr error
	}{
		{
			name: "returns existing device",
			id:   existing.ID,
		},
		{
			name:    "returns not found for unknown id",
			id:      uuid.Must(uuid.NewV7()),
			wantErr: ErrNotFound,
		},
		{
			name:    "returns repository error",
			id:      existing.ID,
			getErr:  errFakeRepo,
			wantErr: errFakeRepo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.devices[existing.ID] = existing
			repo.getErr = tc.getErr

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			got, err := svc.Get(context.Background(), tc.id)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Get() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("Get() = %s, want nil on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Get() returned a nil device")
			}
			assertDeviceEqual(t, *got, existing)
		})
	}
}

func TestServiceList(t *testing.T) {
	cursor := uuid.Must(uuid.NewV7())

	tests := []struct {
		name        string
		filter      Filter
		page        PageRequest
		deviceCount int
		listErr     error

		wantFilter      Filter
		wantRequest     PageRequest
		wantDeviceCount int
		wantCursor      bool // should be ID of the last returned device
		wantErr         error
	}{
		{
			name:            "uses default size when not set",
			deviceCount:     5,
			wantRequest:     PageRequest{Size: DefaultPageLimit + 1},
			wantDeviceCount: 5,
		},
		{
			name:        "uses default size when negative",
			page:        PageRequest{Size: -1},
			wantRequest: PageRequest{Size: DefaultPageLimit + 1},
		},
		{
			name:        "caps size at max",
			page:        PageRequest{Size: MaxPageLimit + 50},
			wantRequest: PageRequest{Size: MaxPageLimit + 1},
		},
		{
			name:        "passes cursor to repository",
			page:        PageRequest{Size: 2, Cursor: cursor},
			wantRequest: PageRequest{Size: 3, Cursor: cursor},
		},
		{
			name:            "returns cursor when more pages exist",
			page:            PageRequest{Size: 2},
			deviceCount:     3,
			wantRequest:     PageRequest{Size: 3},
			wantDeviceCount: 2,
			wantCursor:      true,
		},
		{
			name:            "returns no cursor on exact page size",
			page:            PageRequest{Size: 2},
			deviceCount:     2,
			wantRequest:     PageRequest{Size: 3},
			wantDeviceCount: 2,
		},
		{
			name:        "trims brand filter",
			filter:      Filter{Brand: "  Samsung "},
			wantFilter:  Filter{Brand: "Samsung"},
			wantRequest: PageRequest{Size: DefaultPageLimit + 1},
		},
		{
			name:        "passes state filter",
			filter:      Filter{State: StateInUse},
			wantFilter:  Filter{State: StateInUse},
			wantRequest: PageRequest{Size: DefaultPageLimit + 1},
		},
		{
			name:    "rejects brand and state together",
			filter:  Filter{Brand: "Apple", State: StateInUse},
			wantErr: ErrInvalidInput,
		},
		{
			name:    "rejects invalid state filter",
			filter:  Filter{State: "broken"},
			wantErr: ErrInvalidState,
		},
		{
			name:    "returns repository error",
			listErr: errFakeRepo,
			wantErr: errFakeRepo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.listResult = newTestDevices(tc.deviceCount)
			repo.listErr = tc.listErr

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			got, err := svc.List(context.Background(), tc.filter, tc.page)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("List() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("List() = %+v, want nil on error", got)
				}
				if tc.listErr == nil && repo.listCalled {
					t.Fatal("repository List called, want validation to fail first")
				}
				return
			}
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("List() returned a nil page")
			}

			if repo.lastFilter != tc.wantFilter {
				t.Fatalf("repository filter = %+v, want %+v", repo.lastFilter, tc.wantFilter)
			}
			if repo.lastPageRequest != tc.wantRequest {
				t.Fatalf("repository page request = %+v, want %+v", repo.lastPageRequest, tc.wantRequest)
			}

			if got.Devices == nil {
				t.Fatal("List() Devices = nil, want empty slice")
			}
			wantIDs := ids(repo.listResult[:tc.wantDeviceCount])
			if gotIDs := ids(got.Devices); !slices.Equal(gotIDs, wantIDs) {
				t.Fatalf("List() ids = %v, want %v", gotIDs, wantIDs)
			}

			wantCursor := uuid.Nil
			if tc.wantCursor {
				wantCursor = wantIDs[len(wantIDs)-1]
			}
			if got.Cursor != wantCursor {
				t.Fatalf("List() cursor = %v, want %v", got.Cursor, wantCursor)
			}
			if got.HasNext() != tc.wantCursor {
				t.Fatalf("HasNext() = %v, want %v", got.HasNext(), tc.wantCursor)
			}
		})
	}
}

func TestServiceDelete(t *testing.T) {
	tests := []struct {
		name      string
		deleteErr error
		wantErr   error
	}{
		{
			name: "deletes device",
		},
		{
			name:      "returns in use from repository",
			deleteErr: ErrInUse,
			wantErr:   ErrInUse,
		},
		{
			name:      "returns not found from repository",
			deleteErr: ErrNotFound,
			wantErr:   ErrNotFound,
		},
		{
			name:      "returns repository error",
			deleteErr: errFakeRepo,
			wantErr:   errFakeRepo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device := newTestDevice(StateAvailable)
			repo := newFakeRepository()
			repo.devices[device.ID] = device
			repo.deleteErr = tc.deleteErr

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			err := svc.Delete(context.Background(), device.ID)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Delete() error = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Delete() unexpected error: %v", err)
			}

			if want := []uuid.UUID{device.ID}; !slices.Equal(repo.deletedIDs, want) {
				t.Fatalf("repository Delete called with %v, want %v", repo.deletedIDs, want)
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	tests := []struct {
		name      string
		stored    Device
		params    UpdateParams
		unknownID bool
		updateErr error

		want           func(stored Device) Device
		wantErr        error
		wantRepoCalled bool
	}{
		{
			name:   "updates only sent fields",
			stored: newTestDevice(StateAvailable),
			params: UpdateParams{Name: new("Galaxy")},
			want: func(d Device) Device {
				d.Name = "Galaxy"
				return d
			},
			wantRepoCalled: true,
		},
		{
			name:   "updates all fields",
			stored: newTestDevice(StateAvailable),
			params: UpdateParams{Name: new("Galaxy"), Brand: new("Samsung"), State: new(StateInUse)},
			want: func(d Device) Device {
				d.Name, d.Brand, d.State = "Galaxy", "Samsung", StateInUse
				return d
			},
			wantRepoCalled: true,
		},
		{
			name:   "trims name and brand",
			stored: newTestDevice(StateAvailable),
			params: UpdateParams{Name: new("  Galaxy "), Brand: new(" Samsung  ")},
			want: func(d Device) Device {
				d.Name, d.Brand = "Galaxy", "Samsung"
				return d
			},
			wantRepoCalled: true,
		},
		{
			name:   "changes state of in-use device",
			stored: newTestDevice(StateInUse),
			params: UpdateParams{State: new(StateAvailable)},
			want: func(d Device) Device {
				d.State = StateAvailable
				return d
			},
			wantRepoCalled: true,
		},
		{
			name:   "allows unchanged name and brand while in use",
			stored: newTestDevice(StateInUse),
			params: UpdateParams{Name: new("iPhone"), Brand: new("Apple"), State: new(StateInactive)},
			want: func(d Device) Device {
				d.State = StateInactive
				return d
			},
			wantRepoCalled: true,
		},
		{
			name:           "rejects name change while in use",
			stored:         newTestDevice(StateInUse),
			params:         UpdateParams{Name: new("Galaxy")},
			wantErr:        ErrInUse,
			wantRepoCalled: true,
		},
		{
			name:           "rejects brand change while in use",
			stored:         newTestDevice(StateInUse),
			params:         UpdateParams{Brand: new("Samsung")},
			wantErr:        ErrInUse,
			wantRepoCalled: true,
		},
		{
			name:           "rejects name change while in use even when leaving in-use",
			stored:         newTestDevice(StateInUse),
			params:         UpdateParams{Name: new("Galaxy"), State: new(StateAvailable)},
			wantErr:        ErrInUse,
			wantRepoCalled: true,
		},
		{
			name:           "rejects empty name",
			stored:         newTestDevice(StateAvailable),
			params:         UpdateParams{Name: new("  ")},
			wantErr:        ErrInvalidInput,
			wantRepoCalled: true,
		},
		{
			name:           "rejects invalid state",
			stored:         newTestDevice(StateAvailable),
			params:         UpdateParams{State: new(State("broken"))},
			wantErr:        ErrInvalidState,
			wantRepoCalled: true,
		},
		{
			name:    "rejects empty update",
			stored:  newTestDevice(StateAvailable),
			wantErr: ErrInvalidInput,
		},
		{
			name:           "returns not found for unknown id",
			stored:         newTestDevice(StateAvailable),
			params:         UpdateParams{Name: new("Galaxy")},
			unknownID:      true,
			wantErr:        ErrNotFound,
			wantRepoCalled: true,
		},
		{
			name:           "returns repository error",
			stored:         newTestDevice(StateAvailable),
			params:         UpdateParams{Name: new("Galaxy")},
			updateErr:      errFakeRepo,
			wantErr:        errFakeRepo,
			wantRepoCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.devices[tc.stored.ID] = tc.stored
			repo.updateErr = tc.updateErr

			id := tc.stored.ID
			if tc.unknownID {
				id = uuid.Must(uuid.NewV7())
			}

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			got, err := svc.Update(context.Background(), id, tc.params)

			if repo.updateCalled != tc.wantRepoCalled {
				t.Fatalf("repository Update called = %v, want %v", repo.updateCalled, tc.wantRepoCalled)
			}

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Update() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("Update() = %s, want nil on error", got)
				}
				// A rejected update must leave the stored device untouched.
				assertDeviceEqual(t, repo.devices[tc.stored.ID], tc.stored)
				return
			}
			if err != nil {
				t.Fatalf("Update() unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Update() returned a nil device")
			}

			want := tc.want(tc.stored)
			assertDeviceEqual(t, *got, want)
			assertDeviceEqual(t, repo.devices[tc.stored.ID], want)
		})
	}
}

func TestServiceUpdateErrorMessages(t *testing.T) {
	tests := []struct {
		name      string
		stored    Device
		params    UpdateParams
		updateErr error
		wantMsg   string
	}{
		{
			name:    "validation error is not wrapped",
			stored:  newTestDevice(StateAvailable),
			params:  UpdateParams{Name: new("  ")},
			wantMsg: "invalid device input: name is required",
		},
		{
			name:    "in-use error is not wrapped",
			stored:  newTestDevice(StateInUse),
			params:  UpdateParams{Name: new("Galaxy")},
			wantMsg: "device is in use: name and brand cannot change",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.devices[tc.stored.ID] = tc.stored

			svc := NewService(repo, &fakeNotifier{}, discardLogger)
			_, err := svc.Update(context.Background(), tc.stored.ID, tc.params)

			if err == nil || err.Error() != tc.wantMsg {
				t.Fatalf("Update() error = %v, want %q", err, tc.wantMsg)
			}
		})
	}

	t.Run("repository error keeps the device id", func(t *testing.T) {
		stored := newTestDevice(StateAvailable)
		repo := newFakeRepository()
		repo.devices[stored.ID] = stored
		repo.updateErr = errFakeRepo

		svc := NewService(repo, &fakeNotifier{}, discardLogger)
		_, err := svc.Update(context.Background(), stored.ID, UpdateParams{Name: new("Galaxy")})

		if want := "update device " + stored.ID.String() + ": " + errFakeRepo.Error(); err == nil || err.Error() != want {
			t.Fatalf("Update() error = %v, want %q", err, want)
		}
	})
}

func TestServiceUpdateNotifies(t *testing.T) {
	tests := []struct {
		name      string
		stored    Device
		params    UpdateParams
		updateErr error
		want      []StateChange
	}{
		{
			name:   "notifies state change with previous and new state",
			stored: newTestDevice(StateAvailable),
			params: UpdateParams{State: new(StateInUse)},
			want:   []StateChange{{Previous: StateAvailable, Current: StateInUse}},
		},
		{
			name:   "notifies when state changes along with other fields",
			stored: newTestDevice(StateInactive),
			params: UpdateParams{Name: new("Galaxy"), State: new(StateAvailable)},
			want:   []StateChange{{Previous: StateInactive, Current: StateAvailable}},
		},
		{
			name:   "does not notify a name-only change",
			stored: newTestDevice(StateAvailable),
			params: UpdateParams{Name: new("Galaxy")},
		},
		{
			name:   "does not notify when the same state is sent",
			stored: newTestDevice(StateInUse),
			params: UpdateParams{State: new(StateInUse)},
		},
		{
			name:   "does not notify a rejected update",
			stored: newTestDevice(StateInUse),
			params: UpdateParams{Name: new("Galaxy"), State: new(StateAvailable)},
		},
		{
			name:      "does not notify when the repository fails",
			stored:    newTestDevice(StateAvailable),
			params:    UpdateParams{State: new(StateInUse)},
			updateErr: errFakeRepo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.devices[tc.stored.ID] = tc.stored
			repo.updateErr = tc.updateErr
			notifier := &fakeNotifier{}

			svc := NewService(repo, notifier, discardLogger)
			_, _ = svc.Update(context.Background(), tc.stored.ID, tc.params)

			for i := range tc.want {
				tc.want[i].DeviceID = tc.stored.ID
			}
			if !slices.Equal(notifier.changes, tc.want) {
				t.Fatalf("notifications = %+v, want %+v", notifier.changes, tc.want)
			}
		})
	}
}
