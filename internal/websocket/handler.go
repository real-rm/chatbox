// Package websocket provides WebSocket connection handling with JWT authentication.
// It implements HTTP to WebSocket upgrade, connection management, and user context association.
package websocket

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/golog"
)

var (
	// upgrader configures the WebSocket upgrade
	// SECURITY: In production, this service MUST be deployed behind a reverse proxy
	// (nginx, traefik, etc.) that terminates TLS/SSL connections, ensuring all
	// WebSocket connections use the WSS (WebSocket Secure) protocol.
	// The CheckOrigin function is configured per-handler to validate allowed origins.
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// CheckOrigin is set per-handler instance
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

	// ConnectionID is a unique identifier for this connection
	ConnectionID string

	// UserID is the authenticated user's ID from JWT
	UserID string

	// Name is the user's display name from JWT
	Name string

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
		Name:   userID, // Default to userID if name not provided
		Roles:  roles,
		send:   make(chan []byte, 256),
	}
}

// GetUserID returns the user ID for this connection
func (c *Connection) GetUserID() string {
	return c.UserID
}

// GetSessionID returns the session ID for this connection
func (c *Connection) GetSessionID() string {
	return c.SessionID
}

// GetRoles returns the roles for this connection
func (c *Connection) GetRoles() []string {
	return c.Roles
}

// Handler manages WebSocket connections and upgrades
type Handler struct {
	validator      *auth.JWTValidator
	logger         *golog.Logger
	connLimiter    *ratelimit.ConnectionLimiter
	router         MessageRouter // Add router for message processing
	allowedOrigins map[string]bool // Allowed origins for CORS

	// connections tracks active connections by user ID and connection ID
	connections map[string]map[string]*Connection
	mu          sync.RWMutex
}

// MessageRouter interface for routing messages
type MessageRouter interface {
	RouteMessage(conn *Connection, msg *message.Message) error
	RegisterConnection(sessionID string, conn *Connection) error
	UnregisterConnection(sessionID string)
}

// NewHandler creates a new WebSocket handler
func NewHandler(validator *auth.JWTValidator, router MessageRouter, logger *golog.Logger) *Handler {
	wsLogger := logger.WithGroup("websocket")
	return &Handler{
		validator:      validator,
		router:         router,
		logger:         wsLogger,
		connLimiter:    ratelimit.NewConnectionLimiter(10), // Max 10 connections per user
		allowedOrigins: make(map[string]bool),
		connections:    make(map[string]map[string]*Connection),
	}
}

// SetAllowedOrigins configures the allowed origins for WebSocket connections
// If no origins are set, all origins are allowed (development mode)
func (h *Handler) SetAllowedOrigins(origins []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.allowedOrigins = make(map[string]bool)
	for _, origin := range origins {
		h.allowedOrigins[origin] = true
	}
	
	h.logger.Info("Configured allowed origins",
		"count", len(origins),
		"origins", origins)
}

// checkOrigin validates the origin of a WebSocket upgrade request
func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	
	// If no origins configured, allow all (development mode)
	if len(h.allowedOrigins) == 0 {
		h.logger.Debug("No origin restrictions configured, allowing all origins")
		return true
	}
	
	// Check if origin is in allowed list
	if h.allowedOrigins[origin] {
		return true
	}
	
	h.logger.Warn("Origin not allowed",
		"origin", origin,
		"allowed_origins", h.allowedOrigins)
	return false
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
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	// Check connection rate limit
	if !h.connLimiter.Allow(claims.UserID) {
		h.logger.Warn("Connection limit exceeded",
			"user_id", claims.UserID,
			"component", "websocket")

		// Notify existing connections about the limit
		h.notifyConnectionLimit(claims.UserID)

		chatErr := chaterrors.ErrConnectionLimitExceeded(5000)
		http.Error(w, chatErr.Message, http.StatusTooManyRequests)
		return
	}

	// Upgrade HTTP connection to WebSocket
	localUpgrader := upgrader
	localUpgrader.CheckOrigin = h.checkOrigin
	
	conn, err := localUpgrader.Upgrade(w, r, nil)
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
	// Generate unique connection ID using random bytes for better uniqueness
	// The connection ID format: userID-nanosecondTimestamp-randomHex
	// This ensures uniqueness even for rapid connections from the same user
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-only if random generation fails (extremely rare)
		h.logger.Error("Failed to generate random bytes for connection ID, using timestamp only", "error", err)
		connectionID := fmt.Sprintf("%s-%d", claims.UserID, time.Now().UnixNano())
		return &Connection{
			conn:         conn,
			ConnectionID: connectionID,
			UserID:       claims.UserID,
			Name:         claims.Name,
			Roles:        claims.Roles,
			send:         make(chan []byte, 256),
		}
	}
	
	connectionID := fmt.Sprintf("%s-%d-%s", claims.UserID, time.Now().UnixNano(), hex.EncodeToString(randomBytes))
	
	return &Connection{
		conn:         conn,
		ConnectionID: connectionID,
		UserID:       claims.UserID,
		Name:         claims.Name,
		Roles:        claims.Roles,
		send:         make(chan []byte, 256),
	}
}

