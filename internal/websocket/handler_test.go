package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger creates a test logger for unit tests
func testLogger() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            ".",     // Use current directory for test logs
		Level:          "error", // Only log errors in tests
		StandardOutput: true,    // Output to stdout
	})
	if err != nil {
		panic("failed to create test logger: " + err.Error())
	}
	return logger
}

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
			handler := NewHandler(validator, nil, testLogger())

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
	handler := NewHandler(validator, nil, testLogger())

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

	// Wait for a short time (we don't need to wait for a full ping cycle in tests)
	time.Sleep(200 * time.Millisecond)

	// Connection should still be alive
	handler.mu.RLock()
	_, exists := handler.connections[connection.UserID]
	handler.mu.RUnlock()
	assert.True(t, exists, "connection should still be registered after setup")

	// Clean up
	connection.Close()
}

// TestConnection_GracefulClose tests graceful connection closure
func TestConnection_GracefulClose(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger())

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
	handler := NewHandler(validator, nil, testLogger())

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

// TestHandler_CheckOrigin tests the origin validation functionality
func TestHandler_CheckOrigin(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		expectedResult bool
		description    string
	}{
		{
			name:           "no origins configured allows all",
			allowedOrigins: []string{},
			requestOrigin:  "https://example.com",
			expectedResult: true,
			description:    "When no origins are configured, all origins should be allowed (development mode)",
		},
		{
			name:           "exact match allowed",
			allowedOrigins: []string{"https://example.com", "https://app.example.com"},
			requestOrigin:  "https://example.com",
			expectedResult: true,
			description:    "Exact match of allowed origin should be accepted",
		},
		{
			name:           "second origin allowed",
			allowedOrigins: []string{"https://example.com", "https://app.example.com"},
			requestOrigin:  "https://app.example.com",
			expectedResult: true,
			description:    "Second configured origin should be accepted",
		},
		{
			name:           "disallowed origin rejected",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://evil.com",
			expectedResult: false,
			description:    "Origin not in allowed list should be rejected",
		},
		{
			name:           "subdomain not allowed by default",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://sub.example.com",
			expectedResult: false,
			description:    "Subdomain should not match parent domain",
		},
		{
			name:           "http vs https mismatch",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "http://example.com",
			expectedResult: false,
			description:    "Protocol mismatch should be rejected",
		},
		{
			name:           "port mismatch",
			allowedOrigins: []string{"https://example.com:443"},
			requestOrigin:  "https://example.com:8080",
			expectedResult: false,
			description:    "Port mismatch should be rejected",
		},
		{
			name:           "empty origin with restrictions",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "",
			expectedResult: false,
			description:    "Empty origin should be rejected when restrictions are configured",
		},
		{
			name:           "localhost allowed",
			allowedOrigins: []string{"http://localhost:3000"},
			requestOrigin:  "http://localhost:3000",
			expectedResult: true,
			description:    "Localhost with port should be allowed if configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := auth.NewJWTValidator("test-secret")
			handler := NewHandler(validator, nil, testLogger())

			// Configure allowed origins
			if len(tt.allowedOrigins) > 0 {
				handler.SetAllowedOrigins(tt.allowedOrigins)
			}

			// Create a mock request with the origin header
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Origin", tt.requestOrigin)

			// Test the checkOrigin method
			result := handler.checkOrigin(req)

			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

// TestHandler_SetAllowedOrigins tests the SetAllowedOrigins configuration method
func TestHandler_SetAllowedOrigins(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger())

	// Test setting origins
	origins := []string{"https://example.com", "https://app.example.com"}
	handler.SetAllowedOrigins(origins)

	// Verify origins are set correctly
	handler.mu.RLock()
	assert.Equal(t, 2, len(handler.allowedOrigins))
	assert.True(t, handler.allowedOrigins["https://example.com"])
	assert.True(t, handler.allowedOrigins["https://app.example.com"])
	handler.mu.RUnlock()

	// Test updating origins
	newOrigins := []string{"https://newsite.com"}
	handler.SetAllowedOrigins(newOrigins)

	// Verify old origins are replaced
	handler.mu.RLock()
	assert.Equal(t, 1, len(handler.allowedOrigins))
	assert.True(t, handler.allowedOrigins["https://newsite.com"])
	assert.False(t, handler.allowedOrigins["https://example.com"])
	handler.mu.RUnlock()

	// Test setting empty origins
	handler.SetAllowedOrigins([]string{})

	// Verify origins are cleared
	handler.mu.RLock()
	assert.Equal(t, 0, len(handler.allowedOrigins))
	handler.mu.RUnlock()
}

// TestHandler_OriginValidationIntegration tests origin validation during WebSocket upgrade
func TestHandler_OriginValidationIntegration(t *testing.T) {
	// Helper function to create a valid JWT token for testing
	createTestToken := func(userID string, roles []string, secret string) string {
		claims := jwt.MapClaims{
			"user_id": userID,
			"roles":   roles,
			"exp":     time.Now().Add(time.Hour).Unix(),
			"iat":     time.Now().Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, _ := token.SignedString([]byte(secret))
		return tokenString
	}

	// Create a valid JWT token
	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	token := createTestToken("test-user", []string{"user"}, testSecret)

	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		expectUpgrade  bool
		description    string
	}{
		{
			name:           "allowed origin upgrades successfully",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://example.com",
			expectUpgrade:  true,
			description:    "WebSocket upgrade should succeed with allowed origin",
		},
		{
			name:           "disallowed origin fails upgrade",
			allowedOrigins: []string{"https://example.com"},
			requestOrigin:  "https://evil.com",
			expectUpgrade:  false,
			description:    "WebSocket upgrade should fail with disallowed origin",
		},
		{
			name:           "no restrictions allows upgrade",
			allowedOrigins: []string{},
			requestOrigin:  "https://any-origin.com",
			expectUpgrade:  true,
			description:    "WebSocket upgrade should succeed when no restrictions configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(validator, nil, testLogger())

			// Configure allowed origins
			if len(tt.allowedOrigins) > 0 {
				handler.SetAllowedOrigins(tt.allowedOrigins)
			}

			// Create a test server with the handler
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.HandleWebSocket(w, r)
			}))
			defer server.Close()

			// Create WebSocket URL with token
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

			// Create dialer with origin header
			dialer := websocket.Dialer{}
			headers := http.Header{}
			headers.Set("Origin", tt.requestOrigin)

			// Attempt to connect
			conn, resp, err := dialer.Dial(wsURL, headers)

			if tt.expectUpgrade {
				// Should succeed
				assert.NoError(t, err, tt.description)
				if conn != nil {
					conn.Close()
				}
			} else {
				// Should fail with 403 Forbidden
				assert.Error(t, err, tt.description)
				if resp != nil {
					assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Expected 403 Forbidden for disallowed origin")
				}
			}
		})
	}
}
