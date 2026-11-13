package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mtsous/devices-api/internal/health"
	"github.com/mtsous/devices-api/internal/router"
)

func TestServeShutsDownGracefully(t *testing.T) {
	port := freePort(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: router.New(health.NewHandler(logger)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, logger) }()

	waitForHealthy(t, "http://localhost:"+strconv.Itoa(port)+"/healthz")

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after context cancellation")
	}
}

func TestRunFailsOnInvalidConfig(t *testing.T) {
	t.Setenv("PORT", "invalid")

	if err := run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealthy(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become healthy", url)
}
