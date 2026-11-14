package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func startStream(t *testing.T) (*Broker, *bufio.Reader, func()) {
	t.Helper()
	broker := NewBroker(discardLogger)
	mux := http.NewServeMux()
	NewHandler(broker, discardLogger).RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/devices/events", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	waitFor(t, func() bool { return broker.subscriberCount() == 1 }, "client to subscribe")
	return broker, bufio.NewReader(resp.Body), cancel
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	var name, data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		switch {
		case line == "":
			if name != "" || data != "" {
				return name, data
			}
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestHandlerStreamsStateChanges(t *testing.T) {
	broker, stream, _ := startStream(t)

	change := newChange()
	broker.Notify(context.Background(), change)

	name, data := readEvent(t, stream)
	if name != "state-changed" {
		t.Fatalf("event = %q, want state-changed", name)
	}
	var got stateChangeEvent
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatalf("decode event data %q: %v", data, err)
	}
	want := stateChangeEvent{
		DeviceID:      change.DeviceID.String(),
		PreviousState: change.Previous,
		NewState:      change.Current,
	}
	if got != want {
		t.Fatalf("event data = %+v, want %+v", got, want)
	}
}

func TestHandlerUnsubscribesOnDisconnect(t *testing.T) {
	broker, _, disconnect := startStream(t)

	disconnect()

	waitFor(t, func() bool { return broker.subscriberCount() == 0 }, "subscriber to be removed")
}

func TestHandlerEndsStreamWhenBrokerCloses(t *testing.T) {
	broker, stream, _ := startStream(t)

	broker.Close()

	done := make(chan error, 1)
	go func() {
		_, err := stream.ReadString('\n')
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stream still open after broker closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after broker closed")
	}
}
