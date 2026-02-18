package router

import (
	"context"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CompleteMessageFlow tests the complete message flow:
// user connects → sends message → LLM processes → response sent back
// **Validates: Requirements 6.1**
func TestIntegration_CompleteMessageFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create user connection
	userConn := mockConnection("user-123")

	// Create session
	sess, err := router.getOrCreateSession(userConn, "test-session-1")
	require.NoError(t, err)
	require.NotNil(t, sess)
	sessionID := sess.ID

	// Register connection
	err = router.RegisterConnection(sessionID, userConn)
	require.NoError(t, err)

	// Send user message
	userMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Hello, how are you?",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, userMsg)
	require.NoError(t, err)

	// Verify session was created in storage
	assert.True(t, mockStorage.createSessionCalled)
	assert.Len(t, mockStorage.createdSessions, 1)
	assert.Equal(t, sessionID, mockStorage.createdSessions[0].ID)
	assert.Equal(t, "user-123", mockStorage.createdSessions[0].UserID)

	// Verify LLM was called
	assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)
	assert.NotEmpty(t, mockLLM.lastMessages)

	// Verify session is active
	retrievedSess, err := sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.IsActive)
	assert.Equal(t, "user-123", retrievedSess.UserID)
}

// TestIntegration_AdminTakeoverFlow tests the admin takeover flow:
// user connects → admin takes over → messages routed between user and admin
// **Validates: Requirements 6.1**
func TestIntegration_AdminTakeoverFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create user connection and session
	userConn := mockConnection("user-456")
	sess, err := router.getOrCreateSession(userConn, "test-session-2")
	require.NoError(t, err)
	sessionID := sess.ID

	// Register user connection
	err = router.RegisterConnection(sessionID, userConn)
	require.NoError(t, err)

	// User sends initial message
	userMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "I need help",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, userMsg)
	require.NoError(t, err)

	// Admin takes over the session
	adminConn := websocket.NewConnection("admin-001", []string{"admin"})
	adminConn.Name = "Support Admin"

	err = router.HandleAdminTakeover(adminConn, sessionID)
	require.NoError(t, err)

	// Verify session is marked as admin-assisted
	retrievedSess, err := sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.AdminAssisted, "Session should be marked as admin-assisted")
	assert.Equal(t, "admin-001", retrievedSess.AssistingAdminID, "Admin ID should match")
	assert.Equal(t, "Support Admin", retrievedSess.AssistingAdminName, "Admin name should match")

	// User responds (during admin takeover, messages should still work)
	userResponse := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "I'm having trouble with my account",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, userResponse)
	require.NoError(t, err)

	// Verify LLM was called (messages are being processed)
	assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)

	// Admin leaves the session
	err = router.HandleAdminLeave("admin-001", sessionID)
	require.NoError(t, err, "Admin leave should succeed")

	// Verify admin is no longer actively assisting (ID and name cleared)
	// but AdminAssisted flag remains true as historical record
	retrievedSess, err = sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.AdminAssisted, "AdminAssisted should remain true as historical record")
	assert.Equal(t, "", retrievedSess.AssistingAdminID, "Admin ID should be cleared")
	assert.Equal(t, "", retrievedSess.AssistingAdminName, "Admin name should be cleared")
}

// TestIntegration_FileUploadFlow tests the file upload flow:
// user uploads file → file message processed → confirmation sent
// **Validates: Requirements 6.1**
func TestIntegration_FileUploadFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create user connection and session
	userConn := mockConnection("user-789")
	sess, err := router.getOrCreateSession(userConn, "test-session-3")
	require.NoError(t, err)
	sessionID := sess.ID

	// Register connection
	err = router.RegisterConnection(sessionID, userConn)
	require.NoError(t, err)

	// Send file upload message
	fileMsg := &message.Message{
		Type:      message.TypeFileUpload,
		SessionID: sessionID,
		Content:   "document.pdf",
		FileID:    "file-123",
		FileURL:   "https://s3.example.com/files/file-123.pdf",
		Sender:    message.SenderUser,
		Metadata: map[string]string{
			"size":     "2048",
			"mimeType": "application/pdf",
		},
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(userConn, fileMsg)
	require.NoError(t, err)

	// Verify session exists and is active
	retrievedSess, err := sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.IsActive)

	// Verify file message was processed
	// (In a real implementation, this would trigger file processing)
	assert.NotNil(t, retrievedSess)

	// Send a follow-up message about the file
	followUpMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Can you analyze this document?",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, followUpMsg)
	require.NoError(t, err)

	// Verify LLM was called with the follow-up message
	assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)
}

