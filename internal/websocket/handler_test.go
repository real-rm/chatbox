package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/chat-websocket/internal/auth"
)

// TestConnection_WritePump tests the writePump functionality
func TestConnection_WritePump(t *testing.T) {
	tests := []struct {
		name        string
		messages    [][]byte
		expectClose bool
	}{
		{
			name:        "sends messages from channel",
			messages:    [][]byte{[]byte("hello"), []byte("world")},
			expectClose: false,
		},
		{
			name:        "handles channel close",
			messages:    [][]byte{},
			expectClose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				require.NoError(t, err)
				defer conn.Close()

				// Read messages sent by writePump
				for i := 0; i < len(tt.messages); i++ {
					_, msg, err := conn.ReadMessage()
					if err != nil {
						return
					}
					assert.Contains(t, string(msg), string(tt.messages[i]))
				}

				// Wait for close message if expected
				if tt.expectClose {
					_, _, err := conn.ReadMessage()
					assert.Error(t, err)
				}
			}))
			defer server.Close()

			// Connect to test server
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			require.NoError(t, err)

			// Create connection
			connection := &Connection{
				conn:   conn,
				UserID: "test-user",
				send:   make(chan []byte, 256),
			}

			// Start writePump
			go connection.writePump()

			// Send messages
			for _, msg := range tt.messages {
				connection.send <- msg
			}

			// Close channel if testing close behavior
			if tt.expectClose {
				close(connection.send)
			}

			// Give time for messages to be sent
			time.Sleep(100 * time.Millisecond)
		})
	}
}

// TestConnection_ReadPump tests the readPump functionality
func TestConnection_ReadPump(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
	}{
		{
			name:     "receives messages",
			messages: []string{"hello", "world"},
		},
		{
			name:     "handles empty messages",
			messages: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := auth.NewJWTValidator("test-secret")
			handler := NewHandler(validator)

			// Create a test server that will act as the client
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				require.NoError(t, err)
				defer conn.Close()

				// Send test messages
				for _, msg := range tt.messages {
					err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
					require.NoError(t, err)
				}

				// Close connection
				time.Sleep(100 * time.Millisecond)
			}))
			defer server.Close()

			// Connect to test server
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			require.NoError(t, err)

			// Create connection
			connection := &Connection{
				conn:   conn,
				UserID: "test-user",
				send:   make(chan []byte, 256),
			}

			// Register connection
			handler.registerConnection(connection)

			// Start readPump (will block until connection closes)
			done := make(chan bool)
			go func() {
				connection.readPump(handler)
				done <- true
			}()

			// Wait for readPump to finish
			select {
			case <-done:
				// Success
			case <-time.After(2 * time.Second):
				t.Fatal("readPump did not finish in time")
			}

			// Verify connection was unregistered
			handler.mu.RLock()
			_, exists := handler.connections[connection.UserID]
			handler.mu.RUnlock()
			assert.False(t, exists, "connection should be unregistered")
		})
	}
}

// TestConnection_PingPong tests the ping/pong heartbeat mechanism
func TestConnection_PingPong(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator)

	// Create a test server that responds to pings
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Set up ping handler (gorilla/websocket automatically responds to pings with pongs)
		conn.SetPingHandler(func(appData string) error {
			return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
		})

		// Keep connection alive
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start pumps
	go connection.readPump(handler)
	go connection.writePump()

	// Wait for at least one ping cycle
	time.Sleep(pingPeriod + 100*time.Millisecond)

	// Connection should still be alive
	handler.mu.RLock()
	_, exists := handler.connections[connection.UserID]
	handler.mu.RUnlock()
	assert.True(t, exists, "connection should still be registered after ping/pong")

	// Clean up
	connection.Close()
}

// TestConnection_GracefulClose tests graceful connection closure
func TestConnection_GracefulClose(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Wait for close
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Verify connection is registered
	handler.mu.RLock()
	_, exists := handler.connections[connection.UserID]
	handler.mu.RUnlock()
	assert.True(t, exists, "connection should be registered")

	// Close connection gracefully
	err = connection.Close()
	assert.NoError(t, err)

	// Unregister connection
	handler.unregisterConnection(connection)

	// Verify connection is unregistered
	handler.mu.RLock()
	_, exists = handler.connections[connection.UserID]
	handler.mu.RUnlock()
	assert.False(t, exists, "connection should be unregistered after close")

	// Verify send channel is closed
	_, ok := <-connection.send
	assert.False(t, ok, "send channel should be closed")
}

// TestConnection_ResourceCleanup tests that resources are cleaned up properly
func TestConnection_ResourceCleanup(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		
		// Close immediately to trigger cleanup
		conn.Close()
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:   conn,
		UserID: "test-user",
		send:   make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump (will exit immediately due to closed connection)
	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	// Wait for cleanup
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not complete in time")
	}

	// Verify connection was unregistered
	handler.mu.RLock()
	_, exists := handler.connections[connection.UserID]
	handler.mu.RUnlock()
	assert.False(t, exists, "connection should be unregistered after cleanup")
}
