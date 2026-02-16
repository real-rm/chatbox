package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiDevice_MessageBroadcastToAllConnections tests that messages sent to a user
// are received by all their active connections (simulating multiple devices)
func TestMultiDevice_MessageBroadcastToAllConnections(t *testing.T) {
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

	// Connect from 3 different "devices"
	numDevices := 3
	connections := make([]*websocket.Conn, numDevices)

	for i := 0; i < numDevices; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d", i)
		connections[i] = conn
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns, exists := handler.connections[userID]
	handler.mu.RUnlock()
	require.True(t, exists, "User should have connections registered")
	require.Equal(t, numDevices, len(userConns), "User should have %d connections", numDevices)

	// Send a message to all user connections
	testMessage := message.Message{
		Type:      message.TypeUserMessage,
		Content:   "Hello from server to all devices!",
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}
	messageBytes, err := json.Marshal(testMessage)
	require.NoError(t, err)

	// Broadcast to all user connections
	handler.mu.RLock()
	for _, conn := range userConns {
		select {
		case conn.send <- messageBytes:
		default:
			t.Fatal("Failed to send message to connection")
		}
	}
	handler.mu.RUnlock()

	// Verify all connections received the message
	receivedCount := 0
	for i, conn := range connections {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, receivedBytes, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("Connection %d failed to receive message: %v", i, err)
			continue
		}

		var receivedMsg message.Message
		err = json.Unmarshal(receivedBytes, &receivedMsg)
		require.NoError(t, err, "Failed to unmarshal message on connection %d", i)

		assert.Equal(t, testMessage.Content, receivedMsg.Content, "Connection %d received wrong content", i)
		assert.Equal(t, testMessage.Type, receivedMsg.Type, "Connection %d received wrong type", i)
		receivedCount++
	}

	assert.Equal(t, numDevices, receivedCount, "All devices should receive the broadcast message")

	// Clean up
	for _, conn := range connections {
		conn.Close()
	}
}

// TestMultiDevice_IndependentSessions tests that each device connection can have its own session
func TestMultiDevice_IndependentSessions(t *testing.T) {
	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	
	// Create a mock router to track session registrations
	mockRouter := &mockMessageRouter{
		sessions: make(map[string]*Connection),
	}
	
	handler := NewHandler(validator, mockRouter, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create token for a single user
	userID := "test-user"
	token := createTestToken(userID, []string{"user"}, testSecret)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

	// Connect from 2 different "devices"
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Send messages with different session IDs from each connection
	session1ID := "session-1"
	session2ID := "session-2"

	msg1 := message.Message{
		Type:      message.TypeUserMessage,
		Content:   "Message from device 1",
		SessionID: session1ID,
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	msg2 := message.Message{
		Type:      message.TypeUserMessage,
		Content:   "Message from device 2",
		SessionID: session2ID,
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Send from device 1
	msg1Bytes, _ := json.Marshal(msg1)
	err = conn1.WriteMessage(websocket.TextMessage, msg1Bytes)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Send from device 2
	msg2Bytes, _ := json.Marshal(msg2)
	err = conn2.WriteMessage(websocket.TextMessage, msg2Bytes)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Verify both sessions are registered independently
	mockRouter.mu.Lock()
	assert.Contains(t, mockRouter.sessions, session1ID, "Session 1 should be registered")
	assert.Contains(t, mockRouter.sessions, session2ID, "Session 2 should be registered")
	assert.NotEqual(t, mockRouter.sessions[session1ID].ConnectionID, 
		mockRouter.sessions[session2ID].ConnectionID, 
		"Each session should have a different connection")
	mockRouter.mu.Unlock()

	// Clean up
	conn1.Close()
	conn2.Close()
}

// TestMultiDevice_ConnectionFailureIsolation tests that if one device connection fails,
// other device connections remain functional
func TestMultiDevice_ConnectionFailureIsolation(t *testing.T) {
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

	// Connect from 3 different "devices"
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	conn3, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Verify all 3 connections are active
	handler.mu.RLock()
	userConns := handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 3, len(userConns), "Should have 3 active connections")

	// Simulate device 2 connection failure (abrupt close)
	conn2.Close()
	time.Sleep(150 * time.Millisecond)

	// Verify only 2 connections remain
	handler.mu.RLock()
	userConns = handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 2, len(userConns), "Should have 2 active connections after one fails")

	// Send a test message to remaining connections
	testMessage := []byte(`{"type":"text","content":"Test after failure","sender":"ai"}`)
	handler.mu.RLock()
	for _, conn := range userConns {
		conn.send <- testMessage
	}
	handler.mu.RUnlock()

	// Verify conn1 and conn3 can still receive messages
	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	assert.NoError(t, err, "Device 1 should still receive messages")
	assert.Contains(t, string(msg1), "Test after failure")

	conn3.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg3, err := conn3.ReadMessage()
	assert.NoError(t, err, "Device 3 should still receive messages")
	assert.Contains(t, string(msg3), "Test after failure")

	// Clean up
	conn1.Close()
	conn3.Close()
}

// TestMultiDevice_ConcurrentMessageSending tests that multiple devices can send messages
// concurrently without interference
func TestMultiDevice_ConcurrentMessageSending(t *testing.T) {
	testSecret := "test-secret"
	validator := auth.NewJWTValidator(testSecret)
	
	mockRouter := &mockMessageRouter{
		sessions:       make(map[string]*Connection),
		receivedMsgs:   make([]message.Message, 0),
	}
	
	handler := NewHandler(validator, mockRouter, testLogger(), 1048576)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r)
	}))
	defer server.Close()

	// Create token for a single user
	userID := "test-user"
	token := createTestToken(userID, []string{"user"}, testSecret)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token

	// Connect from 3 different "devices"
	numDevices := 3
	connections := make([]*websocket.Conn, numDevices)

	for i := 0; i < numDevices; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Failed to establish connection %d", i)
		connections[i] = conn
		time.Sleep(50 * time.Millisecond)
	}

	// Each device sends multiple messages concurrently
	messagesPerDevice := 5
	done := make(chan bool, numDevices)

	for deviceIdx, conn := range connections {
		go func(idx int, c *websocket.Conn) {
			for msgIdx := 0; msgIdx < messagesPerDevice; msgIdx++ {
				msg := message.Message{
					Type:      message.TypeUserMessage,
					Content:   fmt.Sprintf("Message %d from device %d", msgIdx, idx),
					SessionID: fmt.Sprintf("session-%d", idx),
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				}
				msgBytes, _ := json.Marshal(msg)
				c.WriteMessage(websocket.TextMessage, msgBytes)
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}(deviceIdx, conn)
	}

	// Wait for all devices to finish sending
	for i := 0; i < numDevices; i++ {
		<-done
	}

	time.Sleep(200 * time.Millisecond)

	// Verify all messages were received by the router
	mockRouter.mu.Lock()
	totalReceived := len(mockRouter.receivedMsgs)
	mockRouter.mu.Unlock()

	expectedTotal := numDevices * messagesPerDevice
	assert.Equal(t, expectedTotal, totalReceived, 
		"Router should receive all messages from all devices")

	// Clean up
	for _, conn := range connections {
		conn.Close()
	}
}

