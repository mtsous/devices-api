package device

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type State string

const (
	StateAvailable State = "available"
	StateInUse     State = "in-use"
	StateInactive  State = "inactive"
)

func (s State) IsValid() bool {
	switch s {
	case StateAvailable, StateInUse, StateInactive:
		return true
	default:
		return false
	}
}

type Device struct {
	ID        uuid.UUID
	Name      string
	Brand     string
	State     State
	CreatedAt time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time
}

var (
	ErrNotFound     = errors.New("device not found")
	ErrInUse        = errors.New("device is in use")
	ErrInvalidState = errors.New("invalid device state")
	ErrInvalidInput = errors.New("invalid device input")
)
