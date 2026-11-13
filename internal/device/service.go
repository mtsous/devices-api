package device

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(
	repo Repository,
	logger *slog.Logger,
) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

func (s *Service) Create(ctx context.Context, device Device) (*Device, error) {
	device.Name = strings.TrimSpace(device.Name)
	device.Brand = strings.TrimSpace(device.Brand)

	if device.State == "" {
		device.State = StateInactive
	}
	if err := validate(device); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate device id: %w", err)
	}

	device.ID = id

	created, err := s.repo.Create(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}

	s.logger.InfoContext(ctx, "device created",
		"device_id", created.ID,
		"brand", created.Brand,
		"state", created.State,
	)

	return created, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Device, error) {
	device, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get device %s: %w", id, err)
	}

	return device, nil
}

func (s *Service) List(
	ctx context.Context,
	filter Filter,
	page PageRequest,
) (*Page, error) {
	filter.Brand = strings.TrimSpace(filter.Brand)
	if filter.State != "" && !filter.State.IsValid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidState, filter.State)
	}

	if !filter.HasSingleCriterion() {
		return nil, fmt.Errorf("%w: filter by brand or state, not both", ErrInvalidInput)
	}

	size := page.Size
	switch {
	case size <= 0:
		size = DefaultPageLimit
	case size > MaxPageLimit:
		size = MaxPageLimit
	}

	// Fetch one extra device to know whether a next page exists without a COUNT query.
	devices, err := s.repo.List(ctx, filter, PageRequest{Size: size + 1, Cursor: page.Cursor})
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	result := &Page{Devices: devices}
	if len(devices) > size {
		result.Devices = devices[:size]
		result.Cursor = devices[size-1].ID
	}
	if result.Devices == nil {
		result.Devices = []*Device{}
	}

	return result, nil
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	device Device,
) (*Device, error) {
	panic("not implemented")
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete device %s: %w", id, err)
	}

	s.logger.InfoContext(ctx, "device deleted", "device_id", id)

	return nil
}

func validate(device Device) error {
	if device.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if device.Brand == "" {
		return fmt.Errorf("%w: brand is required", ErrInvalidInput)
	}
	if !device.State.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidState, device.State)
	}
	return nil
}