// TestMultiDevice_ReconnectionScenario tests that a user can disconnect and reconnect
// from a device while maintaining other connections
func TestMultiDevice_ReconnectionScenario(t *testing.T) {
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

	// Initial connections from 2 devices
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Verify 2 connections
	handler.mu.RLock()
	userConns := handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 2, len(userConns), "Should have 2 connections")

	// Device 2 disconnects
	conn2.Close()
	time.Sleep(150 * time.Millisecond)

	// Verify only 1 connection remains
	handler.mu.RLock()
	userConns = handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 1, len(userConns), "Should have 1 connection after disconnect")

	// Device 2 reconnects
	conn2New, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Verify 2 connections again
	handler.mu.RLock()
	userConns = handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, 2, len(userConns), "Should have 2 connections after reconnect")

	// Verify both connections are functional
	testMessage := []byte(`{"type":"text","content":"Test after reconnect","sender":"ai"}`)
	handler.mu.RLock()
	for _, conn := range userConns {
		conn.send <- testMessage
	}
	handler.mu.RUnlock()

	// Both devices should receive the message
	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	assert.NoError(t, err, "Device 1 should receive message")
	assert.Contains(t, string(msg1), "Test after reconnect")

	conn2New.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg2, err := conn2New.ReadMessage()
	assert.NoError(t, err, "Device 2 (reconnected) should receive message")
	assert.Contains(t, string(msg2), "Test after reconnect")

	// Clean up
	conn1.Close()
	conn2New.Close()
}

// TestMultiDevice_MaxConnectionsEnforcement tests that connection limits are enforced
// across multiple devices
func TestMultiDevice_MaxConnectionsEnforcement(t *testing.T) {
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

	// Connect up to the limit (10 devices)
	maxConnections := 10
	connections := make([]*websocket.Conn, maxConnections)

	for i := 0; i < maxConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err, "Should be able to connect device %d", i)
		connections[i] = conn
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all connections are registered
	handler.mu.RLock()
	userConns := handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, maxConnections, len(userConns), "Should have max connections")

	// Try to connect an 11th device - should be rejected
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	assert.Error(t, err, "11th device connection should be rejected")
	if resp != nil {
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, 
			"Should return 429 Too Many Requests")
	}

	// Disconnect one device
	connections[0].Close()
	time.Sleep(150 * time.Millisecond)

	// Now the 11th device should be able to connect
	conn11, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "Should be able to connect after one device disconnects")
	time.Sleep(50 * time.Millisecond)

	// Verify we're back at max connections
	handler.mu.RLock()
	userConns = handler.connections[userID]
	handler.mu.RUnlock()
	assert.Equal(t, maxConnections, len(userConns), "Should be at max connections again")

	// Clean up
	conn11.Close()
	for i := 1; i < maxConnections; i++ {
		connections[i].Close()
	}
}

// Helper function to create a test JWT token
func createTestToken(userID string, roles []string, secret string) string {
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

// mockMessageRouter is a mock implementation of MessageRouter for testing
type mockMessageRouter struct {
	mu           sync.Mutex
	sessions     map[string]*Connection
	receivedMsgs []message.Message
}

func (m *mockMessageRouter) RouteMessage(conn *Connection, msg *message.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedMsgs = append(m.receivedMsgs, *msg)
	return nil
}

func (m *mockMessageRouter) RegisterConnection(sessionID string, conn *Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[sessionID] = conn
	return nil
}

func (m *mockMessageRouter) UnregisterConnection(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}
