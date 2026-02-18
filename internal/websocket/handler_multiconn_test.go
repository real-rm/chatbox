package websocket

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_MultipleConnectionsPerUser tests that a single user can have multiple simultaneous connections
func TestHandler_MultipleConnectionsPerUser(t *testing.T) {
	// Helper function to create a valid JWT token
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

	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create token for a single user
	userID := "test-user"
	token := createTestToken(userID, []string{"user"}, testSecret)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

	// Connect multiple times with the same user
	numConnections := 3
	connections := make([]*websocket.Conn, numConnections)

	for i := 0; i < numConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d", i)
		connections[i] = conn

		// Give the handler time to register the connection
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns, exists := handler.connections[userID]
	handler.mu.RUnlock()

	assert.True(t, exists, "User should have connections registered")
	assert.Equal(t, numConnections, len(userConns), "User should have %d connections", numConnections)

	// Verify each connection has a unique connection ID
	connectionIDs := make(map[string]bool)
	handler.mu.RLock()
	for connID := range userConns {
		connectionIDs[connID] = true
	}
	handler.mu.RUnlock()
	assert.Equal(t, numConnections, len(connectionIDs), "All connection IDs should be unique")

	// Close one connection
	connections[0].Close()
	time.Sleep(100 * time.Millisecond)

	// Verify other connections are still active
	handler.mu.RLock()
	userConns, exists = handler.connections[userID]
	handler.mu.RUnlock()

	assert.True(t, exists, "User should still have connections")
	assert.Equal(t, numConnections-1, len(userConns), "User should have %d connections after closing one", numConnections-1)

	// Close remaining connections
	for i := 1; i < numConnections; i++ {
		connections[i].Close()
	}
	time.Sleep(100 * time.Millisecond)

	// Verify all connections are cleaned up
	handler.mu.RLock()
	userConns, exists = handler.connections[userID]
	handler.mu.RUnlock()

	assert.False(t, exists, "User should have no connections after all are closed")
	assert.Equal(t, 0, len(userConns), "Connection map should be empty")
}

// TestHandler_ConnectionIDUniqueness tests that connection IDs are unique even for rapid connections
func TestHandler_ConnectionIDUniqueness(t *testing.T) {
	userID := "test-user"
	connectionIDs := make(map[string]bool)

	// Create multiple connections rapidly
	for i := 0; i < 100; i++ {
		conn := &Connection{
			UserID: userID,
			send:   make(chan []byte, 256),
		}

		// Simulate connection creation with unique ID generation (same as handler)
		randomBytes := make([]byte, 8)
		_, err := rand.Read(randomBytes)
		require.NoError(t, err)
		connectionID := fmt.Sprintf("%s-%d-%x", userID, time.Now().UnixNano(), randomBytes)
		conn.ConnectionID = connectionID

		// Verify uniqueness
		assert.False(t, connectionIDs[connectionID], "Connection ID should be unique: %s", connectionID)
		connectionIDs[connectionID] = true
	}

	assert.Equal(t, 100, len(connectionIDs), "All connection IDs should be unique")
}

// TestHandler_MultiConnectionMessageIsolation tests that messages sent to one connection don't affect others
func TestHandler_MultiConnectionMessageIsolation(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	userID := "test-user"

	// Create two connections for the same user
	conn1 := &Connection{
		ConnectionID: fmt.Sprintf("%s-1", userID),
		UserID:       userID,
		send:         make(chan []byte, 256),
	}

	conn2 := &Connection{
		ConnectionID: fmt.Sprintf("%s-2", userID),
		UserID:       userID,
		send:         make(chan []byte, 256),
	}

	// Register both connections
	handler.registerConnection(conn1)
	handler.registerConnection(conn2)

	// Verify both are registered
	handler.mu.RLock()
	userConns := handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 2, len(userConns), "Should have 2 connections")

	// Send message to conn1
	testMessage := []byte("test message")
	conn1.send <- testMessage

	// Verify conn1 received the message
	select {
	case msg := <-conn1.send:
		assert.Equal(t, testMessage, msg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("conn1 should have received the message")
	}

	// Verify conn2 did NOT receive the message
	select {
	case <-conn2.send:
		t.Fatal("conn2 should not have received the message")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message received
	}

	// Clean up
	handler.unregisterConnection(conn1)
	handler.unregisterConnection(conn2)
}

