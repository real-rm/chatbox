// Package websocket provides WebSocket connection handling with JWT authentication.
// It implements HTTP to WebSocket upgrade, connection management, and user context association.
package websocket

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/golog"
)

var (
	// upgrader configures the WebSocket upgrade
	// SECURITY: In production, this service MUST be deployed behind a reverse proxy
	// (nginx, traefik, etc.) that terminates TLS/SSL connections, ensuring all
	// WebSocket connections use the WSS (WebSocket Secure) protocol.
	// The CheckOrigin function should be configured to validate allowed origins.
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// TODO: Implement proper origin checking for production
			// For now, allow all origins (should be restricted in production)
			return true
		},
	}

	// Connection lifecycle timeouts
	// pongWait is the time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// pingPeriod is the interval for sending ping messages (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// writeWait is the time allowed to write a message to the peer
	writeWait = 10 * time.Second
)

// Connection represents an active WebSocket connection with user context
type Connection struct {
	// conn is the underlying WebSocket connection
	conn *websocket.Conn

	// UserID is the authenticated user's ID from JWT
	UserID string

	// SessionID is the current session identifier
	SessionID string

	// Roles are the user's roles from JWT
	Roles []string

	// send is a buffered channel for outbound messages
	send chan []byte

	// mu protects concurrent access to the connection
	mu sync.RWMutex
}

// NewConnection creates a new Connection for testing purposes
// This is primarily used in tests to create mock connections
func NewConnection(userID string, roles []string) *Connection {
	return &Connection{
		UserID: userID,
		Roles:  roles,
		send:   make(chan []byte, 256),
	}
}

// Handler manages WebSocket connections and upgrades
type Handler struct {
	validator   *auth.JWTValidator
	logger      *golog.Logger
	connLimiter *ratelimit.ConnectionLimiter

	// connections tracks active connections by user ID
	connections map[string]*Connection
	mu          sync.RWMutex
}

// NewHandler creates a new WebSocket handler
func NewHandler(validator *auth.JWTValidator, logger *golog.Logger) *Handler {
	wsLogger := logger.WithGroup("websocket")
	return &Handler{
		validator:   validator,
		logger:      wsLogger,
		connLimiter: ratelimit.NewConnectionLimiter(10), // Max 10 connections per user
		connections: make(map[string]*Connection),
	}
}

// HandleWebSocket handles HTTP to WebSocket upgrade requests
// It performs the following steps:
// 1. Extract JWT token from query parameter or header
// 2. Validate the JWT token
// 3. Upgrade the HTTP connection to WebSocket
// 4. Create a Connection struct with user context
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract token from query parameter or Authorization header
	token := r.URL.Query().Get("token")
	if token == "" {
		// Try Authorization header
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
	}

	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	// Validate JWT token
	claims, err := h.validator.ValidateToken(token)
	if err != nil {
		h.logger.Warn("JWT validation failed",
			"error", err,
			"component", "websocket")
		http.Error(w, fmt.Sprintf("Authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Check connection rate limit
	if !h.connLimiter.Allow(claims.UserID) {
		h.logger.Warn("Connection limit exceeded",
			"user_id", claims.UserID,
			"component", "websocket")

		chatErr := errors.ErrConnectionLimitExceeded(5000)
		http.Error(w, chatErr.Message, http.StatusTooManyRequests)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed",
			"error", err,
			"component", "websocket")
		return
	}

	// Create connection with user context
	connection := h.createConnection(conn, claims)

	// Register the connection
	h.registerConnection(connection)

	h.logger.Info("WebSocket connection established",
		"user_id", claims.UserID,
		"component", "websocket")

	// Start read and write pumps in goroutines
	go connection.readPump(h)
	go connection.writePump()
}

// createConnection creates a new Connection with user context from JWT claims
func (h *Handler) createConnection(conn *websocket.Conn, claims *auth.Claims) *Connection {
	return &Connection{
		conn:   conn,
		UserID: claims.UserID,
		Roles:  claims.Roles,
		send:   make(chan []byte, 256),
	}
}

// registerConnection adds a connection to the active connections map
func (h *Handler) registerConnection(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store connection by user ID
	h.connections[conn.UserID] = conn
}

// unregisterConnection removes a connection from the active connections map
func (h *Handler) unregisterConnection(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.connections[conn.UserID]; ok {
		delete(h.connections, conn.UserID)
		close(conn.send)

		// Release connection from rate limiter
		h.connLimiter.Release(conn.UserID)
	}
}

// Shutdown gracefully closes all active WebSocket connections
// It sends close messages to all connected clients and waits for them to close
func (h *Handler) Shutdown() {
	h.logger.Info("Shutting down WebSocket handler, closing all connections")

	h.mu.Lock()
	connections := make([]*Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.mu.Unlock()

	// Close all connections
	for _, conn := range connections {
		h.logger.Info("Closing WebSocket connection", "user_id", conn.UserID)

		// Send close message
		conn.mu.Lock()
		if conn.conn != nil {
			conn.conn.SetWriteDeadline(time.Now().Add(writeWait))
			conn.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
		}
		conn.mu.Unlock()

		// Close the connection
		conn.Close()
	}

	h.logger.Info("All WebSocket connections closed")
}

// Close gracefully closes the WebSocket connection and cleans up resources
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Send returns the send channel for this connection
// This allows external components to send messages to the connection
func (c *Connection) Send() chan<- []byte {
	return c.send
}

// readPump reads messages from the WebSocket connection
// It handles:
// - Setting read deadline based on pongWait
// - Configuring pong handler to reset read deadline
// - Reading messages from the client
// - Graceful cleanup on connection close or error
func (c *Connection) readPump(h *Handler) {
	defer func() {
		h.logger.Info("WebSocket connection closed",
			"user_id", c.UserID,
			"session_id", c.SessionID,
			"component", "websocket")
		h.unregisterConnection(c)
		c.Close()
	}()

	// Set initial read deadline
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	// Configure pong handler to reset read deadline
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		h.logger.Debug("Heartbeat pong received",
			"user_id", c.UserID,
			"session_id", c.SessionID,
			"component", "websocket")
		return nil
	})

	// Read messages in a loop
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error("WebSocket unexpected close",
					"user_id", c.UserID,
					"session_id", c.SessionID,
					"component", "websocket",
					"error", err)
			} else {
				h.logger.Info("WebSocket connection closing",
					"user_id", c.UserID,
					"session_id", c.SessionID,
					"component", "websocket")
			}
			break
		}

		// TODO: Process incoming message
		h.logger.Debug("Message received",
			"user_id", c.UserID,
			"session_id", c.SessionID,
			"component", "websocket",
			"message_length", len(message))
	}
}

// writePump writes messages to the WebSocket connection
// It handles:
// - Sending periodic ping messages for heartbeat
// - Writing messages from the send channel
// - Setting write deadlines
// - Graceful connection closure
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			// Set write deadline
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// Channel closed, send close message
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write the message
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			// Send ping message
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
