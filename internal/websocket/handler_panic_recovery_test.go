package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panicOnFirstCallRouter panics on the first RouteMessage call, succeeds on the second.
// Used to verify that panic recovery in the dispatch goroutine works correctly.
type panicOnFirstCallRouter struct {
	mu          sync.Mutex
	callCount   atomic.Int32
	routed      []*message.Message
	recoveredCh chan struct{} // closed when second call succeeds
	closeOnce   sync.Once
}

func newPanicOnFirstCallRouter() *panicOnFirstCallRouter {
	return &panicOnFirstCallRouter{
		routed:      make([]*message.Message, 0),
		recoveredCh: make(chan struct{}),
	}
}

func (r *panicOnFirstCallRouter) RouteMessage(conn *Connection, msg *message.Message) error {
	n := r.callCount.Add(1)
	if n == 1 {
		// First call: panic to simulate a router bug
		panic("deliberate test panic in RouteMessage")
	}
	// Subsequent calls: record and signal success
	r.mu.Lock()
	r.routed = append(r.routed, msg)
	r.mu.Unlock()
	r.closeOnce.Do(func() { close(r.recoveredCh) })
	return nil
}

func (r *panicOnFirstCallRouter) RegisterConnection(sessionID string, conn *Connection) error {
	return nil
}

func (r *panicOnFirstCallRouter) UnregisterConnection(sessionID string) {}

// TestReadPump_PanicInRouteMessageIsRecovered verifies that a panic inside
// RouteMessage does not crash readPump or the whole process. After the panic
// is recovered, subsequent messages must still be processed normally.
func TestReadPump_PanicInRouteMessageIsRecovered(t *testing.T) {
	panicRouter := newPanicOnFirstCallRouter()
	validator := auth.NewJWTValidator("test-secret-32-bytes-padding-ok!")
	handler := NewHandler(validator, panicRouter, testLogger(), 1048576)

	// The test HTTP server plays the role of the remote peer.
	// It sends two messages then closes the connection.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		sendMsg := func(content string) {
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: "panic-test-session",
				Content:   content,
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}
			_ = conn.WriteJSON(msg)
		}

		// Message 1 — will cause RouteMessage to panic
		sendMsg("trigger-panic")
		// Give readPump time to dispatch the goroutine
		time.Sleep(150 * time.Millisecond)

		// Message 2 — should be processed normally after panic recovery
		sendMsg("after-panic")
		// Give readPump time to dispatch and recover
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer wsConn.Close()

	conn := &Connection{
		conn:         wsConn,
		ConnectionID: "panic-test-conn",
		UserID:       "test-user-panic",
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(conn)

	done := make(chan struct{})
	go func() {
		conn.readPump(handler)
		close(done)
	}()

	// The test passes only when the second message is processed after the panic recovery.
	select {
	case <-panicRouter.recoveredCh:
		// Panic was recovered and subsequent message was routed successfully.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: panic was not recovered or second message was not routed")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readPump did not finish after connection closed")
	}

	assert.Equal(t, int32(2), panicRouter.callCount.Load(),
		"RouteMessage should have been called twice (first panic, second success)")
	assert.Len(t, panicRouter.routed, 1,
		"exactly one message should be recorded in the router after recovery")
}
