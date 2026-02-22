// Package websocket provides WebSocket connection handling with JWT authentication.
// It implements HTTP to WebSocket upgrade, connection management, and user context association.
package websocket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/real-rm/chatbox/internal/auth"
	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/util"
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

	// closing indicates the connection is being torn down.
	// Set before closing the send channel to prevent send-on-closed-channel panics.
	closing atomic.Bool

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
	c.mu.RLock()
	defer c.mu.RUnlock()
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
	router         MessageRouter   // Add router for message processing
	allowedOrigins map[string]bool // Allowed origins for CORS
	maxMessageSize int64           // Maximum message size in bytes

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
func NewHandler(validator *auth.JWTValidator, router MessageRouter, logger *golog.Logger, maxMessageSize int64) *Handler {
	wsLogger := logger.WithGroup("websocket")
	return &Handler{
		validator:      validator,
		router:         router,
		logger:         wsLogger,
		connLimiter:    ratelimit.NewConnectionLimiter(10), // Max 10 connections per user
		allowedOrigins: make(map[string]bool),
		maxMessageSize: maxMessageSize,
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

// IsOpenOrigin returns true when no allowed origins are configured,
// meaning all origins are accepted. Callers can use this to log security
// warnings or enforce stricter policies at the application level.
// SECURITY: When true, any website can establish WebSocket connections.
// This is acceptable only when the service sits behind a reverse proxy
// that performs its own origin validation.
func (h *Handler) IsOpenOrigin() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.allowedOrigins) == 0
}

// checkOrigin validates the origin of a WebSocket upgrade request
func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	h.mu.RLock()
	defer h.mu.RUnlock()

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
	// Extract token: prefer Authorization header, fall back to query parameter
	var token string
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}
	if token == "" {
		token = r.URL.Query().Get("token")
		if token != "" {
			h.logger.Warn("JWT provided via query parameter (deprecated, use Authorization header)",
				"component", "websocket")
		}
	}

	// No else needed: early return pattern (guard clause)
	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusUnauthorized)
		return
	}

	// Validate JWT token
	claims, err := h.validator.ValidateToken(token)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		h.logger.Warn("JWT validation failed",
			"error", err,
			"component", "websocket")
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	// Check connection rate limit
	// No else needed: early return pattern (guard clause)
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
	// No else needed: early return pattern (guard clause)
	if err != nil {
		util.LogError(h.logger, "websocket", "upgrade connection", err)
		return
	}

	// Set read limit to prevent memory exhaustion from oversized messages
	conn.SetReadLimit(h.maxMessageSize)

	// Create connection with user context
	connection := h.createConnection(conn, claims)

	// Register the connection
	h.registerConnection(connection)

	h.logger.Info("WebSocket connection established",
		"user_id", claims.UserID,
		"component", "websocket")

	// Start read and write pumps in goroutines with panic recovery
	util.SafeGo(h.logger, "readPump", func() { connection.readPump(h) })
	util.SafeGo(h.logger, "writePump", func() { connection.writePump() })
}

