package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRouter implements the MessageRouter interface for testing
type mockRouter struct {
	registeredSessions   map[string]*Connection
	unregisteredSessions []string
	routedMessages       []*message.Message
}

func newMockRouter() *mockRouter {
	return &mockRouter{
		registeredSessions:   make(map[string]*Connection),
		unregisteredSessions: make([]string, 0),
		routedMessages:       make([]*message.Message, 0),
	}
}

func (m *mockRouter) RouteMessage(conn *Connection, msg *message.Message) error {
	m.routedMessages = append(m.routedMessages, msg)
	return nil
}

func (m *mockRouter) RegisterConnection(sessionID string, conn *Connection) error {
	m.registeredSessions[sessionID] = conn
	return nil
}

func (m *mockRouter) UnregisterConnection(sessionID string) {
	m.unregisteredSessions = append(m.unregisteredSessions, sessionID)
	delete(m.registeredSessions, sessionID)
}

// TestReadPump_RouterIntegration tests that readPump properly connects to the message router
func TestReadPump_RouterIntegration(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send test message with session ID
		testMsg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "test-session-123",
			Content:   "Hello, world!",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		
		err = conn.WriteJSON(testMsg)
		require.NoError(t, err)

		// Wait a bit for message to be processed
		time.Sleep(200 * time.Millisecond)
		
		// Close connection
		conn.Close()
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-1",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump
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

	// Verify that the connection was registered with the router
	assert.Len(t, router.registeredSessions, 0, "connection should be unregistered after close")
	assert.Len(t, router.unregisteredSessions, 1, "connection should have been unregistered")
	assert.Equal(t, "test-session-123", router.unregisteredSessions[0])

	// Verify that the message was routed
	assert.Len(t, router.routedMessages, 1, "message should have been routed")
	if len(router.routedMessages) > 0 {
		assert.Equal(t, message.TypeUserMessage, router.routedMessages[0].Type)
		assert.Equal(t, "test-session-123", router.routedMessages[0].SessionID)
		assert.Equal(t, "Hello, world!", router.routedMessages[0].Content)
	}
}

// TestReadPump_SessionIDAssignment tests that session ID is assigned from first message
func TestReadPump_SessionIDAssignment(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send first message with session ID
		msg1 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-abc",
			Content:   "First message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		conn.WriteJSON(msg1)

		// Send second message with same session ID
		time.Sleep(50 * time.Millisecond)
		msg2 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-abc",
			Content:   "Second message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		conn.WriteJSON(msg2)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection without session ID
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-2",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Verify session ID is empty initially
	assert.Empty(t, connection.SessionID)

	// Register connection
	handler.registerConnection(connection)

	// Start readPump
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

	// Verify that session ID was assigned
	assert.Equal(t, "session-abc", connection.SessionID)

	// Verify that connection was registered only once with the router
	assert.Len(t, router.unregisteredSessions, 1)
	assert.Equal(t, "session-abc", router.unregisteredSessions[0])

	// Verify both messages were routed
	assert.Len(t, router.routedMessages, 2)
}

// TestReadPump_InvalidJSON tests error handling for invalid JSON
func TestReadPump_InvalidJSON(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	receivedError := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send invalid JSON
		conn.WriteMessage(websocket.TextMessage, []byte("invalid json"))

		// Try to read error response
		_, msg, err := conn.ReadMessage()
		if err == nil {
			// Check if it's an error message
			var errorMsg message.Message
			if errorMsg.UnmarshalJSON(msg) == nil {
				if errorMsg.Type == message.TypeError {
					receivedError = true
				}
			}
		}
		
		// Wait a bit before closing
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-3",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump and writePump
	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()
	
	go connection.writePump()

	// Wait for readPump to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify error was sent
	assert.True(t, receivedError, "error message should have been sent")

	// Verify no messages were routed
	assert.Len(t, router.routedMessages, 0)
}

