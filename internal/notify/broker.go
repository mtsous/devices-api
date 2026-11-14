package notify

import (
	"context"
	"log/slog"
	"sync"

	"github.com/mtsous/devices-api/internal/device"
)

const subscriberBuffer = 16

type Broker struct {
	logger *slog.Logger

	mu          sync.Mutex
	subscribers map[chan device.StateChange]struct{}
	closed      bool
}

var _ device.Notifier = (*Broker)(nil)

func NewBroker(logger *slog.Logger) *Broker {
	return &Broker{
		logger:      logger,
		subscribers: map[chan device.StateChange]struct{}{},
	}
}

func (b *Broker) Notify(ctx context.Context, change device.StateChange) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		select {
		case ch <- change:
		default:
			b.logger.WarnContext(ctx, "dropped state change for slow subscriber",
				"device_id", change.DeviceID)
		}
	}
}

func (b *Broker) Subscribe() (<-chan device.StateChange, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan device.StateChange, subscriberBuffer)
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subscribers[ch] = struct{}{}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}
	return ch, unsubscribe
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for ch := range b.subscribers {
		delete(b.subscribers, ch)
		close(ch)
	}
}

func (b *Broker) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}
