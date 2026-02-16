// Package router provides message routing functionality for the WebSocket chat application.
// It routes messages to appropriate handlers based on message type and manages connections.
package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/upload"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
)

var (
	// ErrInvalidMessage is returned when a message is invalid
	ErrInvalidMessage = errors.New("invalid message")
	// ErrSessionNotFound is returned when a session is not found
	ErrSessionNotFound = errors.New("session not found")
	// ErrConnectionNotFound is returned when a connection is not found
	ErrConnectionNotFound = errors.New("connection not found")
	// ErrNilConnection is returned when a nil connection is provided
	ErrNilConnection = errors.New("connection cannot be nil")
	// ErrNilMessage is returned when a nil message is provided
	ErrNilMessage = errors.New("message cannot be nil")
)

// LLMService interface for LLM operations (to avoid circular dependency)
type LLMService interface {
	SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error)
	StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error)
}

// NotificationService interface for notification operations (to avoid circular dependency)
type NotificationService interface {
	SendHelpRequestAlert(userID, sessionID string) error
}

// MessageRouter routes messages between clients, LLM backends, and admin users
type MessageRouter struct {
	llmService          LLMService
	uploadService       *upload.UploadService
	notificationService NotificationService
	sessionManager      *session.SessionManager
	messageLimiter      *ratelimit.MessageLimiter
	connections         map[string]*websocket.Connection // sessionID -> Connection
	adminConns          map[string]*websocket.Connection // adminID -> Connection
	mu                  sync.RWMutex
	logger              *golog.Logger
}

// NewMessageRouter creates a new message router
func NewMessageRouter(sessionManager *session.SessionManager, llmService LLMService, uploadService *upload.UploadService, notificationService NotificationService, logger *golog.Logger) *MessageRouter {
	routerLogger := logger.WithGroup("router")
	return &MessageRouter{
		sessionManager:      sessionManager,
		llmService:          llmService,
		uploadService:       uploadService,
		notificationService: notificationService,
		messageLimiter:      ratelimit.NewMessageLimiter(1*time.Minute, 100), // 100 messages per minute
		connections:         make(map[string]*websocket.Connection),
		adminConns:          make(map[string]*websocket.Connection),
		logger:              routerLogger,
	}
}