// TestReadPump_RoutingErrorHandling tests that routing errors are properly handled and logged
func TestReadPump_RoutingErrorHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	
	// Create a mock router that returns an error
	router := &mockRouterWithError{
		shouldError: true,
		errorToReturn: chaterrors.ErrLLMUnavailable(nil),
	}
	
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	var receivedErrorMsg *message.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send test message
		testMsg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "test-session-error",
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		
		err = conn.WriteJSON(testMsg)
		require.NoError(t, err)

		// Wait a bit for message to be processed
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-error",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump and writePump
	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()
	
	// Start writePump to send error messages
	go func() {
		for {
			select {
			case msg, ok := <-connection.send:
				if !ok {
					return
				}
				// Parse the error message
				var errorMsg message.Message
				if err := json.Unmarshal(msg, &errorMsg); err == nil {
					if errorMsg.Type == message.TypeError {
						receivedErrorMsg = &errorMsg
					}
				}
			case <-done:
				return
			}
		}
	}()

	// Wait for readPump to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify that an error message was sent
	require.NotNil(t, receivedErrorMsg, "error message should have been sent")
	assert.Equal(t, message.TypeError, receivedErrorMsg.Type)
	assert.NotNil(t, receivedErrorMsg.Error)
	assert.Equal(t, string(chaterrors.ErrCodeLLMUnavailable), receivedErrorMsg.Error.Code)
	assert.True(t, receivedErrorMsg.Error.Recoverable)
}

// mockRouterWithError is a mock router that can return errors
type mockRouterWithError struct {
	shouldError   bool
	errorToReturn error
	routedMessages []*message.Message
}

func (m *mockRouterWithError) RouteMessage(conn *Connection, msg *message.Message) error {
	m.routedMessages = append(m.routedMessages, msg)
	if m.shouldError {
		return m.errorToReturn
	}
	return nil
}

func (m *mockRouterWithError) RegisterConnection(sessionID string, conn *Connection) error {
	return nil
}

func (m *mockRouterWithError) UnregisterConnection(sessionID string) {
}

// TestReadPump_RegistrationErrorHandling tests that connection registration errors are properly handled
func TestReadPump_RegistrationErrorHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	
	// Create a mock router that returns an error on registration
	router := &mockRouterWithRegistrationError{
		registrationError: chaterrors.ErrDatabaseError(nil),
	}
	
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	var receivedErrorMsg *message.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send test message with session ID (triggers registration)
		testMsg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "test-session-reg-error",
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		
		err = conn.WriteJSON(testMsg)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection without session ID
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-reg-error",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump and writePump
	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()
	
	// Start writePump to capture error messages
	go func() {
		for {
			select {
			case msg, ok := <-connection.send:
				if !ok {
					return
				}
				// Parse the error message
				var errorMsg message.Message
				if err := json.Unmarshal(msg, &errorMsg); err == nil {
					if errorMsg.Type == message.TypeError {
						receivedErrorMsg = &errorMsg
					}
				}
			case <-done:
				return
			}
		}
	}()

	// Wait for readPump to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify that an error message was sent for registration failure
	require.NotNil(t, receivedErrorMsg, "error message should have been sent for registration failure")
	assert.Equal(t, message.TypeError, receivedErrorMsg.Type)
	assert.NotNil(t, receivedErrorMsg.Error)
	assert.Equal(t, string(chaterrors.ErrCodeServiceError), receivedErrorMsg.Error.Code)
	assert.Contains(t, receivedErrorMsg.Error.Message, "Failed to establish session connection")
	assert.True(t, receivedErrorMsg.Error.Recoverable)
}

// mockRouterWithRegistrationError is a mock router that returns errors on registration
type mockRouterWithRegistrationError struct {
	registrationError error
}

func (m *mockRouterWithRegistrationError) RouteMessage(conn *Connection, msg *message.Message) error {
	return nil
}

func (m *mockRouterWithRegistrationError) RegisterConnection(sessionID string, conn *Connection) error {
	return m.registrationError
}

func (m *mockRouterWithRegistrationError) UnregisterConnection(sessionID string) {
}

