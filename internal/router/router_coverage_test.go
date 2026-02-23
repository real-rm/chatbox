package router

import (
	"fmt"
	"testing"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
)

// createTestLogger creates a logger for testing
func createTestLoggerForCoverage() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            "/tmp/chatbox-router-coverage-test-logs",
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize test logger: %v", err))
	}
	return logger
}

// TestSendFileUploadError tests the SendFileUploadError function
func TestSendFileUploadError(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name        string
		sessionID   string
		errorCode   string
		errorMsg    string
		setupConn   bool
		expectError bool
	}{
		{
			name:        "empty session ID",
			sessionID:   "",
			errorCode:   "FILE_TOO_LARGE",
			errorMsg:    "File exceeds size limit",
			setupConn:   false,
			expectError: true,
		},
		{
			name:        "valid error with connection",
			sessionID:   "test-session-1",
			errorCode:   "FILE_TOO_LARGE",
			errorMsg:    "File exceeds size limit",
			setupConn:   true,
			expectError: false,
		},
		{
			name:        "valid error without connection",
			sessionID:   "test-session-2",
			errorCode:   "INVALID_FILE_TYPE",
			errorMsg:    "File type not supported",
			setupConn:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupConn {
				mockConn := websocket.NewConnection("test-user", []string{"user"})
				router.RegisterConnection(tt.sessionID, mockConn)
				defer router.UnregisterConnection(tt.sessionID)
			}

			err := router.SendFileUploadError(tt.sessionID, tt.errorCode, tt.errorMsg)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestHandleAIGeneratedFile tests the HandleAIGeneratedFile function
func TestHandleAIGeneratedFile(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name            string
		fileURL         string
		fileDescription string
		metadata        map[string]string
		setupSession    bool
		setupConn       bool
		expectError     bool
	}{
		{
			name:            "empty file URL",
			fileURL:         "",
			fileDescription: "Generated report",
			expectError:     true,
		},
		{
			name:            "session not found",
			fileURL:         "https://example.com/file.pdf",
			fileDescription: "Generated report",
			setupSession:    false,
			expectError:     true,
		},
		{
			name:            "valid AI generated file",
			fileURL:         "https://example.com/file.pdf",
			fileDescription: "Generated report",
			metadata:        map[string]string{"type": "pdf", "size": "1024"},
			setupSession:    true,
			setupConn:       true,
			expectError:     false,
		},
		{
			name:            "valid file without connection",
			fileURL:         "https://example.com/chart.png",
			fileDescription: "Generated chart",
			setupSession:    true,
			setupConn:       false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessionID string
			if tt.setupSession {
				sess, err := sm.CreateSession("test-user-" + tt.name)
				if err != nil {
					t.Fatalf("Failed to create session: %v", err)
				}
				sessionID = sess.ID
			} else {
				sessionID = "nonexistent-session"
			}

			if tt.setupConn {
				mockConn := websocket.NewConnection("test-user", []string{"user"})
				router.RegisterConnection(sessionID, mockConn)
				defer router.UnregisterConnection(sessionID)
			}

			err := router.HandleAIGeneratedFile(sessionID, tt.fileURL, tt.fileDescription, tt.metadata)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestUnregisterAdminConnection tests the UnregisterAdminConnection function
func TestUnregisterAdminConnection(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	adminID := "admin-123"
	sessionID := "session-456"
	mockConn := websocket.NewConnection("admin-user", []string{"admin"})

	// Register admin connection with compound key
	router.RegisterAdminConnection(adminID, sessionID, mockConn)

	// Verify it's registered
	adminConnKey := adminID + ":" + sessionID
	router.mu.RLock()
	_, exists := router.adminConns[adminConnKey]
	router.mu.RUnlock()
	if !exists {
		t.Error("Admin connection should be registered")
	}

	// Unregister admin connection
	router.UnregisterAdminConnection(adminID, sessionID)

	// Verify it's unregistered
	router.mu.RLock()
	_, exists = router.adminConns[adminConnKey]
	router.mu.RUnlock()
	if exists {
		t.Error("Admin connection should be unregistered")
	}
}

// TestSendErrorMessage tests the SendErrorMessage function
func TestSendErrorMessage(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name        string
		sessionID   string
		code        chaterrors.ErrorCode
		errorMsg    string
		recoverable bool
		setupConn   bool
		expectError bool
	}{
		{
			name:        "send recoverable error",
			sessionID:   "test-session-1",
			code:        chaterrors.ErrCodeInvalidFormat,
			errorMsg:    "Invalid input",
			recoverable: true,
			setupConn:   true,
			expectError: false,
		},
		{
			name:        "send non-recoverable error",
			sessionID:   "test-session-2",
			code:        chaterrors.ErrCodeDatabaseError,
			errorMsg:    "Internal server error",
			recoverable: false,
			setupConn:   true,
			expectError: false,
		},
		{
			name:        "send error without connection",
			sessionID:   "test-session-3",
			code:        chaterrors.ErrCodeNotFound,
			errorMsg:    "Resource not found",
			recoverable: true,
			setupConn:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupConn {
				mockConn := websocket.NewConnection("test-user", []string{"user"})
				router.RegisterConnection(tt.sessionID, mockConn)
				defer router.UnregisterConnection(tt.sessionID)
			}

			err := router.SendErrorMessage(tt.sessionID, tt.code, tt.errorMsg, tt.recoverable)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestShutdown tests the Shutdown function
func TestShutdown(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Should not panic
	router.Shutdown()

	// Test with nil message limiter
	router.messageLimiter = nil
	router.Shutdown()
}

// TestHandleChatError tests the handleChatError function
func TestHandleChatError(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		setupConn bool
		setupSess bool
	}{
		{
			name:      "with connection and session",
			setupConn: true,
			setupSess: true,
		},
		{
			name:      "without connection",
			setupConn: false,
			setupSess: true,
		},
		{
			name:      "without session",
			setupConn: true,
			setupSess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessionID string
			if tt.setupSess {
				sess, err := sm.CreateSession("test-user-" + tt.name)
				if err != nil {
					t.Fatalf("Failed to create session: %v", err)
				}
				sessionID = sess.ID
			} else {
				sessionID = "nonexistent-session"
			}

			if tt.setupConn {
				mockConn := websocket.NewConnection("test-user", []string{"user"})
				router.RegisterConnection(sessionID, mockConn)
				defer router.UnregisterConnection(sessionID)
			}

			testErr := chaterrors.NewValidationError(chaterrors.ErrCodeInvalidFormat, "Test error", nil)
			router.handleChatError(sessionID, testErr)
		})
	}
}

// TestHandleError tests the HandleError function
func TestHandleError(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		setupConn bool
		setupSess bool
		errType   error
	}{
		{
			name:      "chat error with connection",
			setupConn: true,
			setupSess: true,
			errType:   chaterrors.NewValidationError(chaterrors.ErrCodeInvalidFormat, "Test error", nil),
		},
		{
			name:      "generic error with connection",
			setupConn: true,
			setupSess: true,
			errType:   fmt.Errorf("generic error"),
		},
		{
			name:      "error without connection",
			setupConn: false,
			setupSess: true,
			errType:   chaterrors.NewServiceError(chaterrors.ErrCodeDatabaseError, "DB error", nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessionID string
			if tt.setupSess {
				sess, err := sm.CreateSession("test-user-" + tt.name)
				if err != nil {
					t.Fatalf("Failed to create session: %v", err)
				}
				sessionID = sess.ID
			} else {
				sessionID = "nonexistent-session"
			}

			if tt.setupConn {
				mockConn := websocket.NewConnection("test-user", []string{"user"})
				router.RegisterConnection(sessionID, mockConn)
				defer router.UnregisterConnection(sessionID)
			}

			router.HandleError(sessionID, tt.errType)
		})
	}
}

// TestRegisterAdminConnection tests the RegisterAdminConnection function
func TestRegisterAdminConnection(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		adminID   string
		sessionID string
		expectErr bool
	}{
		{
			name:      "valid admin connection",
			adminID:   "admin-123",
			sessionID: "session-456",
			expectErr: false,
		},
		{
			name:      "empty admin ID",
			adminID:   "",
			sessionID: "session-456",
			expectErr: true,
		},
		{
			name:      "empty session ID",
			adminID:   "admin-123",
			sessionID: "",
			expectErr: true,
		},
		{
			name:      "nil connection",
			adminID:   "admin-456",
			sessionID: "session-789",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conn *websocket.Connection
			if tt.name != "nil connection" && tt.adminID != "" {
				conn = websocket.NewConnection("admin-user", []string{"admin"})
			}

			err := router.RegisterAdminConnection(tt.adminID, tt.sessionID, conn)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestHandleAIGeneratedFileEmptySessionID tests empty session ID case
func TestHandleAIGeneratedFileEmptySessionID(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	err := router.HandleAIGeneratedFile("", "https://example.com/file.pdf", "Test file", nil)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

// TestHandleHelpRequest tests the handleHelpRequest function
func TestHandleHelpRequest(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	// Create a session
	sess, err := sm.CreateSession("test-user")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Register connection
	mockConn := websocket.NewConnection("test-user", []string{"user"})
	router.RegisterConnection(sess.ID, mockConn)
	defer router.UnregisterConnection(sess.ID)

	// Create help request message
	msg := &message.Message{
		Type:      message.TypeHelpRequest,
		SessionID: sess.ID,
		Content:   "I need help",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Test help request
	err = router.handleHelpRequest(mockConn, msg)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify session has help requested flag set
	updatedSess, err := sm.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	if !updatedSess.HelpRequested {
		t.Error("Expected HelpRequested to be true")
	}
}

// TestHandleHelpRequestEdgeCases tests edge cases for handleHelpRequest
func TestHandleHelpRequestEdgeCases(t *testing.T) {
	logger := createTestLoggerForCoverage()
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, nil, nil, nil, nil, 120*time.Second, logger)

	tests := []struct {
		name      string
		conn      *websocket.Connection
		msg       *message.Message
		expectErr bool
	}{
		{
			name:      "nil connection",
			conn:      nil,
			msg:       &message.Message{SessionID: "test", Type: message.TypeHelpRequest},
			expectErr: true,
		},
		{
			name:      "nil message",
			conn:      websocket.NewConnection("test-user", []string{"user"}),
			msg:       nil,
			expectErr: true,
		},
		{
			name: "empty session ID",
			conn: websocket.NewConnection("test-user", []string{"user"}),
			msg: &message.Message{
				SessionID: "",
				Type:      message.TypeHelpRequest,
			},
			expectErr: true,
		},
		{
			name: "session not found",
			conn: websocket.NewConnection("test-user", []string{"user"}),
			msg: &message.Message{
				SessionID: "nonexistent",
				Type:      message.TypeHelpRequest,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.handleHelpRequest(tt.conn, tt.msg)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