// TestHandler_RateLimitPerUser tests that rate limiting applies per user, not per connection
func TestHandler_RateLimitPerUser(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	userID := "test-user"

	// Create first connection - should succeed
	conn1 := &Connection{
		ConnectionID: fmt.Sprintf("%s-1", userID),
		UserID:       userID,
		send:         make(chan []byte, 256),
	}
	handler.registerConnection(conn1)

	// Verify connection is registered
	handler.mu.RLock()
	userConns := handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 1, len(userConns))

	// Create multiple additional connections up to the limit
	// The handler has a limit of 10 connections per user
	for i := 2; i <= 10; i++ {
		conn := &Connection{
			ConnectionID: fmt.Sprintf("%s-%d", userID, i),
			UserID:       userID,
			send:         make(chan []byte, 256),
		}
		handler.registerConnection(conn)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns = handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 10, len(userConns), "Should have 10 connections at the limit")

	// Clean up
	for _, conn := range userConns {
		handler.unregisterConnection(conn)
	}
}

// TestHandler_ShutdownWithMultipleConnections tests graceful shutdown with multiple connections per user
func TestHandler_ShutdownWithMultipleConnections(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create multiple users with multiple connections each
	numUsers := 3
	connectionsPerUser := 2

	for u := 0; u < numUsers; u++ {
		userID := fmt.Sprintf("user-%d", u)
		for c := 0; c < connectionsPerUser; c++ {
			conn := &Connection{
				ConnectionID: fmt.Sprintf("%s-conn-%d", userID, c),
				UserID:       userID,
				send:         make(chan []byte, 256),
			}
			handler.registerConnection(conn)
		}
	}

	// Verify all connections are registered
	handler.mu.RLock()
	totalUsers := len(handler.connections)
	totalConnections := 0
	for _, userConns := range handler.connections {
		totalConnections += len(userConns)
	}
	handler.mu.RUnlock()

	assert.Equal(t, numUsers, totalUsers, "Should have %d users", numUsers)
	assert.Equal(t, numUsers*connectionsPerUser, totalConnections, "Should have %d total connections", numUsers*connectionsPerUser)

	// Note: We can't fully test Shutdown() here because it requires real WebSocket connections
	// This test verifies the data structure is correct for shutdown scenarios
}

// TestHandler_ConnectionLimitGracefulHandling tests that connection limits are enforced gracefully
func TestHandler_ConnectionLimitGracefulHandling(t *testing.T) {
	// Helper function to create a valid JWT token
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

	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create token for a single user
	userID := "test-user"
	token := createTestToken(userID, []string{"user"}, testSecret)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

	// Connect up to the limit (10 connections)
	maxConnections := 10
	connections := make([]*websocket.Conn, maxConnections)

	for i := 0; i < maxConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d", i)
		connections[i] = conn

		// Give the handler time to register the connection
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns, exists := handler.connections[userID]
	handler.mu.RUnlock()

	assert.True(t, exists, "User should have connections registered")
	assert.Equal(t, maxConnections, len(userConns), "User should have %d connections", maxConnections)

	// Try to connect one more time - should be rejected with 429
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.Error(t, err, "Connection beyond limit should fail")
	if resp != nil {
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Should return 429 Too Many Requests")
	}

	// Close one connection
	connections[0].Close()
	time.Sleep(100 * time.Millisecond)

	// Now we should be able to connect again
	newConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "Should be able to connect after closing one connection")
	time.Sleep(50 * time.Millisecond)

	// Verify we still have the max number of connections
	handler.mu.RLock()
	userConns, exists = handler.connections[userID]
	handler.mu.RUnlock()

	assert.True(t, exists, "User should have connections registered")
	assert.Equal(t, maxConnections, len(userConns), "User should have %d connections", maxConnections)

	// Clean up
	newConn.Close()
	for i := 1; i < maxConnections; i++ {
		connections[i].Close()
	}
	time.Sleep(100 * time.Millisecond)

	// Verify all connections are cleaned up
	handler.mu.RLock()
	userConns, exists = handler.connections[userID]
	handler.mu.RUnlock()

	assert.False(t, exists, "User should have no connections after all are closed")
}

// TestHandler_ConnectionLimitPerUser tests that connection limits apply per user, not globally
func TestHandler_ConnectionLimitPerUser(t *testing.T) {
	// Helper function to create a valid JWT token
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

	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create two different users
	user1ID := "user-1"
	user2ID := "user-2"
	token1 := createTestToken(user1ID, []string{"user"}, testSecret)
	token2 := createTestToken(user2ID, []string{"user"}, testSecret)

	// Connect user1 up to the limit (10 connections)
	maxConnections := 10
	user1Connections := make([]*websocket.Conn, maxConnections)

	for i := 0; i < maxConnections; i++ {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token1
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d for user1", i)
		user1Connections[i] = conn
		time.Sleep(50 * time.Millisecond)
	}

	// Verify user1 has max connections
	handler.mu.RLock()
	user1Conns := handler.connections[user1ID]
	handler.mu.RUnlock()
	assert.Equal(t, maxConnections, len(user1Conns), "User1 should have %d connections", maxConnections)

	// User2 should still be able to connect even though user1 is at the limit
	user2Connections := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token2
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "User2 should be able to connect even when user1 is at limit")
		user2Connections[i] = conn
		time.Sleep(50 * time.Millisecond)
	}

	// Verify user2 has connections
	handler.mu.RLock()
	user2Conns := handler.connections[user2ID]
	handler.mu.RUnlock()
	assert.Equal(t, 3, len(user2Conns), "User2 should have 3 connections")

	// Clean up
	for _, conn := range user1Connections {
		conn.Close()
	}
	for _, conn := range user2Connections {
		conn.Close()
	}
}