// TestEndToEndMessageFlow tests the complete message flow from WebSocket to router
func TestEndToEndMessageFlow(t *testing.T) {
	tests := []struct {
		name           string
		messages       []*message.Message
		expectedRouted int
		validateFunc   func(t *testing.T, router *mockRouter)
	}{
		{
			name: "single user message",
			messages: []*message.Message{
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-001",
					Content:   "Hello AI",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				},
			},
			expectedRouted: 1,
			validateFunc: func(t *testing.T, router *mockRouter) {
				assert.Equal(t, "Hello AI", router.routedMessages[0].Content)
				assert.Equal(t, message.TypeUserMessage, router.routedMessages[0].Type)
			},
		},
		{
			name: "multiple messages in sequence",
			messages: []*message.Message{
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-002",
					Content:   "First message",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				},
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-002",
					Content:   "Second message",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				},
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-002",
					Content:   "Third message",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				},
			},
			expectedRouted: 3,
			validateFunc: func(t *testing.T, router *mockRouter) {
				assert.Equal(t, "First message", router.routedMessages[0].Content)
				assert.Equal(t, "Second message", router.routedMessages[1].Content)
				assert.Equal(t, "Third message", router.routedMessages[2].Content)
				// All should have same session ID
				for _, msg := range router.routedMessages {
					assert.Equal(t, "session-002", msg.SessionID)
				}
			},
		},
		{
			name: "message with metadata",
			messages: []*message.Message{
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-003",
					Content:   "Message with metadata",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
					Metadata: map[string]string{
						"client_version": "1.0.0",
						"platform":       "web",
					},
				},
			},
			expectedRouted: 1,
			validateFunc: func(t *testing.T, router *mockRouter) {
				assert.NotNil(t, router.routedMessages[0].Metadata)
				assert.Equal(t, "1.0.0", router.routedMessages[0].Metadata["client_version"])
				assert.Equal(t, "web", router.routedMessages[0].Metadata["platform"])
			},
		},
		{
			name: "empty content message",
			messages: []*message.Message{
				{
					Type:      message.TypeUserMessage,
					SessionID: "session-004",
					Content:   "",
					Sender:    message.SenderUser,
					Timestamp: time.Now(),
				},
			},
			expectedRouted: 1,
			validateFunc: func(t *testing.T, router *mockRouter) {
				assert.Equal(t, "", router.routedMessages[0].Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := auth.NewJWTValidator("test-secret")
			router := newMockRouter()
			handler := NewHandler(validator, router, testLogger())

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				require.NoError(t, err)
				defer conn.Close()

				// Send all test messages
				for _, msg := range tt.messages {
					err = conn.WriteJSON(msg)
					require.NoError(t, err)
					time.Sleep(50 * time.Millisecond) // Small delay between messages
				}

				// Wait for processing
				time.Sleep(200 * time.Millisecond)
			}))
			defer server.Close()

			// Connect to test server
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			require.NoError(t, err)

			// Create connection
			connection := &Connection{
				conn:         conn,
				ConnectionID: "test-conn-" + tt.name,
				UserID:       "test-user",
				send:         make(chan []byte, 256),
			}

			// Register and start pumps
			handler.registerConnection(connection)

			done := make(chan bool)
			go func() {
				connection.readPump(handler)
				done <- true
			}()

			// Wait for completion
			select {
			case <-done:
				// Success
			case <-time.After(3 * time.Second):
				t.Fatal("readPump did not finish in time")
			}

			// Verify expected number of messages routed
			assert.Len(t, router.routedMessages, tt.expectedRouted)

			// Run custom validation
			if tt.validateFunc != nil {
				tt.validateFunc(t, router)
			}
		})
	}
}

