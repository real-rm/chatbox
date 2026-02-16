// Package chatbox provides the main service registration for the chat application.
// It integrates with gomain by implementing a Register function that sets up all
// WebSocket and HTTP endpoints for the chat service.
package chatbox

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/chatbox/internal/httperrors"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/notification"
	"github.com/real-rm/chatbox/internal/router"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/storage"
	"github.com/real-rm/chatbox/internal/upload"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/real-rm/goupload"
)

var (
	// Global references for graceful shutdown
	globalWSHandler *websocket.Handler
	globalLogger    *golog.Logger
	shutdownMu      sync.Mutex
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
	// Priority: Environment variable > Config file
	// This allows Kubernetes secrets to override config.toml values
	var err error
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// Fall back to config file
		jwtSecret, err = config.ConfigString("chatbox.jwt_secret")
		if err != nil {
			return fmt.Errorf("failed to get JWT secret: %w", err)
		}
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

	// Load encryption key for message content at rest
	// Priority: Environment variable > Config file
	// The key must be exactly 32 bytes for AES-256 encryption
	var encryptionKey []byte
	encryptionKeyStr := os.Getenv("ENCRYPTION_KEY")
	if encryptionKeyStr == "" {
		// Fall back to config file
		encryptionKeyStr, err = config.ConfigStringWithDefault("chatbox.encryption_key", "")
		if err != nil {
			return fmt.Errorf("failed to get encryption key: %w", err)
		}
	}
	if encryptionKeyStr != "" {
		// Convert string to bytes
		encryptionKey = []byte(encryptionKeyStr)
		chatboxLogger.Info("Message encryption enabled", "key_length", len(encryptionKey))
	} else {
		chatboxLogger.Warn("No encryption key configured, messages will be stored unencrypted")
	}

	// Validate encryption key length before any encryption operations
	if err := validateEncryptionKey(encryptionKey); err != nil {
		return err
	}

	// Load maximum message size for WebSocket connections
	// Priority: Environment variable > Config file
	// Default: 1MB (1048576 bytes)
	maxMessageSize := int64(1048576) // Default 1MB
	if maxSizeStr := os.Getenv("MAX_MESSAGE_SIZE"); maxSizeStr != "" {
		// Parse from environment variable
		var parsedSize int64
		if _, err := fmt.Sscanf(maxSizeStr, "%d", &parsedSize); err == nil {
			maxMessageSize = parsedSize
			chatboxLogger.Info("Using MAX_MESSAGE_SIZE from environment", "size_bytes", maxMessageSize)
		} else {
			chatboxLogger.Warn("Invalid MAX_MESSAGE_SIZE environment variable, using default", "value", maxSizeStr, "default", maxMessageSize)
		}
	} else {
		// Try to load from config file
		if configSizeStr, err := config.ConfigStringWithDefault("chatbox.max_message_size", "1048576"); err == nil {
			var parsedSize int64
			if _, parseErr := fmt.Sscanf(configSizeStr, "%d", &parsedSize); parseErr == nil {
				maxMessageSize = parsedSize
				chatboxLogger.Info("Using max_message_size from config", "size_bytes", maxMessageSize)
			} else {
				chatboxLogger.Warn("Invalid max_message_size in config, using default", "value", configSizeStr, "default", maxMessageSize)
			}
		} else {
			chatboxLogger.Info("Using default max_message_size", "size_bytes", maxMessageSize)
		}
	}

	// Create storage service with encryption key
	storageService := storage.NewStorageService(mongo, "chat", "sessions", chatboxLogger, encryptionKey)

	// Ensure MongoDB indexes are created for optimal query performance
	indexCtx, indexCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer indexCancel()
	if err := storageService.EnsureIndexes(indexCtx); err != nil {
		chatboxLogger.Warn("Failed to create MongoDB indexes", "error", err)
		// Don't fail startup - indexes can be created manually if needed
	}

	// Create session manager
	sessionManager := session.NewSessionManager(reconnectTimeout, chatboxLogger)

	// Create LLM service
	llmService, err := llm.NewLLMService(config, chatboxLogger)
	if err != nil {
		return fmt.Errorf("failed to create LLM service: %w", err)
	}

	// Create notification service
	notificationService, err := notification.NewNotificationService(chatboxLogger, config, mongo)
	if err != nil {
		return fmt.Errorf("failed to create notification service: %w", err)
	}

	// Create message router
	messageRouter := router.NewMessageRouter(sessionManager, llmService, uploadService, notificationService, storageService, chatboxLogger)

	// Create JWT validator
	validator := auth.NewJWTValidator(jwtSecret)

	// Create WebSocket handler with router
	wsHandler := websocket.NewHandler(validator, messageRouter, chatboxLogger, maxMessageSize)
	
	// Configure allowed origins for WebSocket connections
	allowedOriginsStr, err := config.ConfigStringWithDefault("chatbox.allowed_origins", "")
	if err == nil && allowedOriginsStr != "" {
		origins := strings.Split(allowedOriginsStr, ",")
		for i, origin := range origins {
			origins[i] = strings.TrimSpace(origin)
		}
		wsHandler.SetAllowedOrigins(origins)
	} else {
		chatboxLogger.Warn("No allowed origins configured, allowing all origins (development mode)")
	}

	// Store global references for graceful shutdown
	shutdownMu.Lock()
	globalWSHandler = wsHandler
	globalLogger = chatboxLogger
	shutdownMu.Unlock()

	// Configure CORS middleware
	// Load CORS configuration from config file or environment
	corsOriginsStr, err := config.ConfigStringWithDefault("chatbox.cors_allowed_origins", "")
	if err == nil && corsOriginsStr != "" {
		// Parse allowed origins from comma-separated string
		allowedOrigins := strings.Split(corsOriginsStr, ",")
		for i, origin := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(origin)
		}

		// Configure CORS middleware
		corsConfig := cors.Config{
			AllowOrigins:     allowedOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}

		// Apply CORS middleware to the router
		r.Use(cors.New(corsConfig))

		chatboxLogger.Info("CORS middleware configured",
			"allowed_origins", allowedOrigins,
			"allow_credentials", true)
	} else {
		chatboxLogger.Warn("No CORS origins configured, CORS middleware not enabled")
	}

	// Register routes
	chatGroup := r.Group("/chat")
	{
		// WebSocket endpoint - use Gin context adapter
		chatGroup.GET("/ws", func(c *gin.Context) {
			// Adapt Gin context to http.ResponseWriter and *http.Request
			wsHandler.HandleWebSocket(c.Writer, c.Request)
		})

		// User session list endpoint (authenticated but not admin-only)
		chatGroup.GET("/sessions", userAuthMiddleware(validator, chatboxLogger), handleUserSessions(storageService, chatboxLogger))

		// Admin HTTP endpoints
		adminGroup := chatGroup.Group("/admin")
		adminGroup.Use(authMiddleware(validator, chatboxLogger))
		{
			adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager, chatboxLogger))
			adminGroup.GET("/metrics", handleGetMetrics(storageService, chatboxLogger))
			adminGroup.POST("/takeover/:sessionID", handleAdminTakeover(messageRouter, chatboxLogger))
		}

		// Health check endpoints
		chatGroup.GET("/healthz", handleHealthCheck)
		chatGroup.GET("/readyz", handleReadyCheck(mongo, chatboxLogger))
	}

	// Prometheus metrics endpoint (public, no authentication required)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	chatboxLogger.Info("Chatbox service registered successfully",
		"websocket_endpoint", "/chat/ws",
		"admin_endpoints", "/chat/admin/*",
		"health_endpoints", "/chat/healthz, /chat/readyz",
		"metrics_endpoint", "/metrics",
	)

	return nil
}
// validateEncryptionKey checks if the encryption key is exactly 32 bytes
// Returns error if key is provided but not 32 bytes
// Returns nil if key is empty (encryption disabled) or exactly 32 bytes
func validateEncryptionKey(key []byte) error {
	keyLen := len(key)

	// Empty key is valid (encryption disabled)
	if keyLen == 0 {
		return nil
	}

	// 32 bytes is valid for AES-256
	if keyLen == 32 {
		return nil
	}

	// Any other length is invalid
	return fmt.Errorf("encryption key must be exactly 32 bytes for AES-256, got %d bytes. Please provide a valid 32-byte key or remove the key to disable encryption", keyLen)
}


