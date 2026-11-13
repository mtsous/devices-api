package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mtsous/devices-api/internal/device"
)

type DeviceRepository struct {
	pool *pgxpool.Pool
}

func NewDeviceRepository(pool *pgxpool.Pool) *DeviceRepository {
	return &DeviceRepository{pool: pool}
}

func (r *DeviceRepository) Create(ctx context.Context, d device.Device) (*device.Device, error) {
	const query = `
		INSERT INTO devices (id, name, brand, state)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, brand, state, created_at, updated_at, deleted_at`

	var created device.Device
	err := r.pool.QueryRow(
		ctx,
		query,
		d.ID,
		d.Name,
		d.Brand,
		d.State,
	).Scan(
		&created.ID,
		&created.Name,
		&created.Brand,
		&created.State,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.DeletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert device: %w", err)
	}
	return &created, nil
}

func (r *DeviceRepository) Get(ctx context.Context, id uuid.UUID) (*device.Device, error) {
	const query = `
		SELECT id, name, brand, state, created_at, updated_at, deleted_at
		FROM devices
		WHERE id = $1 AND deleted_at IS NULL`

	var found device.Device
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&found.ID,
		&found.Name,
		&found.Brand,
		&found.State,
		&found.CreatedAt,
		&found.UpdatedAt,
		&found.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, device.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select device: %w", err)
	}
	return &found, nil
}

func (r *DeviceRepository) List(context.Context, device.Filter, device.PageRequest) ([]*device.Device, error) {
	return nil, errors.New("not implemented")
}

func (r *DeviceRepository) Update(context.Context, uuid.UUID, func(d *device.Device) error) (*device.Device, error) {
	return nil, errors.New("not implemented")
}

func (r *DeviceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var state device.State
	err = tx.QueryRow(ctx, `
		SELECT state
		FROM devices
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE`, id,
	).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return device.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("select device for update: %w", err)
	}
	if state == device.StateInUse {
		return device.ErrInUse
	}

	if _, err := tx.Exec(ctx, `UPDATE devices SET deleted_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("soft delete device: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
