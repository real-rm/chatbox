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
	mu                   sync.RWMutex
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
	m.mu.Lock()
	m.routedMessages = append(m.routedMessages, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockRouter) RegisterConnection(sessionID string, conn *Connection) error {
	m.mu.Lock()
	m.registeredSessions[sessionID] = conn
	m.mu.Unlock()
	return nil
}

func (m *mockRouter) UnregisterConnection(sessionID string) {
	m.mu.Lock()
	m.unregisteredSessions = append(m.unregisteredSessions, sessionID)
	delete(m.registeredSessions, sessionID)
	m.mu.Unlock()
}

// RoutedMessages returns a snapshot of all routed messages (thread-safe).
func (m *mockRouter) RoutedMessages() []*message.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*message.Message, len(m.routedMessages))
	copy(result, m.routedMessages)
	return result
}

// RegisteredSessions returns a snapshot of the registered sessions map (thread-safe).
func (m *mockRouter) RegisteredSessions() map[string]*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Connection, len(m.registeredSessions))
	for k, v := range m.registeredSessions {
		result[k] = v
	}
	return result
}

// UnregisteredSessions returns a snapshot of unregistered session IDs (thread-safe).
func (m *mockRouter) UnregisteredSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.unregisteredSessions))
	copy(result, m.unregisteredSessions)
	return result
}

// TestReadPump_RouterIntegration tests that readPump properly connects to the message router
func TestReadPump_RouterIntegration(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	assert.Len(t, router.RegisteredSessions(), 0, "connection should be unregistered after close")
	assert.Len(t, router.UnregisteredSessions(), 1, "connection should have been unregistered")
	assert.Equal(t, "test-session-123", router.UnregisteredSessions()[0])

	// Verify that the message was routed
	assert.Len(t, router.RoutedMessages(), 1, "message should have been routed")
	if len(router.RoutedMessages()) > 0 {
		assert.Equal(t, message.TypeUserMessage, router.RoutedMessages()[0].Type)
		assert.Equal(t, "test-session-123", router.RoutedMessages()[0].SessionID)
		assert.Equal(t, "Hello, world!", router.RoutedMessages()[0].Content)
	}
}

// TestReadPump_SessionIDAssignment tests that session ID is assigned from first message
func TestReadPump_SessionIDAssignment(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	assert.Len(t, router.UnregisteredSessions(), 1)
	assert.Equal(t, "session-abc", router.UnregisteredSessions()[0])

	// Verify both messages were routed
	assert.Len(t, router.RoutedMessages(), 2)
}

