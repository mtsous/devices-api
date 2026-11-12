package device

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	Repo   Repository
	Logger *slog.Logger
}

type Service struct {
	repo   Repository
	logger *slog.Logger
}

func NewService(config Config) *Service {
	return &Service{
		repo:   config.Repo,
		logger: config.Logger,
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
	device.CreatedAt = time.Now().UTC()
	device.UpdatedAt = nil

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
	panic("not implemented")
}

func (s *Service) List(
	ctx context.Context,
	filter Filter,
	page PageRequest,
) (*Page, error) {
	panic("not implemented")
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	device Device,
) (*Device, error) {
	panic("not implemented")
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	panic("not implemented")
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