// RegisterConnection registers a connection for a session
func (mr *MessageRouter) RegisterConnection(sessionID string, conn *websocket.Connection) error {
	if conn == nil {
		return ErrNilConnection
	}
	if sessionID == "" {
		return ErrInvalidMessage
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.connections[sessionID] = conn
	return nil
}

// UnregisterConnection removes a connection for a session
func (mr *MessageRouter) UnregisterConnection(sessionID string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	delete(mr.connections, sessionID)
}

// RouteMessage routes a message to the appropriate handler based on message type
func (mr *MessageRouter) RouteMessage(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Check message rate limit for user messages
	if msg.Type == message.TypeUserMessage {
		if !mr.messageLimiter.Allow(conn.UserID) {
			retryAfter := mr.messageLimiter.GetRetryAfter(conn.UserID)
			mr.logger.Warn("Message rate limit exceeded",
				"user_id", conn.UserID,
				"session_id", msg.SessionID,
				"retry_after", retryAfter)

			chatErr := chaterrors.ErrTooManyRequests(retryAfter)
			mr.HandleError(msg.SessionID, chatErr)
			return chatErr
		}
	}

	// Route based on message type
	var err error
	switch msg.Type {
	case message.TypeUserMessage:
		err = mr.HandleUserMessage(conn, msg)
	case message.TypeHelpRequest:
		err = mr.handleHelpRequest(conn, msg)
	case message.TypeModelSelect:
		err = mr.handleModelSelection(conn, msg)
	case message.TypeFileUpload:
		err = mr.handleFileUpload(conn, msg)
	case message.TypeVoiceMessage:
		err = mr.handleVoiceMessage(conn, msg)
	default:
		err = chaterrors.ErrInvalidMessageFormat(
			fmt.Sprintf("unknown message type %s", msg.Type),
			nil,
		)
	}

	// Handle any errors that occurred
	if err != nil {
		mr.HandleError(msg.SessionID, err)
		return err // Still return the error for logging/testing
	}

	return nil
}

// HandleUserMessage processes user messages and forwards them to the LLM
func (mr *MessageRouter) HandleUserMessage(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session exists
	if msg.SessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	sess, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	mr.logger.Debug("Routing user message to LLM",
		"session_id", msg.SessionID,
		"content_length", len(msg.Content),
		"model_id", sess.ModelID)

	// Send loading indicator to client
	loadingMsg := &message.Message{
		Type:      message.TypeLoading,
		SessionID: msg.SessionID,
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}
	if err := mr.sendToConnection(msg.SessionID, loadingMsg); err != nil {
		mr.logger.Warn("Failed to send loading indicator", "error", err)
	}

	// Prepare messages for LLM (convert from message.Message to llm.ChatMessage)
	llmMessages := []llm.ChatMessage{
		{
			Role:    "user",
			Content: msg.Content,
		},
	}

	// Use default model if not set
	modelID := sess.ModelID
	if modelID == "" {
		modelID = "gpt-4" // Default model
	}

	// Forward to LLM service with streaming
	ctx := context.Background()
	startTime := time.Now()
	
	// Use streaming for real-time response
	chunkChan, err := mr.llmService.StreamMessage(ctx, modelID, llmMessages)
	if err != nil {
		mr.logger.Error("LLM service error",
			"session_id", msg.SessionID,
			"model_id", modelID,
			"error", err)
		
		// Create appropriate error based on the failure
		llmErr := chaterrors.ErrLLMUnavailable(err)
		
		// Send error response to client
		errorMsg := &message.Message{
			Type:      message.TypeError,
			SessionID: msg.SessionID,
			Sender:    message.SenderAI,
			Error:     llmErr.ToErrorInfo(),
			Timestamp: time.Now(),
		}
		return mr.sendToConnection(msg.SessionID, errorMsg)
	}

	// Stream response chunks to client
	var fullContent strings.Builder
	var tokenCount int
	
	for chunk := range chunkChan {
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			
			// Send chunk to client
			chunkMsg := &message.Message{
				Type:      message.TypeAIResponse,
				SessionID: msg.SessionID,
				Content:   chunk.Content,
				Sender:    message.SenderAI,
				ModelID:   modelID,
				Timestamp: time.Now(),
				Metadata: map[string]string{
					"streaming": "true",
					"done":      fmt.Sprintf("%t", chunk.Done),
				},
			}
			
			if err := mr.sendToConnection(msg.SessionID, chunkMsg); err != nil {
				mr.logger.Warn("Failed to send chunk to client",
					"session_id", msg.SessionID,
					"error", err)
			}
		}
		
		// If this is the final chunk, break
		if chunk.Done {
			break
		}
	}

	// Record response time
	responseTime := time.Since(startTime)
	mr.sessionManager.RecordResponseTime(msg.SessionID, responseTime)

	// Estimate token usage (rough estimate: ~4 chars per token)
	if fullContent.Len() > 0 {
		tokenCount = fullContent.Len() / 4
		mr.sessionManager.UpdateTokenUsage(msg.SessionID, tokenCount)
	}

	return nil
}