// authMiddleware creates a Gin middleware for JWT authentication
func authMiddleware(validator *auth.JWTValidator, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		token := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Validate token
		claims, err := validator.ValidateToken(token)
		if err != nil {
			// Log detailed error server-side
			logger.Warn("Token validation failed",
				"error", err,
				"component", "auth")
			// Send generic error to client
			httperrors.RespondInvalidToken(c)
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
			logger.Warn("Insufficient permissions for admin endpoint",
				"user_id", claims.UserID,
				"roles", claims.Roles,
				"component", "auth")
			httperrors.RespondForbidden(c)
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("claims", claims)
		c.Next()
	}
}

// userAuthMiddleware creates a Gin middleware for JWT authentication (without admin check)
func userAuthMiddleware(validator *auth.JWTValidator, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Remove "Bearer " prefix
		token := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Validate token
		claims, err := validator.ValidateToken(token)
		if err != nil {
			// Log detailed error server-side
			logger.Warn("Token validation failed",
				"error", err,
				"component", "auth")
			// Send generic error to client
			httperrors.RespondInvalidToken(c)
			c.Abort()
			return
		}

		// Store claims in context
		c.Set("claims", claims)
		c.Next()
	}
}

// handleUserSessions returns a handler for listing the authenticated user's sessions
func handleUserSessions(storageService *storage.StorageService, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context
		claimsInterface, exists := c.Get("claims")
		if !exists {
			httperrors.RespondUnauthorized(c, "")
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		if !ok {
			logger.Error("Invalid claims type in context", "component", "http")
			httperrors.RespondInternalError(c)
			return
		}

		// Get user's sessions
		sessions, err := storageService.ListUserSessions(claims.UserID, 0)
		if err != nil {
			// Log detailed error server-side
			logger.Error("Failed to list user sessions",
				"user_id", claims.UserID,
				"error", err,
				"component", "http")
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(200, gin.H{
			"sessions": sessions,
			"user_id":  claims.UserID,
			"count":    len(sessions),
		})
	}
}

// handleListSessions returns a handler for listing sessions with pagination, filtering, and sorting
func handleListSessions(storageService *storage.StorageService, sessionManager *session.SessionManager, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse query parameters
		userID := c.Query("user_id")
		status := c.Query("status")                       // "active" or "ended"
		adminAssistedStr := c.Query("admin_assisted")     // "true" or "false"
		sortBy := c.DefaultQuery("sort_by", "start_time") // "start_time", "end_time", "message_count", "total_tokens", "user_id"
		sortOrder := c.DefaultQuery("sort_order", "desc") // "asc" or "desc"
		limitStr := c.DefaultQuery("limit", "100")
		offsetStr := c.DefaultQuery("offset", "0")
		startTimeFromStr := c.Query("start_time_from") // RFC3339 format
		startTimeToStr := c.Query("start_time_to")     // RFC3339 format

		// Parse limit
		limit := 100
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && l == 1 {
			if limit <= 0 || limit > 1000 {
				limit = 100
			}
		}

		// Parse offset
		offset := 0
		if o, err := fmt.Sscanf(offsetStr, "%d", &offset); err == nil && o == 1 {
			if offset < 0 {
				offset = 0
			}
		}

		// Parse admin_assisted filter
		var adminAssisted *bool
		if adminAssistedStr != "" {
			val := adminAssistedStr == "true"
			adminAssisted = &val
		}

		// Parse active status filter
		var active *bool
		if status != "" {
			if status == "active" {
				val := true
				active = &val
			} else if status == "ended" {
				val := false
				active = &val
			}
		}

		// Parse time range filters
		var startTimeFrom, startTimeTo *time.Time
		if startTimeFromStr != "" {
			t, err := time.Parse(time.RFC3339, startTimeFromStr)
			if err != nil {
				logger.Warn("Invalid start_time_from parameter",
					"value", startTimeFromStr,
					"error", err,
					"component", "http")
				httperrors.RespondBadRequest(c, httperrors.MsgInvalidTimeFormat)
				return
			}
			startTimeFrom = &t
		}
		if startTimeToStr != "" {
			t, err := time.Parse(time.RFC3339, startTimeToStr)
			if err != nil {
				logger.Warn("Invalid start_time_to parameter",
					"value", startTimeToStr,
					"error", err,
					"component", "http")
				httperrors.RespondBadRequest(c, httperrors.MsgInvalidTimeFormat)
				return
			}
			startTimeTo = &t
		}

		// Build options for ListAllSessionsWithOptions
		opts := &storage.SessionListOptions{
			Limit:         limit,
			Offset:        offset,
			UserID:        userID,
			StartTimeFrom: startTimeFrom,
			StartTimeTo:   startTimeTo,
			AdminAssisted: adminAssisted,
			Active:        active,
			SortBy:        sortBy,
			SortOrder:     sortOrder,
		}

		// List sessions with options
		sessions, err := storageService.ListAllSessionsWithOptions(opts)
		if err != nil {
			// Log detailed error server-side
			logger.Error("Failed to list sessions",
				"error", err,
				"component", "http")
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(200, gin.H{
			"sessions": sessions,
			"count":    len(sessions),
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// handleGetMetrics returns a handler for getting session metrics
func handleGetMetrics(storageService *storage.StorageService, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get query parameters for time range
		startTimeStr := c.Query("start_time")
		endTimeStr := c.Query("end_time")

		// Parse time range
		var startTime, endTime time.Time
		var err error

		if startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				logger.Warn("Invalid start_time parameter",
					"value", startTimeStr,
					"error", err,
					"component", "http")
				httperrors.RespondBadRequest(c, httperrors.MsgInvalidTimeFormat)
				return
			}
		} else {
			// Default to last 24 hours
			startTime = time.Now().Add(-24 * time.Hour)
		}

		if endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				logger.Warn("Invalid end_time parameter",
					"value", endTimeStr,
					"error", err,
					"component", "http")
				httperrors.RespondBadRequest(c, httperrors.MsgInvalidTimeFormat)
				return
			}
		} else {
			// Default to now
			endTime = time.Now()
		}

		// Get metrics from storage
		metrics, err := storageService.GetSessionMetrics(startTime, endTime)
		if err != nil {
			// Log detailed error server-side
			logger.Error("Failed to get session metrics",
				"error", err,
				"component", "http")
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		// Get total token usage
		totalTokens, err := storageService.GetTokenUsage(startTime, endTime)
		if err != nil {
			// Log detailed error server-side
			logger.Error("Failed to get token usage",
				"error", err,
				"component", "http")
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		// Update metrics with token usage
		metrics.TotalTokens = totalTokens

		c.JSON(200, gin.H{
			"metrics": metrics,
			"time_range": gin.H{
				"start": startTime.Format(time.RFC3339),
				"end":   endTime.Format(time.RFC3339),
			},
		})
	}
}

// handleAdminTakeover returns a handler for admin session takeover
func handleAdminTakeover(messageRouter *router.MessageRouter, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")

		if sessionID == "" {
			httperrors.RespondBadRequest(c, "Session ID is required")
			return
		}

		// Get admin claims from context (set by authMiddleware)
		claimsInterface, exists := c.Get("claims")
		if !exists {
			httperrors.RespondUnauthorized(c, "")
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		if !ok {
			logger.Error("Invalid claims type in context", "component", "http")
			httperrors.RespondInternalError(c)
			return
		}

		// Create a mock admin connection for the takeover
		// In a real implementation, this would be a WebSocket connection
		// For now, we'll just mark the session as admin-assisted
		adminConn := websocket.NewConnection(claims.UserID, claims.Roles)
		adminConn.Name = claims.Name // Set admin name from JWT claims

		// Handle admin takeover
		if err := messageRouter.HandleAdminTakeover(adminConn, sessionID); err != nil {
			// Log detailed error server-side
			logger.Error("Failed to initiate admin takeover",
				"session_id", sessionID,
				"admin_id", claims.UserID,
				"error", err,
				"component", "http")
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(200, gin.H{
			"message":    "Takeover initiated successfully",
			"session_id": sessionID,
			"admin_id":   claims.UserID,
		})
	}
}

// handleHealthCheck returns a handler for liveness probe endpoint.
// This endpoint checks if the application is alive and should be restarted if it fails.
// It performs minimal checks to determine if the process is running correctly.
func handleHealthCheck(c *gin.Context) {
	// Basic liveness check - if we can respond, we're alive
	c.JSON(200, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleReadyCheck returns a handler for readiness probe endpoint.
// This endpoint checks if the application is ready to serve traffic.
// It performs comprehensive checks on all critical dependencies.
func handleReadyCheck(mongo *gomongo.Mongo, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := make(map[string]interface{})
		allReady := true

		// Check MongoDB connection
		if mongo == nil {
			checks["mongodb"] = map[string]interface{}{
				"status": "not ready",
				"reason": "MongoDB not initialized",
			}
			allReady = false
		} else {
			// Verify MongoDB connection by pinging the server
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			// Use Ping() to check MongoDB connectivity
			// This is the recommended way to verify database health
			testColl := mongo.Coll("admin", "system.version")
			err := testColl.Ping(ctx)
			if err != nil {
				// Log detailed error server-side
				logger.Warn("MongoDB health check failed",
					"error", err,
					"component", "health")
				
				// Send generic error to client
				checks["mongodb"] = map[string]interface{}{
					"status": "not ready",
					"reason": "Database connectivity check failed",
				}
				allReady = false
			} else {
				checks["mongodb"] = map[string]interface{}{
					"status": "ready",
				}
			}
		}

		// Determine overall status
		status := "ready"
		statusCode := 200
		if !allReady {
			status = "not ready"
			statusCode = 503
		}

		c.JSON(statusCode, gin.H{
			"status":    status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"checks":    checks,
		})
	}
}

// Shutdown gracefully shuts down the chatbox service.
// It closes all active WebSocket connections and flushes logs.
// This function should be called when the application receives a SIGTERM or SIGINT signal.
func Shutdown(ctx context.Context) error {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()

	if globalLogger != nil {
		globalLogger.Info("Starting graceful shutdown of chatbox service")
	}

	// Close all WebSocket connections
	if globalWSHandler != nil {
		globalWSHandler.Shutdown()
	}

	// Flush logs
	if globalLogger != nil {
		globalLogger.Info("Chatbox service shutdown complete")
		// Note: Logger.Close() should be called by gomain, not here
	}

	return nil
}