// TestEndToEndMessageFlow_SessionRegistration tests that session registration happens correctly
func TestEndToEndMessageFlow_SessionRegistration(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	sessionID := "session-registration-test"

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send message with session ID
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		conn.WriteJSON(msg)

		// Wait for processing
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection without session ID
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-reg",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Verify session ID is empty initially
	assert.Empty(t, connection.SessionID)

	// Register and start pump
	handler.registerConnection(connection)

	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	// Wait for completion
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify session was registered with router
	assert.Contains(t, router.unregisteredSessions, sessionID, "session should have been registered and then unregistered")

	// Verify connection got the session ID
	assert.Equal(t, sessionID, connection.SessionID)

	// Verify message was routed
	assert.Len(t, router.routedMessages, 1)
	assert.Equal(t, sessionID, router.routedMessages[0].SessionID)
}

// TestEndToEndMessageFlow_TimestampHandling tests that timestamps are set correctly
func TestEndToEndMessageFlow_TimestampHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send message without timestamp
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-timestamp",
			Content:   "Message without timestamp",
			Sender:    message.SenderUser,
			// Timestamp intentionally omitted
		}
		conn.WriteJSON(msg)

		time.Sleep(100 * time.Millisecond)

		// Send message with explicit timestamp
		explicitTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		msg2 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-timestamp",
			Content:   "Message with timestamp",
			Sender:    message.SenderUser,
			Timestamp: explicitTime,
		}
		conn.WriteJSON(msg2)

		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-timestamp",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(connection)

	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify both messages were routed
	require.Len(t, router.routedMessages, 2)

	// First message should have timestamp set by server
	assert.False(t, router.routedMessages[0].Timestamp.IsZero(), "timestamp should be set by server")
	assert.True(t, time.Since(router.routedMessages[0].Timestamp) < 5*time.Second, "timestamp should be recent")

	// Second message should preserve explicit timestamp
	assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), router.routedMessages[1].Timestamp)
}

// TestEndToEndMessageFlow_SenderHandling tests that sender field is set correctly
func TestEndToEndMessageFlow_SenderHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send message without sender
		msg1 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-sender",
			Content:   "Message without sender",
			Timestamp: time.Now(),
			// Sender intentionally omitted
		}
		conn.WriteJSON(msg1)

		time.Sleep(50 * time.Millisecond)

		// Send message with explicit sender
		msg2 := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "session-sender",
			Content:   "Message with sender",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		conn.WriteJSON(msg2)

		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-sender",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(connection)

	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify both messages were routed
	require.Len(t, router.routedMessages, 2)

	// Both messages should have sender set to User
	assert.Equal(t, message.SenderUser, router.routedMessages[0].Sender, "sender should default to User")
	assert.Equal(t, message.SenderUser, router.routedMessages[1].Sender)
}

// TestEndToEndMessageFlow_ConcurrentMessages tests handling of rapid message sequences
func TestEndToEndMessageFlow_ConcurrentMessages(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	messageCount := 10
	sessionID := "session-concurrent"

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send multiple messages rapidly
		for i := 0; i < messageCount; i++ {
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sessionID,
				Content:   "Message " + string(rune('A'+i)),
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}
			conn.WriteJSON(msg)
			time.Sleep(10 * time.Millisecond) // Very short delay
		}

		// Wait for processing
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-concurrent",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(connection)

	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify all messages were routed
	assert.Len(t, router.routedMessages, messageCount, "all messages should be routed")

	// Verify messages are in order (content should be sequential)
	for i := 0; i < messageCount; i++ {
		expected := "Message " + string(rune('A'+i))
		assert.Equal(t, expected, router.routedMessages[i].Content, "messages should be in order")
	}
}

// TestEndToEndMessageFlow_UnregistrationOnClose tests that connections are properly unregistered
func TestEndToEndMessageFlow_UnregistrationOnClose(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger())

	sessionID := "session-unregister"

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send message to establish session
		msg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: sessionID,
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		conn.WriteJSON(msg)

		// Wait briefly then close
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-unreg",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	handler.registerConnection(connection)

	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify session was unregistered
	assert.Contains(t, router.unregisteredSessions, sessionID, "session should be unregistered on close")
	assert.NotContains(t, router.registeredSessions, sessionID, "session should not be in registered map")
}

