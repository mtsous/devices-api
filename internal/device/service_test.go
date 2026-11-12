package device

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

var errFake = errors.New("fake repository error")

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeRepository struct {
	devices map[uuid.UUID]Device

	createErr error
	getErr    error
	listErr   error

	lastPageRequest PageRequest
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{devices: map[uuid.UUID]Device{}}
}

func (f *fakeRepository) Create(_ context.Context, device Device) (*Device, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.devices[device.ID] = device
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
	f.lastPageRequest = page

	if f.listErr != nil {
		return nil, f.listErr
	}

	matches := make([]Device, 0, len(f.devices))
	for _, d := range f.devices {
		if filter.Brand != "" && d.Brand != filter.Brand {
			continue
		}
		if filter.State != "" && d.State != filter.State {
			continue
		}
		if page.Cursor != uuid.Nil && bytes.Compare(d.ID[:], page.Cursor[:]) <= 0 {
			continue
		}
		matches = append(matches, d)
	}
	slices.SortFunc(matches, func(a, b Device) int {
		return bytes.Compare(a.ID[:], b.ID[:])
	})

	var result []*Device
	for i := range matches {
		if len(result) == page.Size {
			break
		}
		result = append(result, &matches[i])
	}
	return result, nil
}

func (f *fakeRepository) Update(context.Context, Device) error {
	return errors.New("not implemented")
}

func (f *fakeRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}

func TestServiceCreate(t *testing.T) {
	userInputTime := time.Now().Add(-24 * time.Hour)

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
			name: "ignores client provided id and timestamps",
			input: Device{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000004"),
				Name:      "iPhone",
				Brand:     "Apple",
				CreatedAt: userInputTime,
				UpdatedAt: &userInputTime,
			},
			want: Device{Name: "iPhone", Brand: "Apple", State: StateInactive},
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
			createErr: errFake,
			wantErr:   errFake,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.createErr = tc.createErr

			before := time.Now().UTC()
			svc := NewService(Config{Repo: repo, Logger: discardLogger})
			got, err := svc.Create(context.Background(), tc.input)
			after := time.Now().UTC()

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("Create() = %+v, want nil on error", got)
				}
				if len(repo.devices) != 0 {
					t.Fatalf("repository has %d devices, want 0", len(repo.devices))
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
			if got.ID == tc.input.ID {
				t.Fatalf("ID = %s, want a newly generated id", got.ID)
			}
			if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
				t.Fatalf(
					"CreatedAt = %v, want something between %v and %v",
					got.CreatedAt,
					before,
					after,
				)
			}
			if got.UpdatedAt != nil {
				t.Fatalf("UpdatedAt = %v, want nil", got.UpdatedAt)
			}
			if got.Name != tc.want.Name || got.Brand != tc.want.Brand || got.State != tc.want.State {
				t.Fatalf("Create() = %+v, want name=%q brand=%q state=%q",
					got, tc.want.Name, tc.want.Brand, tc.want.State)
			}

			stored, ok := repo.devices[got.ID]
			if !ok {
				t.Fatalf("device %s was not stored", got.ID)
			}
			if stored != *got {
				t.Fatalf("stored device = %+v, want %+v", stored, got)
			}
		})
	}
}