// registerConnection adds a connection to the active connections map
func (h *Handler) registerConnection(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Initialize user's connection map if it doesn't exist
	if h.connections[conn.UserID] == nil {
		h.connections[conn.UserID] = make(map[string]*Connection)
	}

	// Store connection by user ID and connection ID
	h.connections[conn.UserID][conn.ConnectionID] = conn
	
	// Increment WebSocket connections metric
	metrics.WebSocketConnections.Inc()
	
	h.logger.Info("Connection registered",
		"user_id", conn.UserID,
		"connection_id", conn.ConnectionID,
		"total_connections", len(h.connections[conn.UserID]))
}

// unregisterConnection removes a connection from the active connections map
func (h *Handler) unregisterConnection(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userConns, ok := h.connections[conn.UserID]; ok {
		if _, exists := userConns[conn.ConnectionID]; exists {
			delete(userConns, conn.ConnectionID)
			close(conn.send)
			
			// Release connection from rate limiter for each connection
			h.connLimiter.Release(conn.UserID)
			
			// Decrement WebSocket connections metric
			metrics.WebSocketConnections.Dec()

			// If no more connections for this user, remove the user entry
			if len(userConns) == 0 {
				delete(h.connections, conn.UserID)
			}
			
			h.logger.Info("Connection unregistered",
				"user_id", conn.UserID,
				"connection_id", conn.ConnectionID,
				"remaining_connections", len(userConns))
		}
	}
}

// notifyConnectionLimit sends a notification to all user's connections when connection limit is reached
func (h *Handler) notifyConnectionLimit(userID string) {
	h.mu.RLock()
	userConns, exists := h.connections[userID]
	h.mu.RUnlock()

	if !exists || len(userConns) == 0 {
		return
	}

	// Create notification message
	notificationMsg := &message.Message{
		Type:   message.TypeNotification,
		Sender: message.SenderSystem,
		Content: "Connection limit reached. You have reached the maximum number of simultaneous connections. " +
			"Close an existing connection to open a new one.",
		Timestamp: time.Now(),
	}

	notificationBytes, err := json.Marshal(notificationMsg)
	if err != nil {
		h.logger.Error("Failed to marshal connection limit notification",
			"user_id", userID,
			"error", err)
		return
	}

	// Send notification to all user's connections
	for connID, conn := range userConns {
		select {
		case conn.send <- notificationBytes:
			h.logger.Debug("Sent connection limit notification",
				"user_id", userID,
				"connection_id", connID)
		default:
			h.logger.Warn("Failed to send connection limit notification, channel full",
				"user_id", userID,
				"connection_id", connID)
		}
	}
}

