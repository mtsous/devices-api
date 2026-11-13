package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mtsous/devices-api/internal/device"
)

func newTestRepository(t *testing.T) *DeviceRepository {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE devices"); err != nil {
		t.Fatalf("truncate devices: %v", err)
	}
	return NewDeviceRepository(pool)
}

func TestDeviceRepositoryCreate(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	t.Run("stores device and sets created_at", func(t *testing.T) {
		old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		input := device.Device{
			ID:        uuid.Must(uuid.NewV7()),
			Name:      "iPhone",
			Brand:     "Apple",
			State:     device.StateAvailable,
			CreatedAt: old,
			UpdatedAt: &old,
			DeletedAt: &old,
		}

		got, err := repo.Create(ctx, input)
		if err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}

		if got.ID != input.ID || got.Name != input.Name || got.Brand != input.Brand || got.State != input.State {
			t.Fatalf("Create() = %s, want fields of %s", got, input)
		}
		if time.Since(got.CreatedAt).Abs() > time.Minute {
			t.Fatalf("CreatedAt = %v, want set by the database to about now", got.CreatedAt)
		}
		if got.UpdatedAt != nil || got.DeletedAt != nil {
			t.Fatalf("UpdatedAt = %v, DeletedAt = %v, want both nil", got.UpdatedAt, got.DeletedAt)
		}
	})

	t.Run("rejects duplicate id", func(t *testing.T) {
		input := device.Device{ID: uuid.Must(uuid.NewV7()), Name: "Pixel", Brand: "Google", State: device.StateInUse}
		if _, err := repo.Create(ctx, input); err != nil {
			t.Fatalf("first Create() unexpected error: %v", err)
		}

		if _, err := repo.Create(ctx, input); err == nil {
			t.Fatal("second Create() error = nil, want error")
		}
	})
}

func TestDeviceRepositoryGet(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	stored, err := repo.Create(ctx, device.Device{
		ID:    uuid.Must(uuid.NewV7()),
		Name:  "iPhone",
		Brand: "Apple",
		State: device.StateAvailable,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}

	deleted, err := repo.Create(ctx, device.Device{
		ID:    uuid.Must(uuid.NewV7()),
		Name:  "Pixel",
		Brand: "Google",
		State: device.StateInactive,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if _, err := repo.pool.Exec(
		ctx,
		"UPDATE devices SET deleted_at = now() WHERE id = $1",
		deleted.ID,
	); err != nil {
		t.Fatalf("soft delete device: %v", err)
	}

	t.Run("returns stored device", func(t *testing.T) {
		got, err := repo.Get(ctx, stored.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if got.ID != stored.ID ||
			got.Name != stored.Name ||
			got.Brand != stored.Brand ||
			got.State != stored.State ||
			!got.CreatedAt.Equal(stored.CreatedAt) {
			t.Fatalf("Get() = %s, want %s", got, stored)
		}
	})

	t.Run("returns not found for unknown id", func(t *testing.T) {
		got, err := repo.Get(ctx, uuid.Must(uuid.NewV7()))
		if !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Get() error = %v, want %v", err, device.ErrNotFound)
		}
		if got != nil {
			t.Fatalf("Get() = %s, want nil", got)
		}
	})

	t.Run("returns not found for deleted device", func(t *testing.T) {
		got, err := repo.Get(ctx, deleted.ID)
		if !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Get() error = %v, want %v", err, device.ErrNotFound)
		}
		if got != nil {
			t.Fatalf("Get() = %s, want nil", got)
		}
	})
}
