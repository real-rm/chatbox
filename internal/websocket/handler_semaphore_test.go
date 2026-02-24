package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingRouter blocks until released, allowing tests to control goroutine concurrency.
type blockingRouter struct {
	mu            sync.Mutex
	routed        []*message.Message
	blockedCh     chan struct{} // closed when all slots are occupied
	releaseCh     chan struct{} // close to unblock all waiting goroutines
	maxSeen       atomic.Int32 // highest concurrent count seen
	currentActive atomic.Int32
	expectedBlock int32 // how many goroutines we expect to be blocked simultaneously
}

func newBlockingRouter(expectedBlock int32) *blockingRouter {
	return &blockingRouter{
		blockedCh:     make(chan struct{}),
		releaseCh:     make(chan struct{}),
		expectedBlock: expectedBlock,
	}
}

func (r *blockingRouter) RouteMessage(conn *Connection, msg *message.Message) error {
	current := r.currentActive.Add(1)
	defer r.currentActive.Add(-1)

	// Track max concurrency seen
	for {
		seen := r.maxSeen.Load()
		if current <= seen || r.maxSeen.CompareAndSwap(seen, current) {
			break
		}
	}

	// Signal when we've reached the expected concurrency level
	if current == r.expectedBlock {
		select {
		case <-r.blockedCh:
		default:
			close(r.blockedCh)
		}
	}

	// Block until released
	<-r.releaseCh

	r.mu.Lock()
	r.routed = append(r.routed, msg)
	r.mu.Unlock()
	return nil
}

func (r *blockingRouter) RegisterConnection(sessionID string, conn *Connection) error { return nil }
func (r *blockingRouter) UnregisterConnection(sessionID string)                       {}

// TestReadPump_ConcurrentMessagesSemaphore verifies that at most
// constants.MaxConcurrentMessagesPerConn RouteMessage goroutines can run
// concurrently per connection. Messages beyond that limit are dropped with an
// error response rather than spawning unbounded goroutines.
func TestReadPump_ConcurrentMessagesSemaphore(t *testing.T) {
	maxConc := int32(constants.MaxConcurrentMessagesPerConn)
	// We need maxConc+2 messages: maxConc to fill slots + 2 overflow messages.
	totalMessages := int(maxConc) + 2

	router := newBlockingRouter(maxConc)
	validator := auth.NewJWTValidator("test-secret-32-bytes-padding-ok!")
	handler := NewHandler(validator, router, testLogger(), 1048576)

	// Track error messages sent back to the client
	var errorResponseCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		serverConn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer serverConn.Close()

		// Send totalMessages messages in rapid succession.
		for i := 0; i < totalMessages; i++ {
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: "semaphore-test-session",
				Content:   "message",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}
			if err := serverConn.WriteJSON(msg); err != nil {
				return
			}
		}

		// Wait until we observe maxConc goroutines are active (all slots filled).
		select {
		case <-router.blockedCh:
		case <-time.After(3 * time.Second):
			t.Errorf("timeout: expected %d concurrent goroutines to be active", maxConc)
			return
		}

		// Now read and count any error responses the handler sent back.
		serverConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for {
			_, rawMsg, err := serverConn.ReadMessage()
			if err != nil {
				break
			}
			var resp message.Message
			if json.Unmarshal(rawMsg, &resp) == nil && resp.Type == message.TypeError {
				errorResponseCount.Add(1)
			}
		}

		// Release all blocked goroutines.
		close(router.releaseCh)

		// Give time for goroutines to complete.
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer wsConn.Close()

	sendCh := make(chan []byte, 256)
	conn := &Connection{
		conn:         wsConn,
		ConnectionID: "semaphore-test-conn",
		UserID:       "test-user-sem",
		send:         sendCh,
	}

	handler.registerConnection(conn)

	// Drain error responses concurrently. sendCh is closed by h.unregisterConnection
	// when readPump exits, so this goroutine exits naturally — we must not close sendCh ourselves.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for rawMsg := range sendCh {
			var resp message.Message
			if json.Unmarshal(rawMsg, &resp) == nil && resp.Type == message.TypeError {
				errorResponseCount.Add(1)
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		conn.readPump(handler)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("readPump did not finish")
	}

	// Wait for drain goroutine to finish (it exits once unregisterConnection closes sendCh).
	select {
	case <-drainDone:
	case <-time.After(2 * time.Second):
	}

	// The maximum concurrent goroutines must never exceed the semaphore limit.
	assert.LessOrEqual(t, router.maxSeen.Load(), maxConc,
		"concurrent RouteMessage calls must not exceed MaxConcurrentMessagesPerConn")

	// At least one overflow message should have been rejected (error response sent).
	assert.Greater(t, errorResponseCount.Load(), int32(0),
		"overflow messages should produce error responses")
}
