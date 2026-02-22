package websocket

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSessionID_ThreadSafe(t *testing.T) {
	conn := NewConnection("user-1", []string{"user"})

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrently write SessionID
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			conn.mu.Lock()
			conn.SessionID = "session-abc"
			conn.mu.Unlock()
		}(i)
	}

	// Concurrently read via GetSessionID
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			sid := conn.GetSessionID()
			// Should be either empty (not yet written) or the correct value
			if sid != "" {
				assert.Equal(t, "session-abc", sid)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, "session-abc", conn.GetSessionID())
}

func TestGetSessionID_InitiallyEmpty(t *testing.T) {
	conn := NewConnection("user-1", []string{"user"})
	assert.Equal(t, "", conn.GetSessionID())
}

func TestSessionID_SetUnderLock(t *testing.T) {
	conn := NewConnection("user-1", []string{"user"})

	// Simulate the readPump pattern: check-then-set under lock
	conn.mu.Lock()
	if conn.SessionID == "" {
		conn.SessionID = "session-xyz"
	}
	conn.mu.Unlock()

	assert.Equal(t, "session-xyz", conn.GetSessionID())

	// Second set should be a no-op
	conn.mu.Lock()
	if conn.SessionID == "" {
		conn.SessionID = "session-second"
	}
	conn.mu.Unlock()

	assert.Equal(t, "session-xyz", conn.GetSessionID(), "SessionID should not be overwritten once set")
}