func TestServiceGet(t *testing.T) {
	existing := Device{
		ID:        uuid.Must(uuid.NewV7()),
		Name:      "iPhone",
		Brand:     "Apple",
		State:     StateAvailable,
		CreatedAt: time.Now().UTC(),
	}

	tests := []struct {
		name    string
		id      uuid.UUID
		getErr  error
		want    Device
		wantErr error
	}{
		{
			name: "returns existing device",
			id:   existing.ID,
			want: existing,
		},
		{
			name:    "returns not found for unknown id",
			id:      uuid.Must(uuid.NewV7()),
			wantErr: ErrNotFound,
		},
		{
			name:    "returns repository error",
			id:      existing.ID,
			getErr:  errFake,
			wantErr: errFake,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.devices[existing.ID] = existing
			repo.getErr = tc.getErr

			svc := NewService(Config{Repo: repo})
			got, err := svc.Get(context.Background(), tc.id)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Get() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("Get() = %+v, want nil on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}

			if got == nil {
				t.Fatal("Get() returned a nil device")
			}
			if *got != tc.want {
				t.Fatalf("Get() = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func newTestDevices(t *testing.T, n int, brand string, state State) []Device {
	t.Helper()
	devices := make([]Device, n)
	for i := range devices {
		devices[i] = Device{
			ID:        uuid.Must(uuid.NewV7()),
			Name:      "device",
			Brand:     brand,
			State:     state,
			CreatedAt: time.Now().UTC(),
		}
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

func valueIDs(devices []Device) []uuid.UUID {
	out := make([]uuid.UUID, len(devices))
	for i, d := range devices {
		out[i] = d.ID
	}
	return out
}

func TestServiceList(t *testing.T) {
	apple := newTestDevices(t, 3, "Apple", StateAvailable)
	samsung := newTestDevices(t, 2, "Samsung", StateInUse)
	all := append(append([]Device{}, apple...), samsung...)

	tests := []struct {
		name       string
		filter     Filter
		page       PageRequest
		listErr    error
		wantIDs    []uuid.UUID
		wantCursor uuid.UUID
		wantSize   int
		wantErr    error
	}{
		{
			name:     "returns all devices when no filter",
			wantIDs:  valueIDs(all),
			wantSize: DefaultPageLimit + 1,
		},
		{
			name:     "filters by brand",
			filter:   Filter{Brand: "Apple"},
			wantIDs:  valueIDs(apple),
			wantSize: DefaultPageLimit + 1,
		},
		{
			name:     "trims brand filter",
			filter:   Filter{Brand: "  Samsung "},
			wantIDs:  valueIDs(samsung),
			wantSize: DefaultPageLimit + 1,
		},
		{
			name:     "filters by state",
			filter:   Filter{State: StateInUse},
			wantIDs:  valueIDs(samsung),
			wantSize: DefaultPageLimit + 1,
		},
		{
			name:       "returns cursor when more pages exist",
			page:       PageRequest{Size: 2},
			wantIDs:    valueIDs(all[:2]),
			wantCursor: all[1].ID,
			wantSize:   3,
		},
		{
			name:       "continues from cursor",
			page:       PageRequest{Size: 2, Cursor: all[1].ID},
			wantIDs:    valueIDs(all[2:4]),
			wantCursor: all[3].ID,
			wantSize:   3,
		},
		{
			name:     "last page has no cursor",
			page:     PageRequest{Size: 2, Cursor: all[3].ID},
			wantIDs:  valueIDs(all[4:]),
			wantSize: 3,
		},
		{
			name:     "exact page size has no cursor",
			page:     PageRequest{Size: len(all)},
			wantIDs:  valueIDs(all),
			wantSize: len(all) + 1,
		},
		{
			name:     "caps size at max",
			page:     PageRequest{Size: MaxPageLimit + 50},
			wantIDs:  valueIDs(all),
			wantSize: MaxPageLimit + 1,
		},
		{
			name:     "defaults negative size",
			page:     PageRequest{Size: -1},
			wantIDs:  valueIDs(all),
			wantSize: DefaultPageLimit + 1,
		}, {
			name:     "returns empty slice when nothing matches",
			filter:   Filter{Brand: "Nokia"},
			wantIDs:  []uuid.UUID{},
			wantSize: DefaultPageLimit + 1,
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
			listErr: errFake,
			wantErr: errFake,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository()
			for _, d := range all {
				repo.devices[d.ID] = d
			}
			repo.listErr = tc.listErr

			svc := NewService(Config{Repo: repo, Logger: discardLogger})
			got, err := svc.List(context.Background(), tc.filter, tc.page)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("List() error = %v, want %v", err, tc.wantErr)
				}
				if got != nil {
					t.Fatalf("List() = %+v, want nil on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}

			if got == nil {
				t.Fatal("List() returned a nil page")
			}
			if got.Devices == nil {
				t.Fatal("List() Devices = nil, want empty slice")
			}
			if gotIDs := ids(got.Devices); !slices.Equal(gotIDs, tc.wantIDs) {
				t.Fatalf("List() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
			if got.Cursor != tc.wantCursor {
				t.Fatalf("List() cursor = %v, want %v", got.Cursor, tc.wantCursor)
			}
			if got.HasNext() != (tc.wantCursor != uuid.Nil) {
				t.Fatalf("HasNext() = %v, want %v", got.HasNext(), tc.wantCursor != uuid.Nil)
			}
			if repo.lastPageRequest.Size != tc.wantSize {
				t.Fatalf("repository page size = %d, want %d", repo.lastPageRequest.Size, tc.wantSize)
			}
		})
	}
}
