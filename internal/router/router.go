// Package router provides message routing functionality for the WebSocket chat application.
// It routes messages to appropriate handlers based on message type and manages connections.
package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/yourusername/chat-websocket/internal/message"
	"github.com/yourusername/chat-websocket/internal/session"
	"github.com/yourusername/chat-websocket/internal/websocket"
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

// MessageRouter routes messages between clients, LLM backends, and admin users
type MessageRouter struct {
	// llmService will be implemented later
	// uploadService will be implemented later
	sessionManager *session.SessionManager
	connections    map[string]*websocket.Connection // sessionID -> Connection
	adminConns     map[string]*websocket.Connection // adminID -> Connection
	mu             sync.RWMutex
	// logger will be implemented later
}

// NewMessageRouter creates a new message router
func NewMessageRouter(sessionManager *session.SessionManager) *MessageRouter {
	return &MessageRouter{
		sessionManager: sessionManager,
		connections:    make(map[string]*websocket.Connection),
		adminConns:     make(map[string]*websocket.Connection),
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

	// Route based on message type
	switch msg.Type {
	case message.TypeUserMessage:
		return mr.HandleUserMessage(conn, msg)
	case message.TypeHelpRequest:
		return mr.handleHelpRequest(conn, msg)
	case message.TypeModelSelect:
		return mr.handleModelSelection(conn, msg)
	case message.TypeFileUpload:
		return mr.handleFileUpload(conn, msg)
	case message.TypeVoiceMessage:
		return mr.handleVoiceMessage(conn, msg)
	default:
		return fmt.Errorf("%w: unknown message type %s", ErrInvalidMessage, msg.Type)
	}
}

// HandleUserMessage processes user messages and forwards them to the LLM
// For now, this is a placeholder that will be fully implemented when LLM service is ready
func (mr *MessageRouter) HandleUserMessage(conn *websocket.Connection, msg *message.Message) error {
	if conn == nil {
		return ErrNilConnection
	}
	if msg == nil {
		return ErrNilMessage
	}

	// Validate session exists
	if msg.SessionID == "" {
		return fmt.Errorf("%w: session ID is required", ErrInvalidMessage)
	}

	_, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	// TODO: Forward to LLM service when implemented
	// For now, just log the message
	log.Printf("Routing user message from session %s: %s", msg.SessionID, msg.Content)

	// Placeholder: Echo back a simple response
	response := &message.Message{
		Type:      message.TypeAIResponse,
		SessionID: msg.SessionID,
		Content:   fmt.Sprintf("Received: %s", msg.Content),
		Sender:    message.SenderAI,
	}

	return mr.sendToConnection(msg.SessionID, response)
}

// handleHelpRequest processes help request messages
func (mr *MessageRouter) handleHelpRequest(conn *websocket.Connection, msg *message.Message) error {
	// TODO: Implement help request handling
	log.Printf("Help request from session %s", msg.SessionID)
	return nil
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
		return fmt.Errorf("%w: session ID is required", ErrInvalidMessage)
	}

	// Validate model ID
	if msg.ModelID == "" {
		return fmt.Errorf("%w: model ID is required", ErrInvalidMessage)
	}

	// Verify session exists
	_, err := mr.sessionManager.GetSession(msg.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	// Store the selected model in the session
	if err := mr.sessionManager.SetModelID(msg.SessionID, msg.ModelID); err != nil {
		return fmt.Errorf("failed to set model ID: %w", err)
	}

	log.Printf("Model selection for session %s: %s", msg.SessionID, msg.ModelID)

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
func (mr *MessageRouter) handleFileUpload(conn *websocket.Connection, msg *message.Message) error {
	// TODO: Implement file upload handling
	log.Printf("File upload for session %s: %s", msg.SessionID, msg.FileID)
	return nil
}

// handleVoiceMessage processes voice message uploads
func (mr *MessageRouter) handleVoiceMessage(conn *websocket.Connection, msg *message.Message) error {
	// TODO: Implement voice message handling
	log.Printf("Voice message for session %s: %s", msg.SessionID, msg.FileID)
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
		return ErrInvalidMessage
	}

	// Verify session exists
	sess, err := mr.sessionManager.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}

	// Send to user connection
	if err := mr.sendToConnection(sessionID, msg); err != nil {
		log.Printf("Failed to send to user connection: %v", err)
	}

	// If admin is assisting, send to admin connection too
	if sess.AssistingAdminID != "" {
		mr.mu.RLock()
		adminConn, exists := mr.adminConns[sess.AssistingAdminID]
		mr.mu.RUnlock()

		if exists {
			data, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal message: %w", err)
			}

			select {
			case adminConn.Send() <- data:
			default:
				log.Printf("Admin connection send channel is full for admin %s", sess.AssistingAdminID)
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