// TestReadPump_InvalidJSON tests error handling for invalid JSON
func TestReadPump_InvalidJSON(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

	// Create a test server
	var receivedError atomic.Bool
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
					receivedError.Store(true)
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
	done := make(chan struct{})
	go func() {
		connection.readPump(handler)
		close(done)
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
	assert.True(t, receivedError.Load(), "error message should have been sent")

	// Verify no messages were routed
	assert.Len(t, router.RoutedMessages(), 0)
}

// TestReadPump_RoutingErrorHandling tests that routing errors are properly handled and logged
func TestReadPump_RoutingErrorHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")

	// Create a mock router that returns an error
	router := &mockRouterWithError{
		shouldError:   true,
		errorToReturn: chaterrors.ErrLLMUnavailable(nil),
	}

	handler := NewHandler(validator, router, testLogger(), 1048576)

	// Create a test server
	var (
		receivedErrorMsgMu sync.Mutex
		receivedErrorMsg   *message.Message
	)
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

	// Start readPump; close done to broadcast to all waiters
	done := make(chan struct{})
	go func() {
		connection.readPump(handler)
		close(done)
	}()

	// Start writePump to capture error messages; exit when done is closed
	var wgSend sync.WaitGroup
	wgSend.Add(1)
	go func() {
		defer wgSend.Done()
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
						receivedErrorMsgMu.Lock()
						receivedErrorMsg = &errorMsg
						receivedErrorMsgMu.Unlock()
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

	// Wait for the send-capture goroutine to exit before reading shared state
	wgSend.Wait()

	// Verify that an error message was sent
	receivedErrorMsgMu.Lock()
	captured := receivedErrorMsg
	receivedErrorMsgMu.Unlock()
	require.NotNil(t, captured, "error message should have been sent")
	assert.Equal(t, message.TypeError, captured.Type)
	assert.NotNil(t, captured.Error)
	assert.Equal(t, string(chaterrors.ErrCodeLLMUnavailable), captured.Error.Code)
	assert.True(t, captured.Error.Recoverable)
}

// mockRouterWithError is a mock router that can return errors
type mockRouterWithError struct {
	shouldError    bool
	errorToReturn  error
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

	handler := NewHandler(validator, router, testLogger(), 1048576)

	// Create a test server
	var (
		receivedErrorMsgMu sync.Mutex
		receivedErrorMsg   *message.Message
	)
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

	// Start readPump; close done to broadcast to all waiters
	done := make(chan struct{})
	go func() {
		connection.readPump(handler)
		close(done)
	}()

	// Start writePump to capture error messages; exit when done is closed
	var wgSend sync.WaitGroup
	wgSend.Add(1)
	go func() {
		defer wgSend.Done()
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
						receivedErrorMsgMu.Lock()
						receivedErrorMsg = &errorMsg
						receivedErrorMsgMu.Unlock()
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

	// Wait for the send-capture goroutine to exit before reading shared state
	wgSend.Wait()

	// Verify that an error message was sent for registration failure
	receivedErrorMsgMu.Lock()
	captured := receivedErrorMsg
	receivedErrorMsgMu.Unlock()
	require.NotNil(t, captured, "error message should have been sent for registration failure")
	assert.Equal(t, message.TypeError, captured.Type)
	assert.NotNil(t, captured.Error)
	assert.Equal(t, string(chaterrors.ErrCodeServiceError), captured.Error.Code)
	assert.Contains(t, captured.Error.Message, "Failed to establish session connection")
	assert.True(t, captured.Error.Recoverable)
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
				assert.Equal(t, "Hello AI", router.RoutedMessages()[0].Content)
				assert.Equal(t, message.TypeUserMessage, router.RoutedMessages()[0].Type)
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
				assert.Equal(t, "First message", router.RoutedMessages()[0].Content)
				assert.Equal(t, "Second message", router.RoutedMessages()[1].Content)
				assert.Equal(t, "Third message", router.RoutedMessages()[2].Content)
				// All should have same session ID
				for _, msg := range router.RoutedMessages() {
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
				assert.NotNil(t, router.RoutedMessages()[0].Metadata)
				assert.Equal(t, "1.0.0", router.RoutedMessages()[0].Metadata["client_version"])
				assert.Equal(t, "web", router.RoutedMessages()[0].Metadata["platform"])
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
				assert.Equal(t, "", router.RoutedMessages()[0].Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := auth.NewJWTValidator("test-secret")
			router := newMockRouter()
			handler := NewHandler(validator, router, testLogger(), 1048576)

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
			assert.Len(t, router.RoutedMessages(), tt.expectedRouted)

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
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	assert.Contains(t, router.UnregisteredSessions(), sessionID, "session should have been registered and then unregistered")

	// Verify connection got the session ID
	assert.Equal(t, sessionID, connection.SessionID)

	// Verify message was routed
	assert.Len(t, router.RoutedMessages(), 1)
	assert.Equal(t, sessionID, router.RoutedMessages()[0].SessionID)
}

// TestEndToEndMessageFlow_TimestampHandling tests that timestamps are set correctly
func TestEndToEndMessageFlow_TimestampHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	require.Len(t, router.RoutedMessages(), 2)

	// First message should have timestamp set by server
	assert.False(t, router.RoutedMessages()[0].Timestamp.IsZero(), "timestamp should be set by server")
	assert.True(t, time.Since(router.RoutedMessages()[0].Timestamp) < 5*time.Second, "timestamp should be recent")

	// Second message should preserve explicit timestamp
	assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), router.RoutedMessages()[1].Timestamp)
}

// TestEndToEndMessageFlow_SenderHandling tests that sender field is set correctly
func TestEndToEndMessageFlow_SenderHandling(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	require.Len(t, router.RoutedMessages(), 2)

	// Both messages should have sender set to User
	assert.Equal(t, message.SenderUser, router.RoutedMessages()[0].Sender, "sender should default to User")
	assert.Equal(t, message.SenderUser, router.RoutedMessages()[1].Sender)
}

// TestEndToEndMessageFlow_ConcurrentMessages tests handling of rapid message sequences
func TestEndToEndMessageFlow_ConcurrentMessages(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	assert.Len(t, router.RoutedMessages(), messageCount, "all messages should be routed")

	// Verify messages are in order (content should be sequential)
	for i := 0; i < messageCount; i++ {
		expected := "Message " + string(rune('A'+i))
		assert.Equal(t, expected, router.RoutedMessages()[i].Content, "messages should be in order")
	}
}

// TestEndToEndMessageFlow_UnregistrationOnClose tests that connections are properly unregistered
func TestEndToEndMessageFlow_UnregistrationOnClose(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	assert.Contains(t, router.UnregisteredSessions(), sessionID, "session should be unregistered on close")
	assert.NotContains(t, router.RegisteredSessions(), sessionID, "session should not be in registered map")
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
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	handler := NewHandler(validator, router, testLogger(), 1048576)

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
	handler := NewHandler(validator, nil, testLogger(), 1048576)

	// Create a test server
	var (
		receivedErrorMsgMu sync.Mutex
		receivedErrorMsg   *message.Message
	)
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

	// Start readPump; close done to broadcast to all waiters
	done := make(chan struct{})
	go func() {
		connection.readPump(handler)
		close(done)
	}()

	// Start writePump to capture error messages; exit when done is closed
	var wgSend sync.WaitGroup
	wgSend.Add(1)
	go func() {
		defer wgSend.Done()
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
						receivedErrorMsgMu.Lock()
						receivedErrorMsg = &errorMsg
						receivedErrorMsgMu.Unlock()
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

	// Wait for the send-capture goroutine to exit before reading shared state
	wgSend.Wait()

	// Verify that an error message was sent
	receivedErrorMsgMu.Lock()
	captured := receivedErrorMsg
	receivedErrorMsgMu.Unlock()
	require.NotNil(t, captured, "error message should have been sent when router is nil")
	assert.Equal(t, message.TypeError, captured.Type)
	assert.NotNil(t, captured.Error)
	assert.Equal(t, string(chaterrors.ErrCodeServiceError), captured.Error.Code)
	assert.Equal(t, "Service temporarily unavailable", captured.Error.Message)
	assert.True(t, captured.Error.Recoverable)
}

// TestOversizedMessages_Integration tests the complete flow of message size limit enforcement
// This test validates Requirements 3.1, 3.2, 3.5, 3.6
func TestOversizedMessages_Integration(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()

	// Set message size limit to 1KB for testing
	maxMessageSize := int64(1024)
	handler := NewHandler(validator, router, testLogger(), maxMessageSize)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Generate a valid JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user-oversized",
		"name":    "Test User",
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

	sessionID := "test-session-oversized"

	// Test 1: Send message at limit (should succeed)
	// Create a message that is just under the limit
	// Account for JSON overhead (type, sessionID, sender, timestamp fields)
	contentAtLimit := make([]byte, 700) // Leave room for JSON structure
	for i := range contentAtLimit {
		contentAtLimit[i] = 'A'
	}

	msgAtLimit := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   string(contentAtLimit),
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = conn.WriteJSON(msgAtLimit)
	require.NoError(t, err, "Message at limit should be sent successfully")

	// Wait for message to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify the message was routed successfully
	assert.Len(t, router.RoutedMessages(), 1, "Message at limit should be routed")
	if len(router.RoutedMessages()) > 0 {
		assert.Equal(t, string(contentAtLimit), router.RoutedMessages()[0].Content)
	}

	// Test 2: Send message over limit (should fail and close connection)
	// Create a message that exceeds the limit
	contentOverLimit := make([]byte, maxMessageSize+100)
	for i := range contentOverLimit {
		contentOverLimit[i] = 'B'
	}

	msgOverLimit := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   string(contentOverLimit),
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Try to send oversized message
	err = conn.WriteJSON(msgOverLimit)
	// The write might succeed locally, but the server should close the connection

	// Wait for connection to be closed by server
	time.Sleep(300 * time.Millisecond)

	// Try to read from connection - should get close error
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, readErr := conn.ReadMessage()

	// Verify connection was closed
	assert.Error(t, readErr, "Connection should be closed after oversized message")

	// Verify no additional messages were routed (only the first valid message)
	assert.Len(t, router.RoutedMessages(), 1, "Oversized message should not be routed")

	// Note: Log verification would require capturing log output, which is tested
	// in the property-based tests. The handler logs with user_id, connection_id,
	// and limit when the read limit is exceeded (see handler.go readPump function).
}

// TestOversizedMessages_ExactLimit tests message exactly at the limit
func TestOversizedMessages_ExactLimit(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()

	// Set message size limit to 512 bytes for testing
	maxMessageSize := int64(512)
	handler := NewHandler(validator, router, testLogger(), maxMessageSize)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Generate a valid JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user-exact",
		"name":    "Test User",
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

	sessionID := "test-session-exact"

	// Create a message with content that brings total size close to limit
	// Account for JSON structure overhead
	contentSize := 300 // Conservative size to stay under limit with JSON overhead
	content := make([]byte, contentSize)
	for i := range content {
		content[i] = 'X'
	}

	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   string(content),
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = conn.WriteJSON(msg)
	require.NoError(t, err, "Message near limit should be sent successfully")

	// Wait for message to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify the message was routed successfully
	assert.Len(t, router.RoutedMessages(), 1, "Message near limit should be routed")
	if len(router.RoutedMessages()) > 0 {
		assert.Equal(t, string(content), router.RoutedMessages()[0].Content)
	}

	// Verify connection is still open by sending another message
	msg2 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Follow-up message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = conn.WriteJSON(msg2)
	require.NoError(t, err, "Connection should still be open after message at limit")

	// Wait for second message to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify both messages were routed
	assert.Len(t, router.RoutedMessages(), 2, "Both messages should be routed")
}

// TestOversizedMessages_MultipleConnections tests that size limit is enforced per connection
func TestOversizedMessages_MultipleConnections(t *testing.T) {
	validator := auth.NewJWTValidator("test-secret")
	router := newMockRouter()

	maxMessageSize := int64(1024)
	handler := NewHandler(validator, router, testLogger(), maxMessageSize)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(handler.HandleWebSocket))
	defer server.Close()

	// Generate JWT tokens for two different users
	token1 := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user-1",
		"name":    "Test User 1",
		"roles":   []string{"user"},
		"iat":     time.Now().Unix(),
	})
	tokenString1, err := token1.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	token2 := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":     time.Now().Add(time.Hour).Unix(),
		"user_id": "test-user-2",
		"name":    "Test User 2",
		"roles":   []string{"user"},
		"iat":     time.Now().Unix(),
	})
	tokenString2, err := token2.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	// Connect first user
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers1 := http.Header{}
	headers1.Add("Authorization", "Bearer "+tokenString1)

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, headers1)
	require.NoError(t, err)
	defer conn1.Close()

	// Connect second user
	headers2 := http.Header{}
	headers2.Add("Authorization", "Bearer "+tokenString2)

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, headers2)
	require.NoError(t, err)
	defer conn2.Close()

	// User 1 sends valid message
	msg1 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: "session-user-1",
		Content:   "Valid message from user 1",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	err = conn1.WriteJSON(msg1)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// User 2 sends oversized message
	oversizedContent := make([]byte, maxMessageSize+100)
	for i := range oversizedContent {
		oversizedContent[i] = 'Z'
	}

	msg2 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: "session-user-2",
		Content:   string(oversizedContent),
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	conn2.WriteJSON(msg2) // May or may not error

	time.Sleep(300 * time.Millisecond)

	// Verify user 1's connection is still working
	msg3 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: "session-user-1",
		Content:   "Another message from user 1",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}
	err = conn1.WriteJSON(msg3)
	require.NoError(t, err, "User 1's connection should still work")

	time.Sleep(100 * time.Millisecond)

	// Verify user 2's connection is closed
	conn2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, readErr := conn2.ReadMessage()
	assert.Error(t, readErr, "User 2's connection should be closed")

	// Verify only user 1's messages were routed
	assert.GreaterOrEqual(t, len(router.RoutedMessages()), 2, "User 1's messages should be routed")

	// Check that user 1's messages are present
	foundUser1Messages := 0
	for _, msg := range router.RoutedMessages() {
		if msg.SessionID == "session-user-1" {
			foundUser1Messages++
		}
	}
	assert.Equal(t, 2, foundUser1Messages, "Both of user 1's messages should be routed")
}