// streamingMockRouter is a mock router that simulates streaming LLM responses
type streamingMockRouter struct {
	registeredSessions   map[string]*Connection
	unregisteredSessions []string
	chunks               []string
}

func newStreamingMockRouter(chunks []string) *streamingMockRouter {
	return &streamingMockRouter{
		registeredSessions:   make(map[string]*Connection),
		unregisteredSessions: make([]string, 0),
		chunks:               chunks,
	}
}

func (m *streamingMockRouter) RouteMessage(conn *Connection, msg *message.Message) error {
	// Simulate streaming response by sending multiple chunks
	go func() {
		// Send loading indicator
		loadingMsg := &message.Message{
			Type:      message.TypeLoading,
			SessionID: msg.SessionID,
			Sender:    message.SenderAI,
			Timestamp: time.Now(),
		}
		data, _ := json.Marshal(loadingMsg)
		conn.Send() <- data

		// Send chunks
		for i, chunk := range m.chunks {
			chunkMsg := &message.Message{
				Type:      message.TypeAIResponse,
				SessionID: msg.SessionID,
				Content:   chunk,
				Sender:    message.SenderAI,
				Timestamp: time.Now(),
				Metadata: map[string]string{
					"streaming": "true",
					"done":      "false",
				},
			}
			if i == len(m.chunks)-1 {
				chunkMsg.Metadata["done"] = "true"
			}
			data, _ := json.Marshal(chunkMsg)
			conn.Send() <- data
			time.Sleep(10 * time.Millisecond) // Simulate streaming delay
		}
	}()
	return nil
}

func (m *streamingMockRouter) RegisterConnection(sessionID string, conn *Connection) error {
	m.registeredSessions[sessionID] = conn
	return nil
}

func (m *streamingMockRouter) UnregisterConnection(sessionID string) {
	m.unregisteredSessions = append(m.unregisteredSessions, sessionID)
	delete(m.registeredSessions, sessionID)
}

// TestEndToEndStreamingFlow tests the complete streaming flow from client to LLM and back
func TestEndToEndStreamingFlow(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	chunks := []string{"Hello", ", ", "world", "!"}
	router := newStreamingMockRouter(chunks)
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Generate a valid JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user",
		"roles":   []string{"user"},
		"iat":     time.Now().Unix(),
	})
	tokenString, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	// Connect to test server with JWT token
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+tokenString)
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	require.NoError(t, err)
	defer conn.Close()

	// Send a user message
	sessionID := "test-session-streaming"
	userMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Tell me a story",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	
	err = conn.WriteJSON(userMsg)
	require.NoError(t, err)

	// Collect streaming responses
	var receivedMessages []*message.Message
	timeout := time.After(2 * time.Second)
	expectedMessages := len(chunks) + 1 // +1 for loading indicator

collectLoop:
	for len(receivedMessages) < expectedMessages {
		select {
		case <-timeout:
			break collectLoop
		default:
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var msg message.Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					t.Logf("WebSocket error: %v", err)
				}
				break collectLoop
			}
			receivedMessages = append(receivedMessages, &msg)
		}
	}

	// Verify we received all expected messages
	require.GreaterOrEqual(t, len(receivedMessages), expectedMessages,
		"Should receive loading indicator + all chunks")

	// First message should be loading indicator
	assert.Equal(t, message.TypeLoading, receivedMessages[0].Type)
	assert.Equal(t, sessionID, receivedMessages[0].SessionID)

	// Subsequent messages should be streaming chunks
	var fullContent string
	chunkCount := 0
	for i := 1; i < len(receivedMessages); i++ {
		msg := receivedMessages[i]
		if msg.Type == message.TypeAIResponse {
			chunkCount++
			fullContent += msg.Content
			
			// Verify metadata
			assert.Equal(t, "true", msg.Metadata["streaming"])
			assert.NotEmpty(t, msg.Metadata["done"])
			
			// Last chunk should have done=true
			if i == len(receivedMessages)-1 {
				assert.Equal(t, "true", msg.Metadata["done"])
			}
		}
	}

	assert.Equal(t, len(chunks), chunkCount, "Should receive all chunks")
	assert.Equal(t, "Hello, world!", fullContent, "Full content should match")
}

