package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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

func (r *DeviceRepository) Get(context.Context, uuid.UUID) (*device.Device, error) {
	return nil, errors.New("not implemented")
}

func (r *DeviceRepository) List(context.Context, device.Filter, device.PageRequest) ([]*device.Device, error) {
	return nil, errors.New("not implemented")
}

func (r *DeviceRepository) Update(context.Context, device.Device) error {
	return errors.New("not implemented")
}

func (r *DeviceRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
