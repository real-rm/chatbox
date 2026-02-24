package router

import (
	"sync"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterConnection_OwnershipCheckIsAtomicWithRegistration verifies that
// the session ownership check and the connection registration happen under the
// same lock. This prevents a TOCTOU race where another goroutine could register
// a new connection for the same session between the ownership check and the
// actual registration.
//
// The test races two goroutines each trying to register connections for the
// same session. Only one should succeed after the session is owned; the other
// must be rejected with an auth error if it belongs to a different user.
func TestRegisterConnection_OwnershipCheckIsAtomicWithRegistration(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mr := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session owned by user1.
	sess, err := sm.CreateSession("user1")
	require.NoError(t, err)
	require.NotNil(t, sess)

	sessionID := sess.ID

	// user1's connection — should always succeed.
	conn1 := websocket.NewConnection("user1", []string{"user"})
	// user2's connection — should be rejected because the session belongs to user1.
	conn2 := websocket.NewConnection("user2", []string{"user"})

	const goroutines = 20
	var wg sync.WaitGroup
	errors := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		idx := i
		go func() {
			defer wg.Done()
			var conn *websocket.Connection
			if idx%2 == 0 {
				conn = conn1
			} else {
				conn = conn2
			}
			errors[idx] = mr.RegisterConnection(sessionID, conn)
		}()
	}
	wg.Wait()

	// All even-indexed goroutines (user1) must succeed.
	for i := 0; i < goroutines; i += 2 {
		assert.NoError(t, errors[i], "user1 (session owner) should always be allowed to register")
	}

	// All odd-indexed goroutines (user2) must be rejected.
	for i := 1; i < goroutines; i += 2 {
		assert.Error(t, errors[i], "user2 (non-owner) should be rejected")
	}
}
