package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/storage"
)

func TestServer_RuntimeCompactLoopLifecycle(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer store.Close()

	s := &Server{
		storage: store,
	}
	s.startRuntimeCompactLoop()

	done := make(chan struct{})
	go func() {
		s.StopBackgroundTasks()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StopBackgroundTasks did not return")
	}
}

func TestServer_MaybeCompactRuntimeDBSkipsSmallFile(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	defer store.Close()

	s := &Server{storage: store}
	s.maybeCompactRuntimeDB()

	runtimePath := filepath.Join(dir, "runtime.db")
	info, err := os.Stat(runtimePath)
	if err != nil {
		t.Fatalf("stat runtime db: %v", err)
	}
	if info.Size() >= runtimeCompactMinDBBytes {
		t.Fatalf("expected small runtime db, got %d bytes", info.Size())
	}
}
