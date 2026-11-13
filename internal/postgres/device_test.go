package postgres

import (
	"context"
	"errors"
	"os"
	"slices"
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

func createTestDevice(t *testing.T, repo *DeviceRepository, state device.State) *device.Device {
	t.Helper()
	d, err := repo.Create(context.Background(), device.Device{
		ID:    uuid.Must(uuid.NewV7()),
		Name:  "iPhone",
		Brand: "Apple",
		State: state,
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	return d
}

// waitUntilBlockedBy waits until some session is waiting on a lock held by
// blockerPID. It fails if done receives first, meaning the operation finished
// without waiting for the lock.
func waitUntilBlockedBy(t *testing.T, pool *pgxpool.Pool, blockerPID int, done <-chan error) {
	t.Helper()
	const query = `SELECT EXISTS (
		SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
	)`

	deadline := time.After(5 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("operation returned %v without waiting for the lock", err)
		case <-deadline:
			t.Fatal("no session became blocked by the other transaction")
		default:
		}

		var blocked bool
		err := pool.
			QueryRow(context.Background(), query, blockerPID).
			Scan(&blocked)
		if err != nil {
			t.Fatalf("check blocked sessions: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
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

	stored := createTestDevice(t, repo, device.StateAvailable)
	deleted := createTestDevice(t, repo, device.StateInactive)
	if err := repo.Delete(ctx, deleted.ID); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
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

func TestDeviceRepositoryDelete(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	t.Run("deletes available device", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateAvailable)

		if err := repo.Delete(ctx, d.ID); err != nil {
			t.Fatalf("Delete() unexpected error: %v", err)
		}
		if _, err := repo.Get(ctx, d.ID); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Get() after delete error = %v, want %v", err, device.ErrNotFound)
		}
	})

	t.Run("returns not found for already deleted device", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateInactive)

		if err := repo.Delete(ctx, d.ID); err != nil {
			t.Fatalf("first Delete() unexpected error: %v", err)
		}

		if err := repo.Delete(ctx, d.ID); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("second Delete() error = %v, want %v", err, device.ErrNotFound)
		}
	})

	t.Run("rejects in-use device", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateInUse)

		if err := repo.Delete(ctx, d.ID); !errors.Is(err, device.ErrInUse) {
			t.Fatalf("Delete() error = %v, want %v", err, device.ErrInUse)
		}
		if _, err := repo.Get(ctx, d.ID); err != nil {
			t.Fatalf("Get() after rejected delete error = %v, want device to still exist", err)
		}
	})

	t.Run("returns not found for unknown id", func(t *testing.T) {
		if err := repo.Delete(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Delete() error = %v, want %v", err, device.ErrNotFound)
		}
	})

	t.Run("waits for a concurrent state change", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateAvailable)

		// Another transaction locks the device and puts it in use, but has not committed yet.
		other, err := repo.pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin other transaction: %v", err)
		}
		defer other.Rollback(ctx)

		var otherPID int
		if err := other.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&otherPID); err != nil {
			t.Fatalf("get other transaction pid: %v", err)
		}
		if _, err := other.Exec(ctx, `UPDATE devices SET state = 'in-use' WHERE id = $1`, d.ID); err != nil {
			t.Fatalf("update in other transaction: %v", err)
		}

		done := make(chan error, 1)
		go func() { done <- repo.Delete(ctx, d.ID) }()

		waitUntilBlockedBy(t, repo.pool, otherPID, done)

		if err := other.Commit(ctx); err != nil {
			t.Fatalf("commit other transaction: %v", err)
		}

		select {
		case err := <-done:
			if !errors.Is(err, device.ErrInUse) {
				t.Fatalf("Delete() error = %v, want %v", err, device.ErrInUse)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Delete() did not return after the lock was released")
		}
	})
}

