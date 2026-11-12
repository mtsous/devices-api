package device

import (
	"context"

	"github.com/google/uuid"
)

type Config struct {
	Repo Repository
}

type Service struct {
	repo Repository
}

func NewService(config Config) *Service {
	return &Service{
		repo: config.Repo,
	}
}

func (s *Service) Create(ctx context.Context, device Device) (Device, error) {
	panic("not implemented")
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Device, error) {
	panic("not implemented")
}

func (s *Service) List(
	ctx context.Context,
	filter Filter,
	page PageRequest,
) (Page, error) {
	panic("not implemented")
}

func (s *Service) Update(
	ctx context.Context,
	id uuid.UUID,
	device Device,
) (Device, error) {
	panic("not implemented")
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	panic("not implemented")
}