// handleHelpRequest processes help request messages
func (mr *MessageRouter) handleHelpRequest(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session ID
	if msg.SessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	// Mark session as requiring assistance
	if err := mr.sessionManager.MarkHelpRequested(msg.SessionID); err != nil {
		mr.logger.Error("Failed to mark help requested", "error", err, "session_id", msg.SessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	mr.logger.Info("Help request received",
		"session_id", msg.SessionID,
		"user_id", sess.UserID)

	// Send notification to admins
	if mr.notificationService != nil {
		go func() {
			if err := mr.notificationService.SendHelpRequestAlert(sess.UserID, msg.SessionID); err != nil {
				mr.logger.Error("Failed to send help request notification",
					"error", err,
					"session_id", msg.SessionID,
					"user_id", sess.UserID)
			}
		}()
	}

	// Send confirmation message back to user
	response := &message.Message{
		Type:      message.TypeConnectionStatus,
		SessionID: msg.SessionID,
		Content:   "Help request sent. An administrator will join your session shortly.",
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
	}

	return mr.sendToConnection(msg.SessionID, response)
}

// handleModelSelection processes model selection messages
func (mr *MessageRouter) handleModelSelection(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session ID
	if msg.SessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Validate model ID
	if msg.ModelID == "" {
		return chaterrors.ErrMissingField("model_id")
	}

	// Verify session exists
	_, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	// Store the selected model in the session
	if err := mr.sessionManager.SetModelID(msg.SessionID, msg.ModelID); err != nil {
		return chaterrors.ErrDatabaseError(err)
	}

	mr.logger.Info("Model selection", "session_id", msg.SessionID, "model_id", msg.ModelID)

	// Send confirmation message back to client
	response := &message.Message{
		Type:      message.TypeConnectionStatus,
		SessionID: msg.SessionID,
		Content:   fmt.Sprintf("Model changed to %s", msg.ModelID),
		Sender:    message.SenderAI,
	}

	return mr.sendToConnection(msg.SessionID, response)
}

// handleFileUpload processes file upload messages
// This handles file upload completion notifications from the client
func (mr *MessageRouter) handleFileUpload(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session ID
	if msg.SessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Validate file information
	if msg.FileID == "" {
		return chaterrors.ErrMissingField("file_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	mr.logger.Info("File upload completed",
		"session_id", msg.SessionID,
		"file_id", msg.FileID,
		"file_url", msg.FileURL,
		"user_id", sess.UserID)

	// Convert message.Message to session.Message for storage
	sessionMsg := &session.Message{
		Content:   msg.Content,
		Timestamp: time.Now(),
		Sender:    string(msg.Sender),
		FileID:    msg.FileID,
		FileURL:   msg.FileURL,
		Metadata:  msg.Metadata,
	}

	// Store file upload message in session
	if err := mr.sessionManager.AddMessage(msg.SessionID, sessionMsg); err != nil {
		mr.logger.Error("Failed to store file upload message", "error", err, "session_id", msg.SessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Broadcast file upload notification to all session participants
	notification := &message.Message{
		Type:      message.TypeFileUpload,
		SessionID: msg.SessionID,
		Content:   msg.Content, // Optional description
		FileID:    msg.FileID,
		FileURL:   msg.FileURL,
		Sender:    message.SenderUser,
		Metadata:  msg.Metadata,
		Timestamp: time.Now(),
	}

	if err := mr.BroadcastToSession(msg.SessionID, notification); err != nil {
		mr.logger.Warn("Failed to broadcast file upload", "error", err, "session_id", msg.SessionID)
	}

	return nil
}

// handleVoiceMessage processes voice message uploads and forwards to LLM for transcription
func (mr *MessageRouter) handleVoiceMessage(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session ID
	if msg.SessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Validate file information
	if msg.FileID == "" {
		return chaterrors.ErrMissingField("file_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	mr.logger.Info("Voice message uploaded",
		"session_id", msg.SessionID,
		"file_id", msg.FileID,
		"file_url", msg.FileURL,
		"user_id", sess.UserID)

	// Convert message.Message to session.Message for storage
	sessionMsg := &session.Message{
		Content:   msg.Content,
		Timestamp: time.Now(),
		Sender:    string(msg.Sender),
		FileID:    msg.FileID,
		FileURL:   msg.FileURL,
		Metadata:  msg.Metadata,
	}

	// Store voice message in session
	if err := mr.sessionManager.AddMessage(msg.SessionID, sessionMsg); err != nil {
		mr.logger.Error("Failed to store voice message", "error", err, "session_id", msg.SessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Broadcast voice message notification to all session participants
	notification := &message.Message{
		Type:      message.TypeVoiceMessage,
		SessionID: msg.SessionID,
		Content:   msg.Content, // Optional transcription
		FileID:    msg.FileID,
		FileURL:   msg.FileURL,
		Sender:    message.SenderUser,
		Metadata:  msg.Metadata,
		Timestamp: time.Now(),
	}

	if err := mr.BroadcastToSession(msg.SessionID, notification); err != nil {
		mr.logger.Warn("Failed to broadcast voice message", "error", err, "session_id", msg.SessionID)
	}

	// Forward audio file reference to LLM for transcription/processing if LLM service is available
	if mr.llmService != nil && sess.ModelID != "" {
		go mr.processVoiceMessageWithLLM(msg.SessionID, msg.FileURL, sess.ModelID)
	}

	return nil
}

// processVoiceMessageWithLLM forwards the voice message to LLM for transcription
func (mr *MessageRouter) processVoiceMessageWithLLM(sessionID string, audioFileURL string, modelID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create a message indicating the audio file for the LLM
	// Note: The actual transcription capability depends on the LLM provider
	llmMessages := []llm.ChatMessage{
		{
			Role:    "user",
			Content: fmt.Sprintf("Audio file: %s", audioFileURL),
		},
	}

	mr.logger.Info("Forwarding voice message to LLM",
		"session_id", sessionID,
		"audio_url", audioFileURL,
		"model_id", modelID)

	// Send to LLM for processing
	resp, err := mr.llmService.SendMessage(ctx, modelID, llmMessages)
	if err != nil {
		mr.logger.Error("Failed to process voice message with LLM",
			"error", err,
			"session_id", sessionID)
		return
	}

	// If LLM provides a response (transcription or processing result), send it back
	if resp.Content != "" {
		aiMessage := &message.Message{
			Type:      message.TypeAIResponse,
			SessionID: sessionID,
			Content:   resp.Content,
			Sender:    message.SenderAI,
			Timestamp: time.Now(),
		}

		// Store AI response
		sessionMsg := &session.Message{
			Content:   resp.Content,
			Timestamp: time.Now(),
			Sender:    string(message.SenderAI),
		}
		if err := mr.sessionManager.AddMessage(sessionID, sessionMsg); err != nil {
			mr.logger.Error("Failed to store AI response", "error", err, "session_id", sessionID)
		}

		// Broadcast AI response
		if err := mr.BroadcastToSession(sessionID, aiMessage); err != nil {
			mr.logger.Warn("Failed to broadcast AI response", "error", err, "session_id", sessionID)
		}

		// Update token usage
		if err := mr.sessionManager.UpdateTokenUsage(sessionID, resp.TokensUsed); err != nil {
			mr.logger.Warn("Failed to update token usage", "error", err, "session_id", sessionID)
		}

		// Record response time
		if err := mr.sessionManager.RecordResponseTime(sessionID, resp.Duration); err != nil {
			mr.logger.Warn("Failed to record response time", "error", err, "session_id", sessionID)
		}
	}
}

// SendFileUploadError sends a file upload error message to the client
func (mr *MessageRouter) SendFileUploadError(sessionID string, errorCode string, errorMsg string) error {
	if sessionID == "" {
		return ErrInvalidMessage
	}

	chatErr := chaterrors.NewValidationError(
		chaterrors.ErrorCode(errorCode),
		errorMsg,
		nil,
	)

	errorMessage := &message.Message{
		Type:      message.TypeError,
		SessionID: sessionID,
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
		Error:     chatErr.ToErrorInfo(),
	}

	mr.logger.Warn("File upload error",
		"session_id", sessionID,
		"error_code", errorCode,
		"error_message", errorMsg)

	return mr.sendToConnection(sessionID, errorMessage)
}

// HandleAIGeneratedFile handles files generated by the LLM backend
// This is called when the LLM generates a file (image, document, etc.)
func (mr *MessageRouter) HandleAIGeneratedFile(sessionID string, fileURL string, fileDescription string, metadata map[string]string) error {
	if sessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	if fileURL == "" {
		return chaterrors.ErrMissingField("file_url")
	}

	// Verify session exists
	_, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	mr.logger.Info("AI generated file",
		"session_id", sessionID,
		"file_url", fileURL,
		"description", fileDescription)

	// Convert to session.Message for storage
	sessionMsg := &session.Message{
		Content:   fileDescription,
		Timestamp: time.Now(),
		Sender:    string(message.SenderAI),
		FileURL:   fileURL,
		Metadata:  metadata,
	}

	// Store AI message in session
	if err := mr.sessionManager.AddMessage(sessionID, sessionMsg); err != nil {
		mr.logger.Error("Failed to store AI generated file message", "error", err, "session_id", sessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Create AI response message with file
	aiMessage := &message.Message{
		Type:      message.TypeAIResponse,
		SessionID: sessionID,
		Content:   fileDescription,
		FileURL:   fileURL,
		Sender:    message.SenderAI,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}

	// Broadcast to all session participants
	if err := mr.BroadcastToSession(sessionID, aiMessage); err != nil {
		mr.logger.Warn("Failed to broadcast AI generated file", "error", err, "session_id", sessionID)
	}

	return nil
}

// HandleAIVoiceResponse handles voice responses generated by the LLM backend
// This is called when the LLM generates a voice/audio response
func (mr *MessageRouter) HandleAIVoiceResponse(sessionID string, audioFileURL string, transcription string, metadata map[string]string) error {
	if sessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	if audioFileURL == "" {
		return chaterrors.ErrMissingField("audio_file_url")
	}

	// Verify session exists
	_, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	mr.logger.Info("AI voice response generated",
		"session_id", sessionID,
		"audio_url", audioFileURL,
		"transcription", transcription)

	// Convert to session.Message for storage
	sessionMsg := &session.Message{
		Content:   transcription,
		Timestamp: time.Now(),
		Sender:    string(message.SenderAI),
		FileURL:   audioFileURL,
		Metadata:  metadata,
	}

	// Store AI voice message in session
	if err := mr.sessionManager.AddMessage(sessionID, sessionMsg); err != nil {
		mr.logger.Error("Failed to store AI voice response", "error", err, "session_id", sessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Create AI voice response message
	aiMessage := &message.Message{
		Type:      message.TypeAIResponse,
		SessionID: sessionID,
		Content:   transcription,
		FileURL:   audioFileURL,
		Sender:    message.SenderAI,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}

	// Broadcast to all session participants
	if err := mr.BroadcastToSession(sessionID, aiMessage); err != nil {
		mr.logger.Warn("Failed to broadcast AI voice response", "error", err, "session_id", sessionID)
	}

	return nil
}

// sendToConnection sends a message to a specific session's connection
func (mr *MessageRouter) sendToConnection(sessionID string, msg *message.Message) error {
	mr.mu.RLock()
	conn, exists := mr.connections[sessionID]
	mr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: session %s", ErrConnectionNotFound, sessionID)
	}

	// Marshal message to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send to connection's send channel
	select {
	case conn.Send() <- data:
		return nil
	default:
		return fmt.Errorf("connection send channel is full for session %s", sessionID)
	}
}

// BroadcastToSession sends a message to all participants in a session
// This includes the user and any admin who has taken over the session
func (mr *MessageRouter) BroadcastToSession(sessionID string, msg *message.Message) error {
	if msg == nil {
		return ErrNilMessage
	}
	if sessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	// Send to user connection
	if err := mr.sendToConnection(sessionID, msg); err != nil {
		mr.logger.Warn("Failed to send to user connection", "error", err, "session_id", sessionID)
	}

	// If admin is assisting, send to admin connection too
	if sess.AssistingAdminID != "" {
		mr.mu.RLock()
		adminConn, exists := mr.adminConns[sess.AssistingAdminID]
		mr.mu.RUnlock()

		if exists {
			data, err := json.Marshal(msg)
			if err != nil {
				return chaterrors.ErrInvalidMessageFormat("failed to marshal message", err)
			}

			select {
			case adminConn.Send() <- data:
			default:
				mr.logger.Warn("Admin connection send channel full", "admin_id", sess.AssistingAdminID)
			}
		}
	}

	return nil
}

// GetConnection retrieves a connection by session ID
func (mr *MessageRouter) GetConnection(sessionID string) (*websocket.Connection, error) {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	conn, exists := mr.connections[sessionID]
	if !exists {
		return nil, fmt.Errorf("%w: session %s", ErrConnectionNotFound, sessionID)
	}

	return conn, nil
}

// HandleAdminTakeover handles an admin taking over a user session
// This establishes a connection for the admin to the user's session
func (mr *MessageRouter) HandleAdminTakeover(adminConn *websocket.Connection, sessionID string) error {
	if adminConn == nil {
		return ErrNilConnection
	}
	if sessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	// Check if another admin is already assisting
	if sess.AssistingAdminID != "" && sess.AssistingAdminID != adminConn.UserID {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeInvalidFormat,
			fmt.Sprintf("Session is already being assisted by admin %s (%s)",
				sess.AssistingAdminName, sess.AssistingAdminID),
			nil,
		)
	}

	// Get admin name from connection (extracted from JWT claims)
	adminName := adminConn.Name
	if adminName == "" {
		adminName = adminConn.UserID // Fallback to user ID if name not available
	}

	// Mark session as admin-assisted
	if err := mr.sessionManager.MarkAdminAssisted(sessionID, adminConn.UserID, adminName); err != nil {
		mr.logger.Error("Failed to mark admin assisted", "error", err, "session_id", sessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Register admin connection
	mr.mu.Lock()
	mr.adminConns[adminConn.UserID] = adminConn
	mr.mu.Unlock()
	
	// Increment admin takeover metric
	metrics.AdminTakeovers.Inc()

	mr.logger.Info("Admin takeover initiated",
		"session_id", sessionID,
		"admin_id", adminConn.UserID,
		"admin_name", adminName,
		"user_id", sess.UserID)

	// Send admin join message to user
	adminJoinMsg := &message.Message{
		Type:      message.TypeAdminJoin,
		SessionID: sessionID,
		Content:   fmt.Sprintf("Administrator %s has joined the session", adminName),
		Sender:    message.SenderAdmin,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"admin_id":   adminConn.UserID,
			"admin_name": adminName,
		},
	}

	if err := mr.BroadcastToSession(sessionID, adminJoinMsg); err != nil {
		mr.logger.Warn("Failed to broadcast admin join message", "error", err, "session_id", sessionID)
	}

	return nil
}

// HandleAdminLeave handles an admin leaving a user session
func (mr *MessageRouter) HandleAdminLeave(adminID, sessionID string) error {
	if adminID == "" {
		return chaterrors.ErrMissingField("admin_id")
	}
	if sessionID == "" {
		return chaterrors.ErrMissingField("session_id")
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeMissingField,
			"Session not found",
			err,
		)
	}

	// Verify this admin is assisting the session
	if sess.AssistingAdminID != adminID {
		return chaterrors.NewValidationError(
			chaterrors.ErrCodeInvalidFormat,
			fmt.Sprintf("Admin %s is not assisting session %s", adminID, sessionID),
			nil,
		)
	}

	adminName := sess.AssistingAdminName

	// Clear admin assistance
	if err := mr.sessionManager.ClearAdminAssistance(sessionID); err != nil {
		mr.logger.Error("Failed to clear admin assistance", "error", err, "session_id", sessionID)
		return chaterrors.ErrDatabaseError(err)
	}

	// Unregister admin connection
	mr.mu.Lock()
	delete(mr.adminConns, adminID)
	mr.mu.Unlock()

	mr.logger.Info("Admin left session",
		"session_id", sessionID,
		"admin_id", adminID,
		"admin_name", adminName,
		"user_id", sess.UserID)

	// Send admin leave message to user
	adminLeaveMsg := &message.Message{
		Type:      message.TypeAdminLeave,
		SessionID: sessionID,
		Content:   fmt.Sprintf("Administrator %s has left the session", adminName),
		Sender:    message.SenderAdmin,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"admin_id":   adminID,
			"admin_name": adminName,
		},
	}

	if err := mr.sendToConnection(sessionID, adminLeaveMsg); err != nil {
		mr.logger.Warn("Failed to send admin leave message", "error", err, "session_id", sessionID)
	}

	return nil
}

// RegisterAdminConnection registers an admin connection
func (mr *MessageRouter) RegisterAdminConnection(adminID string, conn *websocket.Connection) error {
	if conn == nil {
		return ErrNilConnection
	}
	if adminID == "" {
		return ErrInvalidMessage
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.adminConns[adminID] = conn
	return nil
}

// UnregisterAdminConnection removes an admin connection
func (mr *MessageRouter) UnregisterAdminConnection(adminID string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	delete(mr.adminConns, adminID)
}

// HandleError handles errors by sending appropriate error messages to the client
// For fatal errors, it closes the connection after sending the error message
func (mr *MessageRouter) HandleError(sessionID string, err error) error {
	if err == nil {
		return nil
	}

	// Check if it's a ChatError
	var chatErr *chaterrors.ChatError
	if errors.As(err, &chatErr) {
		return mr.handleChatError(sessionID, chatErr)
	}

	// For non-ChatError errors, wrap as a generic service error
	genericErr := chaterrors.NewServiceError(
		chaterrors.ErrCodeServiceError,
		"An unexpected error occurred",
		err,
	)
	return mr.handleChatError(sessionID, genericErr)
}

// handleChatError handles a ChatError by sending an error message
// and closing the connection if the error is fatal
func (mr *MessageRouter) handleChatError(sessionID string, chatErr *chaterrors.ChatError) error {
	// Log the error with full context
	mr.logger.Error("Error occurred",
		"session_id", sessionID,
		"error_code", chatErr.Code,
		"error_category", chatErr.Category,
		"error_message", chatErr.Message,
		"recoverable", chatErr.Recoverable,
		"cause", chatErr.Cause)

	// Create error message
	errorMsg := &message.Message{
		Type:      message.TypeError,
		SessionID: sessionID,
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
		Error:     chatErr.ToErrorInfo(),
	}

	// Send error message to client
	if err := mr.sendToConnection(sessionID, errorMsg); err != nil {
		mr.logger.Warn("Failed to send error message to client",
			"session_id", sessionID,
			"error", err)
	}

	// If fatal error, close the connection
	if chatErr.IsFatal() {
		mr.logger.Info("Fatal error, closing connection",
			"session_id", sessionID,
			"error_code", chatErr.Code)

		// Get connection and close it
		mr.mu.RLock()
		conn, exists := mr.connections[sessionID]
		mr.mu.RUnlock()

		if exists {
			// Give a brief moment for the error message to be sent
			time.Sleep(100 * time.Millisecond)

			// Close the connection
			if err := conn.Close(); err != nil {
				mr.logger.Warn("Failed to close connection",
					"session_id", sessionID,
					"error", err)
			}

			// Unregister the connection
			mr.UnregisterConnection(sessionID)
		}
	}

	return nil
}

// SendErrorMessage sends an error message to a session
// This is a convenience method for sending error messages without handling fatal errors
func (mr *MessageRouter) SendErrorMessage(sessionID string, code chaterrors.ErrorCode, errorMessage string, recoverable bool) error {
	chatErr := &chaterrors.ChatError{
		Code:        code,
		Message:     errorMessage,
		Recoverable: recoverable,
	}

	errorMsg := &message.Message{
		Type:      message.TypeError,
		SessionID: sessionID,
		Sender:    message.SenderAI,
		Timestamp: time.Now(),
		Error:     chatErr.ToErrorInfo(),
	}

	return mr.sendToConnection(sessionID, errorMsg)
}