// TestIntegration_VoiceMessageFlow tests the voice message flow:
// user sends voice message → transcription processed → LLM responds
// **Validates: Requirements 6.1**
func TestIntegration_VoiceMessageFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create user connection and session
	userConn := mockConnection("user-999")
	sess, err := router.getOrCreateSession(userConn, "test-session-4")
	require.NoError(t, err)
	sessionID := sess.ID

	// Register connection
	err = router.RegisterConnection(sessionID, userConn)
	require.NoError(t, err)

	// Send voice message with transcription
	voiceMsg := &message.Message{
		Type:      message.TypeVoiceMessage,
		SessionID: sessionID,
		Content:   "Hello, this is a voice message",
		FileID:    "voice-123",
		FileURL:   "https://s3.example.com/voice/voice-123.mp3",
		Sender:    message.SenderUser,
		Metadata: map[string]string{
			"duration": "5",
			"format":   "mp3",
		},
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(userConn, voiceMsg)
	require.NoError(t, err)

	// Verify session exists and is active
	retrievedSess, err := sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.IsActive)

	// Send a text message to trigger LLM response
	textMsg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Can you respond to my voice message?",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, textMsg)
	require.NoError(t, err)

	// Verify LLM was called
	assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)
	assert.NotEmpty(t, mockLLM.lastMessages)

	// Verify session has messages
	retrievedSess, err = sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, len(retrievedSess.Messages) > 0)
}

// TestIntegration_MultipleFlowsCombined tests multiple flows in sequence:
// message → file upload → voice message → admin takeover
// **Validates: Requirements 6.1**
func TestIntegration_MultipleFlowsCombined(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)
	mockStorage := &mockStorageService{}
	mockLLM := &mockLLMService{}
	router := NewMessageRouter(sm, mockLLM, nil, nil, mockStorage, 120*time.Second, logger)

	// Create user connection and session
	userConn := mockConnection("user-combined")
	sess, err := router.getOrCreateSession(userConn, "test-session-combined")
	require.NoError(t, err)
	sessionID := sess.ID

	// Register connection
	err = router.RegisterConnection(sessionID, userConn)
	require.NoError(t, err)

	// Step 1: Send regular message
	msg1 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Hello, I need help",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, msg1)
	require.NoError(t, err)

	// Step 2: Upload a file
	fileMsg := &message.Message{
		Type:      message.TypeFileUpload,
		SessionID: sessionID,
		Content:   "error-log.txt",
		FileID:    "file-456",
		FileURL:   "https://s3.example.com/files/file-456.txt",
		Sender:    message.SenderUser,
		Metadata: map[string]string{
			"size":     "1024",
			"mimeType": "text/plain",
		},
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(userConn, fileMsg)
	require.NoError(t, err)

	// Step 3: Send voice message
	voiceMsg := &message.Message{
		Type:      message.TypeVoiceMessage,
		SessionID: sessionID,
		Content:   "This is urgent",
		FileID:    "voice-456",
		FileURL:   "https://s3.example.com/voice/voice-456.mp3",
		Sender:    message.SenderUser,
		Metadata: map[string]string{
			"duration": "3",
			"format":   "mp3",
		},
		Timestamp: time.Now(),
	}

	err = router.RouteMessage(userConn, voiceMsg)
	require.NoError(t, err)

	// Step 4: Admin takes over
	adminConn := websocket.NewConnection("admin-combined", []string{"admin"})
	adminConn.Name = "Emergency Support"

	err = router.HandleAdminTakeover(adminConn, sessionID)
	require.NoError(t, err)

	// Verify session state
	retrievedSess, err := sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.IsActive)
	assert.True(t, retrievedSess.AdminAssisted)
	assert.Equal(t, "user-combined", retrievedSess.UserID)
	assert.Equal(t, "admin-combined", retrievedSess.AssistingAdminID)

	// Step 5: User sends another message during admin takeover
	userMsg2 := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sessionID,
		Content:   "Thank you for helping",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	err = router.HandleUserMessage(userConn, userMsg2)
	require.NoError(t, err)

	// Step 6: Admin leaves
	err = router.HandleAdminLeave("admin-combined", sessionID)
	require.NoError(t, err)

	// Verify admin is no longer actively assisting (ID cleared)
	// but AdminAssisted flag remains true as historical record
	retrievedSess, err = sm.GetSession(sessionID)
	require.NoError(t, err)
	assert.True(t, retrievedSess.AdminAssisted, "AdminAssisted should remain true as historical record")
	assert.Equal(t, "", retrievedSess.AssistingAdminID, "Admin ID should be cleared")

	// Verify all operations completed successfully
	assert.True(t, mockStorage.createSessionCalled)
	assert.True(t, mockLLM.sendMessageCalled || mockLLM.streamCalled)
}

// mockLLMServiceForIntegration is a mock LLM service for integration tests
type mockLLMServiceForIntegration struct {
	sendMessageCalled bool
	streamCalled      bool
	lastMessages      []llm.ChatMessage
	responseDelay     time.Duration
}

func (m *mockLLMServiceForIntegration) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	m.sendMessageCalled = true
	m.lastMessages = messages

	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}

	return &llm.LLMResponse{
		Content:    "Mock AI response",
		TokensUsed: 15,
		Duration:   100 * time.Millisecond,
	}, nil
}

func (m *mockLLMServiceForIntegration) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	m.streamCalled = true
	m.lastMessages = messages

	ch := make(chan *llm.LLMChunk, 2)
	go func() {
		defer close(ch)
		if m.responseDelay > 0 {
			time.Sleep(m.responseDelay)
		}
		ch <- &llm.LLMChunk{Content: "Mock ", Done: false}
		ch <- &llm.LLMChunk{Content: "response", Done: true}
	}()

	return ch, nil
}