// createConnection creates a new Connection with user context from JWT claims
func (h *Handler) createConnection(conn *websocket.Conn, claims *auth.Claims) *Connection {
	// Generate unique connection ID using random bytes for better uniqueness
	// The connection ID format: userID-nanosecondTimestamp-randomHex
	// This ensures uniqueness even for rapid connections from the same user
	randomBytes := make([]byte, 8)
	// No else needed: fallback logic for rare error case
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-only if random generation fails (extremely rare)
		util.LogError(h.logger, "websocket", "generate random bytes for connection ID", err)
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

	// No else needed: initialize if needed (lazy initialization)
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

// RegisterConnectionForTest registers a connection for testing purposes
// This should only be used in tests
func (h *Handler) RegisterConnectionForTest(conn *Connection) {
	h.registerConnection(conn)
}

// unregisterConnection removes a connection from the active connections map
func (h *Handler) unregisterConnection(conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userConns, ok := h.connections[conn.UserID]; ok {
		if _, exists := userConns[conn.ConnectionID]; exists {
			delete(userConns, conn.ConnectionID)
			conn.closing.Store(true)
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
	// Take a snapshot of the connections under the lock to avoid holding it during channel sends.
	h.mu.RLock()
	userConns := h.connections[userID]
	if len(userConns) == 0 {
		h.mu.RUnlock()
		return
	}
	type connEntry struct {
		id   string
		conn *Connection
	}
	snapshot := make([]connEntry, 0, len(userConns))
	for id, c := range userConns {
		snapshot = append(snapshot, connEntry{id: id, conn: c})
	}
	h.mu.RUnlock()

	// Create notification message
	notificationMsg := &message.Message{
		Type:   message.TypeNotification,
		Sender: message.SenderSystem,
		Content: "Connection limit reached. You have reached the maximum number of simultaneous connections. " +
			"Close an existing connection to open a new one.",
		Timestamp: time.Now(),
	}

	notificationBytes, err := json.Marshal(notificationMsg)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		util.LogError(h.logger, "websocket", "marshal connection limit notification", err,
			"user_id", userID)
		return
	}

	// Send notification to all user's connections using the snapshot
	for _, entry := range snapshot {
		if entry.conn.SafeSend(notificationBytes) {
			h.logger.Debug("Sent connection limit notification",
				"user_id", userID,
				"connection_id", entry.id)
		} else {
			h.logger.Warn("Failed to send connection limit notification, channel full or closing",
				"user_id", userID,
				"connection_id", entry.id)
		}
	}
}

// Shutdown gracefully closes all active WebSocket connections
// It sends close messages to all connected clients and waits for them to close
// Deprecated: Use ShutdownWithContext instead
func (h *Handler) Shutdown() {
	ctx := context.Background()
	h.ShutdownWithContext(ctx)
}

// ShutdownWithContext gracefully closes all active WebSocket connections
// It respects the context deadline and will force shutdown if the deadline is exceeded
func (h *Handler) ShutdownWithContext(ctx context.Context) error {
	h.logger.Info("Shutting down WebSocket handler, closing all connections")

	// Get all connections
	h.mu.Lock()
	connections := make([]*Connection, 0)
	for _, userConns := range h.connections {
		for _, conn := range userConns {
			connections = append(connections, conn)
		}
	}
	h.mu.Unlock()

	// Close connections in parallel with context deadline
	var wg sync.WaitGroup
	errChan := make(chan error, len(connections))

	for _, conn := range connections {
		wg.Add(1)
		go func(c *Connection) {
			defer wg.Done()

			h.logger.Info("Closing WebSocket connection",
				"user_id", c.UserID,
				"connection_id", c.ConnectionID)

			// Send close message
			c.mu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				c.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
			}
			c.mu.Unlock()

			// Close the connection
			if err := c.Close(); err != nil {
				errChan <- err
			}
		}(conn)
	}

	// Wait for all closures or context deadline
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		h.logger.Info("All WebSocket connections closed gracefully")
		return nil
	case <-ctx.Done():
		h.logger.Warn("Shutdown deadline exceeded, forcing closure",
			"remaining_connections", len(connections))
		return ctx.Err()
	}
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

// SetClosing marks the connection as closing.
// After this call, SafeSend will return false.
func (c *Connection) SetClosing() {
	c.closing.Store(true)
}

// SafeSend attempts to send data to the connection's send channel.
// Returns false if the connection is closing or the channel is full.
// This is the preferred method for sending data to avoid panics on closed channels.
func (c *Connection) SafeSend(data []byte) bool {
	if c.closing.Load() {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
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
// sendErrorResponse sends a structured error message to the client via the send channel.
// Uses a select/default guard to avoid blocking if the channel is full.
func (c *Connection) sendErrorResponse(code chaterrors.ErrorCode, msg string) {
	errorMsg := &message.Message{
		Type:   message.TypeError,
		Sender: message.SenderAI,
		Error: &message.ErrorInfo{
			Code:        string(code),
			Message:     msg,
			Recoverable: true,
		},
		Timestamp: time.Now(),
	}
	if errorBytes, err := json.Marshal(errorMsg); err == nil {
		select {
		case c.send <- errorBytes:
		default:
		}
	}
}

// - Setting read deadline based on pongWait
// - Configuring pong handler to reset read deadline
// - Reading messages from the client
// - Graceful cleanup on connection close or error
func (c *Connection) readPump(h *Handler) {
	defer func() {
		sid := c.GetSessionID()
		h.logger.Info("WebSocket connection closed",
			"user_id", c.UserID,
			"session_id", sid,
			"component", "websocket")

		// Unregister from router if we have a session ID
		if sid != "" && h.router != nil {
			h.router.UnregisterConnection(sid)
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
			"session_id", c.GetSessionID(),
			"component", "websocket")
		return nil
	})

	// Read messages in a loop
	for {
		_, rawMessage, err := c.conn.ReadMessage()
		// No else needed: error handling with break (exits loop)
		if err != nil {
			// No else needed: specific error handling (logs and continues to break)
			// Check if error is due to message size limit exceeded
			if errors.Is(err, websocket.ErrReadLimit) {
				h.logger.Warn("WebSocket message size limit exceeded",
					"user_id", c.UserID,
					"connection_id", c.ConnectionID,
					"limit", h.maxMessageSize,
					"component", "websocket")
			} else if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				util.LogError(h.logger, "websocket", "handle unexpected close", err,
					"user_id", c.UserID,
					"session_id", c.GetSessionID(),
					"connection_id", c.ConnectionID)
			} else {
				h.logger.Info("WebSocket connection closing",
					"user_id", c.UserID,
					"session_id", c.GetSessionID(),
					"connection_id", c.ConnectionID,
					"component", "websocket")
			}
			break
		}

		// Parse incoming message
		var msg message.Message
		// No else needed: error handling with continue (skips to next iteration)
		if err := json.Unmarshal(rawMessage, &msg); err != nil {
			h.logger.Warn("Failed to parse message",
				"user_id", c.UserID,
				"connection_id", c.ConnectionID,
				"error", err)

			// Increment message errors metric
			metrics.MessageErrors.Inc()

			// Send error response to client
			c.sendErrorResponse(chaterrors.ErrCodeInvalidFormat, "Invalid message format")
			continue
		}

		// CRITICAL FIX C2: Sanitize incoming message to prevent XSS
		msg.Sanitize()

		// Set defaults before validation (clients may omit these optional fields)
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}
		if msg.Sender == "" {
			msg.Sender = message.SenderUser
		}

		// Validate message fields (type, required fields, length constraints)
		if err := msg.Validate(); err != nil {
			h.logger.Warn("Message validation failed",
				"user_id", c.UserID,
				"connection_id", c.ConnectionID,
				"error", err)

			metrics.MessageErrors.Inc()

			c.sendErrorResponse(chaterrors.ErrCodeInvalidFormat, "Message validation failed")
			continue
		}

		h.logger.Debug("Message received",
			"user_id", c.UserID,
			"session_id", c.GetSessionID(),
			"connection_id", c.ConnectionID,
			"message_type", msg.Type,
			"component", "websocket")

		// Increment messages received metric
		metrics.MessagesReceived.Inc()

		// Route message to message router
		// No else needed: router is required for message processing
		if h.router != nil {
			// If message has a session ID and connection doesn't have one yet, set it.
			// Both check and assign are under the same lock to avoid a data race.
			if msg.SessionID != "" {
				needsRegister := false
				c.mu.Lock()
				if c.SessionID == "" {
					c.SessionID = msg.SessionID
					needsRegister = true
				}
				c.mu.Unlock()

				if needsRegister {
					if err := h.router.RegisterConnection(msg.SessionID, c); err != nil {
						util.LogError(h.logger, "websocket", "register connection with router", err,
							"user_id", c.UserID,
							"session_id", msg.SessionID,
							"connection_id", c.ConnectionID)

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
			}

			// No else needed: error handling (logs and sends error response)
			// Call router with the connection
			if err := h.router.RouteMessage(c, &msg); err != nil {
				// Log detailed error server-side
				util.LogError(h.logger, "websocket", "route message", err,
					"user_id", c.UserID,
					"session_id", c.GetSessionID(),
					"connection_id", c.ConnectionID,
					"message_type", msg.Type)

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
						"session_id", c.GetSessionID())
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
					util.LogError(h.logger, "websocket", "marshal error message", err,
						"user_id", c.UserID,
						"connection_id", c.ConnectionID)
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

			// No else needed: channel closed handling (sends close and returns)
			if !ok {
				// Channel closed, send close message
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// No else needed: error handling with return (exits function)
			// Write each message as a separate WebSocket frame
			// This ensures proper JSON parsing on the client side
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Increment messages sent metric
			metrics.MessagesSent.Inc()

		case <-ticker.C:
			// No else needed: error handling with return (exits function)
			// Send ping message
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
