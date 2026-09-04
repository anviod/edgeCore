package modbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
)

func TestConnectionManager_ConnectingPhaseUsesBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()

	cm.SetState(driver.StateConnecting)
	cm.RecordFailure()

	_, wait := cm.CanRetry()
	if wait <= 0 {
		t.Fatalf("expected positive backoff during connecting phase after failure, got %v", wait)
	}
}

func TestConnectionManager_ScheduleReconnectSingleFlight(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()

	var attempts atomic.Int32
	block := make(chan struct{})

	connect := func(ctx context.Context) error {
		attempts.Add(1)
		<-block
		return ctx.Err()
	}

	cm.ScheduleReconnect(context.Background(), time.Second, connect)
	cm.ScheduleReconnect(context.Background(), time.Second, connect)
	cm.ScheduleReconnect(context.Background(), time.Second, connect)

	deadline := time.After(2 * time.Second)
	for attempts.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("expected scheduled reconnect to start")
		case <-time.After(5 * time.Millisecond):
		}
	}

	if attempts.Load() != 1 {
		t.Fatalf("expected single-flight reconnect, got %d attempts", attempts.Load())
	}

	close(block)
	time.Sleep(20 * time.Millisecond)
}

func TestConnectionManager_EnsureConnectedRetriesWithBackoff(t *testing.T) {
	cm := driver.NewConnectionManager("modbus")
	defer cm.Close()
	cm.SetBackoffParams(10*time.Millisecond, 50*time.Millisecond, 2.0)

	var attempts atomic.Int32
	start := time.Now()

	err := cm.EnsureConnected(context.Background(), func(ctx context.Context) error {
		if attempts.Add(1) < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EnsureConnected failed: %v", err)
	}

	if attempts.Load() != 3 {
		t.Fatalf("expected 3 dial attempts, got %d", attempts.Load())
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("expected backoff delay between attempts, got %v", elapsed)
	}
}