// Shutdown gracefully closes all active WebSocket connections
// It sends close messages to all connected clients and waits for them to close
func (h *Handler) Shutdown() {
	h.logger.Info("Shutting down WebSocket handler, closing all connections")

	h.mu.Lock()
	connections := make([]*Connection, 0)
	for _, userConns := range h.connections {
		for _, conn := range userConns {
			connections = append(connections, conn)
		}
	}
	h.mu.Unlock()

	// Close all connections
	for _, conn := range connections {
		h.logger.Info("Closing WebSocket connection",
			"user_id", conn.UserID,
			"connection_id", conn.ConnectionID)

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

// ReceiveForTest returns the send channel as a receive channel for testing purposes
// This should only be used in tests to verify messages sent to the connection
func (c *Connection) ReceiveForTest() <-chan []byte {
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
		
		// Unregister from router if we have a session ID
		if c.SessionID != "" && h.router != nil {
			h.router.UnregisterConnection(c.SessionID)
		}
		
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
		_, rawMessage, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error("WebSocket unexpected close",
					"user_id", c.UserID,
					"session_id", c.SessionID,
					"connection_id", c.ConnectionID,
					"component", "websocket",
					"error", err)
			} else {
				h.logger.Info("WebSocket connection closing",
					"user_id", c.UserID,
					"session_id", c.SessionID,
					"connection_id", c.ConnectionID,
					"component", "websocket")
			}
			break
		}

		// Parse incoming message
		var msg message.Message
		if err := json.Unmarshal(rawMessage, &msg); err != nil {
			h.logger.Warn("Failed to parse message",
				"user_id", c.UserID,
				"connection_id", c.ConnectionID,
				"error", err)
			
			// Increment message errors metric
			metrics.MessageErrors.Inc()
			
			// Send error response to client
			errorMsg := &message.Message{
				Type:   message.TypeError,
				Sender: message.SenderAI,
				Error: &message.ErrorInfo{
					Code:        string(chaterrors.ErrCodeInvalidFormat),
					Message:     "Invalid message format",
					Recoverable: true,
				},
				Timestamp: time.Now(),
			}
			if errorBytes, err := json.Marshal(errorMsg); err == nil {
				c.send <- errorBytes
			}
			continue
		}

		// Set timestamp if not provided
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		// Set sender if not provided
		if msg.Sender == "" {
			msg.Sender = message.SenderUser
		}

		h.logger.Debug("Message received",
			"user_id", c.UserID,
			"session_id", c.SessionID,
			"connection_id", c.ConnectionID,
			"message_type", msg.Type,
			"component", "websocket")

		// Increment messages received metric
		metrics.MessagesReceived.Inc()

		// Route message to message router
		if h.router != nil {
			// If message has a session ID and connection doesn't have one yet, set it
			if msg.SessionID != "" && c.SessionID == "" {
				c.mu.Lock()
				c.SessionID = msg.SessionID
				c.mu.Unlock()
				
				// Register connection with router for this session
				if err := h.router.RegisterConnection(msg.SessionID, c); err != nil {
					h.logger.Error("Failed to register connection with router",
						"user_id", c.UserID,
						"session_id", msg.SessionID,
						"connection_id", c.ConnectionID,
						"error", err)
					
					// Send error response to client for registration failure
					errorMsg := &message.Message{
						Type:      message.TypeError,
						SessionID: msg.SessionID,
						Sender:    message.SenderAI,
						Error: &message.ErrorInfo{
							Code:        string(chaterrors.ErrCodeServiceError),
							Message:     "Failed to establish session connection",
							Recoverable: true,
						},
						Timestamp: time.Now(),
					}
					if errorBytes, err := json.Marshal(errorMsg); err == nil {
						select {
						case c.send <- errorBytes:
						default:
							h.logger.Warn("Failed to send registration error, channel full",
								"user_id", c.UserID,
								"connection_id", c.ConnectionID)
						}
					}
					continue
				}
				
				h.logger.Info("Connection registered with router",
					"user_id", c.UserID,
					"session_id", msg.SessionID,
					"connection_id", c.ConnectionID)
			}
			
			// Call router with the connection
			if err := h.router.RouteMessage(c, &msg); err != nil {
				// Log detailed error server-side
				h.logger.Error("Failed to route message",
					"user_id", c.UserID,
					"session_id", c.SessionID,
					"connection_id", c.ConnectionID,
					"message_type", msg.Type,
					"error", err)
				
				// Increment message errors metric
				metrics.MessageErrors.Inc()
				
				// Check if it's a ChatError for proper error handling
				var chatErr *chaterrors.ChatError
				var errorInfo *message.ErrorInfo
				
				if errors.As(err, &chatErr) {
					// Use the ChatError's error info
					errorInfo = chatErr.ToErrorInfo()
					
					h.logger.Debug("Routing error details",
						"error_code", chatErr.Code,
						"error_category", chatErr.Category,
						"recoverable", chatErr.Recoverable,
						"user_id", c.UserID,
						"session_id", c.SessionID)
				} else {
					// For non-ChatError errors, create a generic error response
					errorInfo = &message.ErrorInfo{
						Code:        string(chaterrors.ErrCodeServiceError),
						Message:     "Failed to process message",
						Recoverable: true,
					}
				}
				
				// Send error response to client
				errorMsg := &message.Message{
					Type:      message.TypeError,
					SessionID: msg.SessionID,
					Sender:    message.SenderAI,
					Error:     errorInfo,
					Timestamp: time.Now(),
				}
				
				if errorBytes, err := json.Marshal(errorMsg); err == nil {
					select {
					case c.send <- errorBytes:
					default:
						h.logger.Warn("Failed to send routing error, channel full",
							"user_id", c.UserID,
							"connection_id", c.ConnectionID)
					}
				} else {
					h.logger.Error("Failed to marshal error message",
						"user_id", c.UserID,
						"connection_id", c.ConnectionID,
						"error", err)
				}
			}
		} else {
			h.logger.Warn("No router configured, message not processed",
				"user_id", c.UserID,
				"connection_id", c.ConnectionID)
			
			// Send error response to client when router is not configured
			errorMsg := &message.Message{
				Type:      message.TypeError,
				SessionID: msg.SessionID,
				Sender:    message.SenderAI,
				Error: &message.ErrorInfo{
					Code:        string(chaterrors.ErrCodeServiceError),
					Message:     "Service temporarily unavailable",
					Recoverable: true,
				},
				Timestamp: time.Now(),
			}
			if errorBytes, err := json.Marshal(errorMsg); err == nil {
				select {
				case c.send <- errorBytes:
				default:
					h.logger.Warn("Failed to send router unavailable error, channel full",
						"user_id", c.UserID,
						"connection_id", c.ConnectionID)
				}
			}
		}
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

			// Write each message as a separate WebSocket frame
			// This ensures proper JSON parsing on the client side
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
			
			// Increment messages sent metric
			metrics.MessagesSent.Inc()

		case <-ticker.C:
			// Send ping message
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
