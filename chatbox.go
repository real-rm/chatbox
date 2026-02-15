// Package chatbox provides the main service registration for the chat application.
// It integrates with gomain by implementing a Register function that sets up all
// WebSocket and HTTP endpoints for the chat service.
package chatbox

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/real-rm/goupload"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/notification"
	"github.com/real-rm/chatbox/internal/router"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/storage"
	"github.com/real-rm/chatbox/internal/upload"
	"github.com/real-rm/chatbox/internal/websocket"
)

// Register registers the chatbox service with the gomain router.
// This function is called by gomain during service initialization.
//
// Parameters:
//   - r: Gin router for registering HTTP and WebSocket endpoints
//   - config: Configuration accessor for loading service settings
//   - logger: Logger for structured logging
//   - mongo: MongoDB client for data persistence
//
// Returns:
//   - error: Any error that occurred during registration
func Register(r *gin.Engine, config *goconfig.ConfigAccessor, logger *golog.Logger, mongo *gomongo.Mongo) error {
	// Create chatbox-specific logger
	chatboxLogger := logger.WithGroup("chatbox")
	chatboxLogger.Info("Initializing chatbox service")

	// Load configuration
	jwtSecret, err := config.ConfigString("chatbox.jwt_secret")
	if err != nil {
		return fmt.Errorf("failed to get JWT secret: %w", err)
	}

	reconnectTimeoutStr, err := config.ConfigStringWithDefault("chatbox.reconnect_timeout", "15m")
	if err != nil {
		return fmt.Errorf("failed to get reconnect timeout: %w", err)
	}
	reconnectTimeout, err := time.ParseDuration(reconnectTimeoutStr)
	if err != nil {
		return fmt.Errorf("invalid reconnect timeout format: %w", err)
	}

	// Initialize goupload for file uploads
	if err := goupload.Init(goupload.InitOptions{
		Logger: logger,
		Config: config,
	}); err != nil {
		return fmt.Errorf("failed to initialize goupload: %w", err)
	}

	// Create stats updater for file tracking
	statsColl := mongo.Coll("chat", "file_stats")
	uploadService, err := upload.NewUploadService("CHAT", "uploads", statsColl)
	if err != nil {
		return fmt.Errorf("failed to create upload service: %w", err)
	}

	// Create storage service
	storageService := storage.NewStorageService(mongo, "chat", "sessions", chatboxLogger, nil)

	// Create session manager
	sessionManager := session.NewSessionManager(reconnectTimeout, chatboxLogger)

	// Create LLM service
	_, err = llm.NewLLMService(config, chatboxLogger)
	if err != nil {
		return fmt.Errorf("failed to create LLM service: %w", err)
	}

	// Create notification service
	_, err = notification.NewNotificationService(chatboxLogger, config, mongo)
	if err != nil {
		return fmt.Errorf("failed to create notification service: %w", err)
	}

	// Create message router
	messageRouter := router.NewMessageRouter(sessionManager, uploadService, chatboxLogger)

	// Create JWT validator
	validator := auth.NewJWTValidator(jwtSecret)

	// Create WebSocket handler with Gin adapter
	wsHandler := websocket.NewHandler(validator, chatboxLogger)

	// Register routes
	chatGroup := r.Group("/chat")
	{
		// WebSocket endpoint - use Gin context adapter
		chatGroup.GET("/ws", func(c *gin.Context) {
			// Adapt Gin context to http.ResponseWriter and *http.Request
			wsHandler.HandleWebSocket(c.Writer, c.Request)
		})

		// Admin HTTP endpoints
		adminGroup := chatGroup.Group("/admin")
		adminGroup.Use(authMiddleware(validator))
		{
			adminGroup.GET("/sessions", handleListSessions(storageService))
			adminGroup.GET("/metrics", handleGetMetrics(storageService))
			adminGroup.POST("/takeover/:sessionID", handleAdminTakeover(messageRouter))
		}

		// Health check endpoints
		chatGroup.GET("/healthz", handleHealthCheck)
		chatGroup.GET("/readyz", handleReadyCheck(mongo))
	}

	chatboxLogger.Info("Chatbox service registered successfully",
		"websocket_endpoint", "/chat/ws",
		"admin_endpoints", "/chat/admin/*",
		"health_endpoints", "/chat/healthz, /chat/readyz",
	)

	return nil
}

// authMiddleware creates a Gin middleware for JWT authentication
func authMiddleware(validator *auth.JWTValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"error": "Missing authorization header"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		token := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			c.JSON(401, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		// Validate token
		claims, err := validator.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{"error": fmt.Sprintf("Invalid token: %v", err)})
			c.Abort()
			return
		}

		// Check for admin role
		hasAdminRole := false
		for _, role := range claims.Roles {
			if role == "admin" || role == "chat_admin" {
				hasAdminRole = true
				break
			}
		}

		if !hasAdminRole {
			c.JSON(403, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("claims", claims)
		c.Next()
	}
}

// handleListSessions returns a handler for listing sessions
func handleListSessions(storage *storage.StorageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement session listing with filtering and sorting
		c.JSON(200, gin.H{"sessions": []interface{}{}})
	}
}

// handleGetMetrics returns a handler for getting session metrics
func handleGetMetrics(storage *storage.StorageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement metrics calculation
		c.JSON(200, gin.H{"metrics": gin.H{}})
	}
}

// handleAdminTakeover returns a handler for admin session takeover
func handleAdminTakeover(router *router.MessageRouter) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")
		// TODO: Implement admin takeover
		c.JSON(200, gin.H{"message": "Takeover initiated", "session_id": sessionID})
	}
}

// handleHealthCheck returns a handler for health check endpoint
func handleHealthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "healthy"})
}

// handleReadyCheck returns a handler for readiness check endpoint
func handleReadyCheck(mongo *gomongo.Mongo) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check MongoDB connection
		if mongo == nil {
			c.JSON(503, gin.H{"status": "not ready", "reason": "MongoDB not initialized"})
			return
		}

		// TODO: Add more readiness checks (LLM service, etc.)
		c.JSON(200, gin.H{"status": "ready"})
	}
}
