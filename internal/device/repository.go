package device

import (
	"context"

	"github.com/google/uuid"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100
)

type Filter struct {
	Brand string
	State State
}

func (f Filter) HasSingleCriterion() bool {
	return f.Brand == "" || f.State == ""
}

type PageRequest struct {
	Size   int
	Cursor uuid.UUID
}

type Page struct {
	Devices []*Device
	Cursor  uuid.UUID
}

func (p Page) HasNext() bool {
	return p.Cursor != uuid.Nil
}

type Repository interface {
	Create(ctx context.Context, device Device) (*Device, error)
	Get(ctx context.Context, id uuid.UUID) (*Device, error)
	List(
		ctx context.Context,
		filter Filter,
		pageRequest PageRequest,
	) ([]*Device, error)
	Update(ctx context.Context, device Device) error
	Delete(ctx context.Context, id uuid.UUID) error
}
