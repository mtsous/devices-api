package notify

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mtsous/devices-api/internal/device"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func newChange() device.StateChange {
	return device.StateChange{
		DeviceID: uuid.Must(uuid.NewV7()),
		Previous: device.StateAvailable,
		Current:  device.StateInUse,
	}
}

func receive(t *testing.T, ch <-chan device.StateChange) device.StateChange {
	t.Helper()
	select {
	case change, ok := <-ch:
		if !ok {
			t.Fatal("channel closed, want a state change")
		}
		return change
	case <-time.After(time.Second):
		t.Fatal("no state change received")
	}
	return device.StateChange{}
}

func assertClosed(t *testing.T, ch <-chan device.StateChange) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("received a state change, want the channel closed")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}

func TestBrokerDeliversToEverySubscriber(t *testing.T) {
	b := NewBroker(discardLogger)
	first, unsubscribeFirst := b.Subscribe()
	defer unsubscribeFirst()
	second, unsubscribeSecond := b.Subscribe()
	defer unsubscribeSecond()

	change := newChange()
	b.Notify(context.Background(), change)

	if got := receive(t, first); got != change {
		t.Fatalf("first subscriber got %+v, want %+v", got, change)
	}
	if got := receive(t, second); got != change {
		t.Fatalf("second subscriber got %+v, want %+v", got, change)
	}
}

func TestBrokerDoesNotBlockOnSlowSubscriber(t *testing.T) {
	b := NewBroker(discardLogger)
	_, unsubscribe := b.Subscribe()
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for range subscriberBuffer + 10 {
			b.Notify(context.Background(), newChange())
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Notify blocked on a subscriber that does not read")
	}
}

func TestBrokerUnsubscribe(t *testing.T) {
	b := NewBroker(discardLogger)
	ch, unsubscribe := b.Subscribe()

	unsubscribe()
	unsubscribe()

	assertClosed(t, ch)
	if n := b.subscriberCount(); n != 0 {
		t.Fatalf("subscribers = %d, want 0", n)
	}
	b.Notify(context.Background(), newChange())
}

func TestBrokerClose(t *testing.T) {
	b := NewBroker(discardLogger)
	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	b.Close()

	assertClosed(t, ch)
	late, _ := b.Subscribe()
	assertClosed(t, late)
}