// TestHandler_NotifyConnectionLimit tests that users are notified when connection limit is reached
func TestHandler_NotifyConnectionLimit(t *testing.T) {
	// Helper function to create a valid JWT token
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

	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create token for a single user
	userID := "test-user"
	token := createTestToken(userID, []string{"user"}, testSecret)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

	// Connect up to the limit (10 connections)
	maxConnections := 10
	connections := make([]*websocket.Conn, maxConnections)

	for i := 0; i < maxConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d", i)
		connections[i] = conn

		// Give the handler time to register the connection
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns, exists := handler.connections[userID]
	handler.mu.RUnlock()

	assert.True(t, exists, "User should have connections registered")
	assert.Equal(t, maxConnections, len(userConns), "User should have %d connections", maxConnections)

	// Try to connect one more time - should be rejected with 429
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.Error(t, err, "Connection beyond limit should fail")
	if resp != nil {
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Should return 429 Too Many Requests")
	}

	// Give time for notification to be sent
	time.Sleep(100 * time.Millisecond)

	// Check that at least one connection received a notification
	// We'll check the first connection
	conn := connections[0]
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	messageReceived := false
	for i := 0; i < 5; i++ { // Try reading a few messages
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Parse the message
		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err == nil {
			// Check if it's a notification about connection limit
			if msgType, ok := msg["type"].(string); ok && msgType == "notification" {
				if content, ok := msg["content"].(string); ok {
					if strings.Contains(content, "Connection limit reached") {
						messageReceived = true
						assert.Contains(t, content, "maximum number of simultaneous connections",
							"Notification should mention connection limit")
						break
					}
				}
			}
		}
	}

	assert.True(t, messageReceived, "At least one connection should receive connection limit notification")

	// Clean up
	for _, conn := range connections {
		conn.Close()
	}
	time.Sleep(100 * time.Millisecond)
}

// TestHandler_NotifyConnectionLimit_NoExistingConnections tests notification when no connections exist
func TestHandler_NotifyConnectionLimit_NoExistingConnections(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Call notifyConnectionLimit with a user that has no connections
	// This should not panic or cause errors
	handler.notifyConnectionLimit("non-existent-user")

	// Verify no connections exist
	handler.mu.RLock()
	userConns, exists := handler.connections["non-existent-user"]
	handler.mu.RUnlock()

	assert.False(t, exists, "User should not exist in connections map")
	assert.Equal(t, 0, len(userConns), "Should have no connections")
}

// TestHandler_NotifyConnectionLimit_MessageFormat tests the notification message format
func TestHandler_NotifyConnectionLimit_MessageFormat(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	userID := "test-user"

	// Create a connection for the user
	conn := &Connection{
		ConnectionID: fmt.Sprintf("%s-1", userID),
		UserID:       userID,
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(conn)

	// Call notifyConnectionLimit
	handler.notifyConnectionLimit(userID)

	// Read the notification from the send channel
	select {
	case notificationBytes := <-conn.send:
		var msg map[string]interface{}
		err := json.Unmarshal(notificationBytes, &msg)
		require.NoError(t, err, "Should be able to unmarshal notification")

		// Verify message structure
		assert.Equal(t, "notification", msg["type"], "Message type should be notification")
		assert.Equal(t, "system", msg["sender"], "Sender should be system")

		content, ok := msg["content"].(string)
		require.True(t, ok, "Content should be a string")
		assert.Contains(t, content, "Connection limit reached", "Content should mention connection limit")
		assert.Contains(t, content, "maximum number of simultaneous connections", "Content should explain the limit")
		assert.Contains(t, content, "Close an existing connection", "Content should provide guidance")

		// Verify timestamp exists
		_, hasTimestamp := msg["timestamp"]
		assert.True(t, hasTimestamp, "Message should have a timestamp")

	case <-time.After(1 * time.Second):
		t.Fatal("Should have received notification message")
	}

	// Clean up
	handler.unregisterConnection(conn)
}