func TestDeviceRepositoryUpdate(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	t.Run("saves changes and sets updated_at", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateAvailable)

		got, err := repo.Update(ctx, d.ID, func(dd *device.Device) error {
			dd.Name = "Galaxy"
			dd.Brand = "Samsung"
			dd.State = device.StateInUse
			dd.CreatedAt = dd.CreatedAt.AddDate(-10, 0, 0) // must not be written
			return nil
		})
		if err != nil {
			t.Fatalf("Update() unexpected error: %v", err)
		}

		if got.Name != "Galaxy" || got.Brand != "Samsung" || got.State != device.StateInUse {
			t.Fatalf("Update() = %s, want name Galaxy, brand Samsung, state in-use", got)
		}
		if !got.CreatedAt.Equal(d.CreatedAt) {
			t.Fatalf("CreatedAt = %v, want unchanged %v", got.CreatedAt, d.CreatedAt)
		}
		if got.UpdatedAt == nil || !got.UpdatedAt.After(d.CreatedAt) {
			t.Fatalf("UpdatedAt = %v, want set after CreatedAt %v", got.UpdatedAt, d.CreatedAt)
		}

		stored, err := repo.Get(ctx, d.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if stored.Name != got.Name || stored.Brand != got.Brand || stored.State != got.State ||
			!stored.CreatedAt.Equal(d.CreatedAt) || !stored.UpdatedAt.Equal(*got.UpdatedAt) {
			t.Fatalf("stored = %s, want %s", stored, got)
		}
	})

	t.Run("does not save when fn returns an error", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateInUse)
		errRejected := errors.New("rejected")

		got, err := repo.Update(ctx, d.ID, func(x *device.Device) error {
			x.Name = "Galaxy"
			return errRejected
		})
		if !errors.Is(err, errRejected) {
			t.Fatalf("Update() error = %v, want %v", err, errRejected)
		}
		if got != nil {
			t.Fatalf("Update() = %s, want nil on error", got)
		}

		stored, err := repo.Get(ctx, d.ID)
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if stored.Name != "iPhone" || stored.UpdatedAt != nil {
			t.Fatalf("stored = %s, want unchanged", stored)
		}
	})

	t.Run("returns not found for unknown id", func(t *testing.T) {
		called := false
		_, err := repo.Update(ctx, uuid.Must(uuid.NewV7()), func(*device.Device) error {
			called = true
			return nil
		})
		if !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Update() error = %v, want %v", err, device.ErrNotFound)
		}
		if called {
			t.Fatal("fn called for unknown id")
		}
	})

	t.Run("returns not found for deleted device", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateAvailable)
		if err := repo.Delete(ctx, d.ID); err != nil {
			t.Fatalf("Delete() unexpected error: %v", err)
		}

		_, err := repo.Update(ctx, d.ID, func(*device.Device) error { return nil })
		if !errors.Is(err, device.ErrNotFound) {
			t.Fatalf("Update() error = %v, want %v", err, device.ErrNotFound)
		}
	})

	t.Run("waits for a concurrent state change", func(t *testing.T) {
		d := createTestDevice(t, repo, device.StateAvailable)

		// Another transaction puts the device in use, but has not committed yet.
		other, err := repo.pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin other transaction: %v", err)
		}
		defer other.Rollback(ctx)

		var otherPID int
		if err := other.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&otherPID); err != nil {
			t.Fatalf("get other transaction pid: %v", err)
		}
		if _, err := other.Exec(ctx, `UPDATE devices SET state = 'in-use' WHERE id = $1`, d.ID); err != nil {
			t.Fatalf("update in other transaction: %v", err)
		}

		var seenState device.State
		done := make(chan error, 1)
		go func() {
			_, err := repo.Update(ctx, d.ID, func(x *device.Device) error {
				seenState = x.State
				return nil
			})
			done <- err
		}()

		waitUntilBlockedBy(t, repo.pool, otherPID, done)

		if err := other.Commit(ctx); err != nil {
			t.Fatalf("commit other transaction: %v", err)
		}

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Update() unexpected error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Update() did not return after the lock was released")
		}
		if seenState != device.StateInUse {
			t.Fatalf("fn saw state %q, want %q committed by the other transaction", seenState, device.StateInUse)
		}
	})
}

func TestDeviceRepositoryList(t *testing.T) {
	repo := newTestRepository(t)
	ctx := context.Background()

	create := func(brand string, state device.State) uuid.UUID {
		t.Helper()
		d, err := repo.Create(ctx, device.Device{
			ID:    uuid.Must(uuid.NewV7()),
			Name:  "device",
			Brand: brand,
			State: state,
		})
		if err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
		return d.ID
	}

	// UUIDv7 ids are created in increasing order, which is also the list order.
	apple1 := create("Apple", device.StateAvailable)
	samsung1 := create("Samsung", device.StateInUse)
	apple2 := create("Apple", device.StateInUse)
	deleted := create("Apple", device.StateAvailable)
	samsung2 := create("Samsung", device.StateInactive)
	if err := repo.Delete(ctx, deleted); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	tests := []struct {
		name    string
		filter  device.Filter
		page    device.PageRequest
		wantIDs []uuid.UUID
	}{
		{
			name:    "lists active devices in id order",
			page:    device.PageRequest{Size: 10},
			wantIDs: []uuid.UUID{apple1, samsung1, apple2, samsung2},
		},
		{
			name:    "filters by brand",
			filter:  device.Filter{Brand: "Apple"},
			page:    device.PageRequest{Size: 10},
			wantIDs: []uuid.UUID{apple1, apple2},
		},
		{
			name:    "filters by state",
			filter:  device.Filter{State: device.StateInUse},
			page:    device.PageRequest{Size: 10},
			wantIDs: []uuid.UUID{samsung1, apple2},
		},
		{
			name:    "limits page size",
			page:    device.PageRequest{Size: 2},
			wantIDs: []uuid.UUID{apple1, samsung1},
		},
		{
			name:    "continues after cursor",
			page:    device.PageRequest{Size: 2, Cursor: samsung1},
			wantIDs: []uuid.UUID{apple2, samsung2},
		},
		{
			name:    "combines filter and cursor",
			filter:  device.Filter{Brand: "Samsung"},
			page:    device.PageRequest{Size: 10, Cursor: samsung1},
			wantIDs: []uuid.UUID{samsung2},
		},
		{
			name:    "returns nothing past the last device",
			page:    device.PageRequest{Size: 10, Cursor: samsung2},
			wantIDs: nil,
		},
		{
			name:    "returns nothing for unknown brand",
			filter:  device.Filter{Brand: "Nokia"},
			page:    device.PageRequest{Size: 10},
			wantIDs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.List(ctx, tc.filter, tc.page)
			if err != nil {
				t.Fatalf("List() unexpected error: %v", err)
			}

			var gotIDs []uuid.UUID
			for _, d := range got {
				gotIDs = append(gotIDs, d.ID)
				if d.DeletedAt != nil {
					t.Fatalf("List() returned deleted device %s", d)
				}
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Fatalf("List() ids = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}
