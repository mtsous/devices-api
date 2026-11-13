package device

import (
	"errors"
	"fmt"
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

func (d Device) String() string {
	formatTimePtr := func(t *time.Time) string {
		if t == nil {
			return "<nil>"
		}
		return t.String()
	}
	return fmt.Sprintf("{ID:%s Name:%q Brand:%q State:%q CreatedAt:%s UpdatedAt:%s DeletedAt:%s}",
		d.ID, d.Name, d.Brand, d.State, d.CreatedAt, formatTimePtr(d.UpdatedAt), formatTimePtr(d.DeletedAt))
}

var (
	ErrNotFound     = errors.New("device not found")
	ErrInUse        = errors.New("device is in use")
	ErrInvalidState = errors.New("invalid device state")
	ErrInvalidInput = errors.New("invalid device input")
)
