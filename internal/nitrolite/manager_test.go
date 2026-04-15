package nitrolite

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
)

func TestManagerRunReconnectsAfterClientClosure(t *testing.T) {
	t.Parallel()

	firstWait := make(chan struct{})
	secondWait := make(chan struct{})
	firstClient := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return "0xabc" },
		WaitChFunc:         func() <-chan struct{} { return firstWait },
	}
	secondClient := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return "0xabc" },
		WaitChFunc:         func() <-chan struct{} { return secondWait },
	}

	manager := NewManagerWithClient(firstClient, Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: "0xabc",
	}, slog.Default())

	reconnectCalls := 0
	manager.reconnect = func(ctx context.Context) (Client, error) {
		reconnectCalls++
		if reconnectCalls == 1 {
			return nil, errors.New("temporary failure")
		}
		return secondClient, nil
	}
	manager.sleep = func(ctx context.Context, delay time.Duration) bool { return true }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go manager.Run(ctx)
	close(firstWait)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if manager.Client() == secondClient {
			health := manager.Health()
			if !health.Connected || !health.Ready {
				t.Fatalf("health = %#v, want connected/ready", health)
			}
			if reconnectCalls < 2 {
				t.Fatalf("reconnectCalls = %d, want at least 2", reconnectCalls)
			}
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("manager did not reconnect; client = %#v reconnectCalls=%d", manager.Client(), reconnectCalls)
}
