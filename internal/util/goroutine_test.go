package util

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/golog"
)

func createTestLoggerForUtil(t *testing.T) *golog.Logger {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "util-test-logs-*")
	if err != nil {
		t.Fatalf("Failed to create temp log dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            tmpDir,
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	t.Cleanup(func() { logger.Close() })
	return logger
}

func TestSafeGo_NormalExecution(t *testing.T) {
	logger := createTestLoggerForUtil(t)

	var wg sync.WaitGroup
	wg.Add(1)

	executed := false
	SafeGo(logger, "test", func() {
		defer wg.Done()
		executed = true
	})

	wg.Wait()
	if !executed {
		t.Error("expected goroutine to execute")
	}
}

func TestSafeGo_PanicRecovery(t *testing.T) {
	logger := createTestLoggerForUtil(t)

	done := make(chan struct{})

	SafeGo(logger, "test-panic", func() {
		defer func() {
			close(done)
		}()
		panic("test panic")
	})

	select {
	case <-done:
		// Goroutine recovered from panic and completed
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for goroutine to recover from panic")
	}
}

func TestSafeGo_PanicDoesNotCrashProcess(t *testing.T) {
	logger := createTestLoggerForUtil(t)

	// Launch a goroutine that panics
	done := make(chan struct{})
	SafeGo(logger, "panicker", func() {
		defer func() { close(done) }()
		panic("intentional panic")
	})

	select {
	case <-done:
		// Success - process is still alive
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	// Launch another goroutine after the panic to prove the process survived
	done2 := make(chan struct{})
	SafeGo(logger, "survivor", func() {
		close(done2)
	})

	select {
	case <-done2:
		// Process survived
	case <-time.After(2 * time.Second):
		t.Fatal("process died after panic")
	}
}
