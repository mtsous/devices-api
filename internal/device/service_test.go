package device

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

var errFake = errors.New("fake repository error")

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakeRepository struct {
	devices   map[uuid.UUID]Device
	createErr error
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

func (f *fakeRepository) Get(context.Context, uuid.UUID) (*Device, error) {
	return &Device{}, errors.New("not implemented")
}

func (f *fakeRepository) List(context.Context, Filter, PageRequest) ([]*Device, error) {
	return nil, errors.New("not implemented")
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