// TestEndToEndStreamingFlow_MultipleMessages tests streaming with multiple user messages
func TestEndToEndStreamingFlow_MultipleMessages(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	chunks := []string{"Response ", "1"}
	router := newStreamingMockRouter(chunks)
	handler := NewHandler(validator, router, testLogger())

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Generate a valid JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user",
		"roles":   []string{"user"},
		"iat":     time.Now().Unix(),
	})
	tokenString, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	// Connect to test server with JWT token
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+tokenString)
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	require.NoError(t, err)
	defer conn.Close()

	sessionID := "test-session-multi"

	// Send first message
	msg1 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "First message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	err = conn.WriteJSON(msg1)
	require.NoError(t, err)

	// Wait for first response to complete
	time.Sleep(200 * time.Millisecond)

	// Send second message
	msg2 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Second message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	err = conn.WriteJSON(msg2)
	require.NoError(t, err)

	// Collect all responses
	var receivedMessages []*message.Message
	timeout := time.After(2 * time.Second)
	expectedMessages := (len(chunks) + 1) * 2 // Two sets of responses

collectLoop:
	for len(receivedMessages) < expectedMessages {
		select {
		case <-timeout:
			break collectLoop
		default:
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var msg message.Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				break collectLoop
			}
			receivedMessages = append(receivedMessages, &msg)
		}
	}

	// Verify we received responses for both messages
	assert.GreaterOrEqual(t, len(receivedMessages), expectedMessages,
		"Should receive responses for both messages")

	// Count loading indicators (should be 2)
	loadingCount := 0
	for _, msg := range receivedMessages {
		if msg.Type == message.TypeLoading {
			loadingCount++
		}
	}
	assert.Equal(t, 2, loadingCount, "Should receive 2 loading indicators")
}

// TestReadPump_NilRouterHandling tests that messages are handled gracefully when router is nil
func TestReadPump_NilRouterHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	
	// Create handler with nil router
	handler := NewHandler(validator, nil, testLogger())

	// Create a test server
	var receivedErrorMsg *message.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Send test message
		testMsg := &message.Message{
			Type:      message.TypeUserMessage,
			SessionID: "test-session-nil-router",
			Content:   "Test message",
			Sender:    message.SenderUser,
			Timestamp: time.Now(),
		}
		
		err = conn.WriteJSON(testMsg)
		require.NoError(t, err)

		// Wait a bit for message to be processed
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	// Create connection
	connection := &Connection{
		conn:         conn,
		ConnectionID: "test-conn-nil-router",
		UserID:       "test-user",
		send:         make(chan []byte, 256),
	}

	// Register connection
	handler.registerConnection(connection)

	// Start readPump and writePump
	done := make(chan bool)
	go func() {
		connection.readPump(handler)
		done <- true
	}()
	
	// Start writePump to send error messages
	go func() {
		for {
			select {
			case msg, ok := <-connection.send:
				if !ok {
					return
				}
				// Parse the error message
				var errorMsg message.Message
				if err := json.Unmarshal(msg, &errorMsg); err == nil {
					if errorMsg.Type == message.TypeError {
						receivedErrorMsg = &errorMsg
					}
				}
			case <-done:
				return
			}
		}
	}()

	// Wait for readPump to finish
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not finish in time")
	}

	// Verify that an error message was sent
	require.NotNil(t, receivedErrorMsg, "error message should have been sent when router is nil")
	assert.Equal(t, message.TypeError, receivedErrorMsg.Type)
	assert.NotNil(t, receivedErrorMsg.Error)
	assert.Equal(t, string(chaterrors.ErrCodeServiceError), receivedErrorMsg.Error.Code)
	assert.Equal(t, "Service temporarily unavailable", receivedErrorMsg.Error.Message)
	assert.True(t, receivedErrorMsg.Error.Recoverable)
}
