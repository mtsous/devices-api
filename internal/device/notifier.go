package device

import (
	"context"

	"github.com/google/uuid"
)

type StateChange struct {
	DeviceID uuid.UUID
	Previous State
	Current  State
}

type Notifier interface {
	Notify(ctx context.Context, change StateChange)
}
