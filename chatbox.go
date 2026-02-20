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
	"github.com/real-rm/chatbox/internal/constants"
	"github.com/real-rm/chatbox/internal/httperrors"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/notification"
	"github.com/real-rm/chatbox/internal/ratelimit"
	"github.com/real-rm/chatbox/internal/router"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/storage"
	"github.com/real-rm/chatbox/internal/upload"
	"github.com/real-rm/chatbox/internal/util"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/goconfig"
	levelStore "github.com/real-rm/golevelstore"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/real-rm/goupload"
)

func init() {
	// Register "CHAT" as a valid board type for golevelstore's file storage system.
	// golevelstore only ships with predefined board types; we extend it here for the
	// chatbox application. 128 L2 directories provides sufficient distribution for
	// chat file uploads.
	levelStore.SOURCE_L2_SIZE["CHAT"] = 128
}

var (
	// Global references for graceful shutdown
	globalWSHandler     *websocket.Handler
	globalSessionMgr    *session.SessionManager
	globalMessageRouter *router.MessageRouter
	globalAdminLimiter  *ratelimit.MessageLimiter
	globalLogger        *golog.Logger
	shutdownMu          sync.Mutex
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

	// Validate critical configuration at startup
	// This ensures misconfigurations are caught before serving traffic
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// Fall back to config file
		var err error
		jwtSecret, err = config.ConfigString("chatbox.jwt_secret")
		// No else needed: early return pattern (guard clause)
		if err != nil {
			return fmt.Errorf("failed to get JWT secret: %w", err)
		}
	}

	// Validate JWT secret strength
	// No else needed: early return pattern (guard clause)
	if err := validateJWTSecret(jwtSecret); err != nil {
		chatboxLogger.Error("Configuration validation failed", "error", err)
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Load configuration
	// Priority: Environment variable > Config file
	// This allows Kubernetes secrets to override config.toml values
	var err error
	var reconnectTimeoutStr string
	reconnectTimeoutStr, err = config.ConfigStringWithDefault("chatbox.reconnect_timeout", constants.DefaultReconnectTimeout.String())
	if err != nil {
		return fmt.Errorf("failed to get reconnect timeout: %w", err)
	}
	var reconnectTimeout time.Duration
	reconnectTimeout, err = time.ParseDuration(reconnectTimeoutStr)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("invalid reconnect timeout format: %w", err)
	}

	// Load and validate HTTP path prefix early to fail fast on configuration errors.
	// Priority: Environment variable > Config file > Default ("/chatbox")
	pathPrefix := os.Getenv("CHATBOX_PATH_PREFIX")
	if pathPrefix == "" {
		// Fall back to config file
		pathPrefix, err = config.ConfigStringWithDefault("chatbox.path_prefix", constants.DefaultPathPrefix)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			return fmt.Errorf("failed to get path prefix: %w", err)
		}
	}
	// Validate path prefix format
	// No else needed: early return pattern (guard clause)
	if pathPrefix == "" {
		return fmt.Errorf("path prefix cannot be empty")
	}
	// No else needed: early return pattern (guard clause)
	if !strings.HasPrefix(pathPrefix, "/") {
		return fmt.Errorf("path prefix must start with '/' (got: %s)", pathPrefix)
	}

	// Initialize goupload for file uploads
	// No else needed: early return pattern (guard clause)
	if err := goupload.Init(goupload.InitOptions{
		Logger: logger,
		Config: config,
	}); err != nil {
		return fmt.Errorf("failed to initialize goupload: %w", err)
	}

	// Create stats updater for file tracking
	statsColl := mongo.Coll("chat", "file_stats")
	uploadService, err := upload.NewUploadService("CHAT", "uploads", statsColl)
	// No else needed: early return pattern (guard clause)
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
		// No else needed: early return pattern (guard clause)
		if err != nil {
			return fmt.Errorf("failed to get encryption key: %w", err)
		}
	}
	// No else needed: optional operation (logging based on configuration state)
	if encryptionKeyStr != "" {
		// Convert string to bytes
		encryptionKey = []byte(encryptionKeyStr)
		chatboxLogger.Info("Message encryption enabled", "key_length", len(encryptionKey))
	} else {
		chatboxLogger.Warn("No encryption key configured, messages will be stored unencrypted")
	}

	// Validate encryption key length before any encryption operations
	// No else needed: early return pattern (guard clause)
	if err := validateEncryptionKey(encryptionKey); err != nil {
		return err
	}

	// Load maximum message size for WebSocket connections
	// Priority: Environment variable > Config file
	// Default: 1MB (1048576 bytes)
	maxMessageSize := int64(constants.DefaultMaxMessageSize)
	// No else needed: optional operation (configuration loading with fallback)
	if maxSizeStr := os.Getenv("MAX_MESSAGE_SIZE"); maxSizeStr != "" {
		// Parse from environment variable
		var parsedSize int64
		// No else needed: optional operation (logging based on parse result)
		if _, err := fmt.Sscanf(maxSizeStr, "%d", &parsedSize); err == nil {
			maxMessageSize = parsedSize
			chatboxLogger.Info("Using MAX_MESSAGE_SIZE from environment", "size_bytes", maxMessageSize)
		} else {
			chatboxLogger.Warn("Invalid MAX_MESSAGE_SIZE environment variable, using default", "value", maxSizeStr, "default", maxMessageSize)
		}
	} else {
		// Try to load from config file
		// No else needed: optional operation (logging based on parse result)
		if configSizeStr, err := config.ConfigStringWithDefault("chatbox.max_message_size", fmt.Sprintf("%d", constants.DefaultMaxMessageSize)); err == nil {
			var parsedSize int64
			// No else needed: optional operation (logging based on parse result)
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
	indexCtx, indexCancel := util.NewTimeoutContext(constants.MongoIndexTimeout)
	defer indexCancel()
	// No else needed: optional operation (non-critical index creation)
	if err := storageService.EnsureIndexes(indexCtx); err != nil {
		chatboxLogger.Warn("Failed to create MongoDB indexes", "error", err)
		// Don't fail startup - indexes can be created manually if needed
	}

	// Create session manager
	sessionManager := session.NewSessionManager(reconnectTimeout, chatboxLogger)

	// Start cleanup goroutine for expired sessions
	sessionManager.StartCleanup()

	// Create LLM service
	llmService, err := llm.NewLLMService(config, chatboxLogger)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to create LLM service: %w", err)
	}

	// Create notification service
	notificationService, err := notification.NewNotificationService(chatboxLogger, config, mongo)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to create notification service: %w", err)
	}

	// Get LLM stream timeout from config
	llmStreamTimeoutStr, err := config.ConfigStringWithDefault("chatbox.llm_stream_timeout", constants.DefaultLLMStreamTimeout.String())
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to get LLM stream timeout: %w", err)
	}
	llmStreamTimeout, err := time.ParseDuration(llmStreamTimeoutStr)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("invalid LLM stream timeout format: %w", err)
	}

	// Create message router
	messageRouter := router.NewMessageRouter(sessionManager, llmService, uploadService, notificationService, storageService, llmStreamTimeout, chatboxLogger)

	// Create admin rate limiter
	adminRateLimit, err := config.ConfigIntWithDefault("chatbox.admin_rate_limit", constants.DefaultAdminRateLimit)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to get admin rate limit: %w", err)
	}
	adminRateWindowStr, err := config.ConfigStringWithDefault("chatbox.admin_rate_window", constants.DefaultRateWindow.String())
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("failed to get admin rate window: %w", err)
	}
	adminRateWindow, err := time.ParseDuration(adminRateWindowStr)
	// No else needed: early return pattern (guard clause)
	if err != nil {
		return fmt.Errorf("invalid admin rate window format: %w", err)
	}

	adminLimiter := ratelimit.NewMessageLimiter(adminRateWindow, adminRateLimit)
	adminLimiter.StartCleanup()

	chatboxLogger.Info("Admin rate limiter configured",
		"rate_limit", adminRateLimit,
		"window", adminRateWindow)

	// Create JWT validator
	validator := auth.NewJWTValidator(jwtSecret)

	// Create WebSocket handler with router
	wsHandler := websocket.NewHandler(validator, messageRouter, chatboxLogger, maxMessageSize)

	// Configure allowed origins for WebSocket connections
	allowedOriginsStr, err := config.ConfigStringWithDefault("chatbox.allowed_origins", "")
	// No else needed: optional operation (configuration with fallback logging)
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
	globalSessionMgr = sessionManager
	globalMessageRouter = messageRouter
	globalAdminLimiter = adminLimiter
	globalLogger = chatboxLogger
	shutdownMu.Unlock()

	// Configure CORS middleware
	// Load CORS configuration from config file or environment
	corsOriginsStr, err := config.ConfigStringWithDefault("chatbox.cors_allowed_origins", "")
	// No else needed: optional operation (CORS configuration with fallback logging)
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

	chatboxLogger.Info("Using HTTP path prefix", "prefix", pathPrefix)

	// Register routes
	chatGroup := r.Group(pathPrefix)
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
		adminGroup.Use(adminRateLimitMiddleware(adminLimiter, chatboxLogger))
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
		"websocket_endpoint", pathPrefix+"/ws",
		"admin_endpoints", pathPrefix+"/admin/*",
		"health_endpoints", pathPrefix+"/healthz, "+pathPrefix+"/readyz",
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
	if keyLen == constants.EncryptionKeyLength {
		return nil
	}

	// Any other length is invalid
	return fmt.Errorf("encryption key must be exactly %d bytes for AES-256, got %d bytes. Please provide a valid %d-byte key or remove the key to disable encryption", constants.EncryptionKeyLength, keyLen, constants.EncryptionKeyLength)
}

// authMiddleware creates a Gin middleware for JWT authentication
func authMiddleware(validator *auth.JWTValidator, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		token, err := util.ExtractBearerToken(authHeader)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Validate token
		claims, err := validator.ValidateToken(token)
		// No else needed: early return pattern (guard clause)
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
			// No else needed: optional operation (role checking loop)
			if role == constants.RoleAdmin || role == constants.RoleChatAdmin {
				hasAdminRole = true
				break
			}
		}

		// No else needed: early return pattern (guard clause)
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

// adminRateLimitMiddleware creates a Gin middleware for admin endpoint rate limiting
func adminRateLimitMiddleware(limiter *ratelimit.MessageLimiter, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context (set by authMiddleware)
		claimsInterface, exists := c.Get("claims")
		// No else needed: early return pattern (guard clause - let authMiddleware handle missing claims)
		if !exists {
			// If no claims, let authMiddleware handle it
			c.Next()
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		// No else needed: early return pattern (guard clause)
		if !ok {
			util.LogError(logger, "admin_rate_limit", "validate claims type", fmt.Errorf("invalid claims type in context"))
			httperrors.RespondInternalError(c)
			c.Abort()
			return
		}

		// Check rate limit
		// No else needed: early return pattern (guard clause)
		if !limiter.Allow(claims.UserID) {
			retryAfter := limiter.GetRetryAfter(claims.UserID)

			logger.Warn("Admin rate limit exceeded",
				"user_id", claims.UserID,
				"endpoint", c.Request.URL.Path,
				"retry_after_ms", retryAfter,
				"component", "admin_rate_limit")

			// CRITICAL FIX M3: Convert milliseconds to seconds properly with ceiling to avoid 0
			retryAfterSeconds := (retryAfter + constants.MillisecondsPerSecond - 1) / constants.MillisecondsPerSecond // Round up to nearest second
			// No else needed: optional operation (minimum retry after enforcement)
			if retryAfterSeconds < constants.MinRetryAfterSeconds {
				retryAfterSeconds = constants.MinRetryAfterSeconds
			}
			c.Header(constants.HeaderRetryAfter, fmt.Sprintf("%d", retryAfterSeconds))

			// Return 429 Too Many Requests
			c.JSON(constants.StatusTooManyRequests, gin.H{
				"error":          "rate_limit_exceeded",
				"message":        constants.ErrMsgRateLimitExceeded,
				"retry_after_ms": retryAfter,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// userAuthMiddleware creates a Gin middleware for JWT authentication (without admin check)
func userAuthMiddleware(validator *auth.JWTValidator, logger *golog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		token, err := util.ExtractBearerToken(authHeader)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			httperrors.RespondUnauthorized(c, httperrors.MsgInvalidAuthHeader)
			c.Abort()
			return
		}

		// Validate token
		claims, err := validator.ValidateToken(token)
		// No else needed: early return pattern (guard clause)
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
		// No else needed: early return pattern (guard clause)
		if !exists {
			httperrors.RespondUnauthorized(c, "")
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		// No else needed: early return pattern (guard clause)
		if !ok {
			util.LogError(logger, "http", "validate claims type", fmt.Errorf("invalid claims type in context"))
			httperrors.RespondInternalError(c)
			return
		}

		// Get user's sessions
		sessions, err := storageService.ListUserSessions(claims.UserID, 0)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			// Log detailed error server-side
			util.LogError(logger, "http", "list user sessions", err, "user_id", claims.UserID)
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(constants.StatusOK, gin.H{
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
		limit := constants.DefaultSessionLimit
		// No else needed: optional operation (limit parsing with validation)
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && l == 1 {
			// No else needed: optional operation (limit range validation)
			if limit <= 0 || limit > constants.MaxSessionLimit {
				limit = constants.DefaultSessionLimit
			}
		}

		// Parse offset
		offset := 0
		// No else needed: optional operation (offset parsing with validation)
		if o, err := fmt.Sscanf(offsetStr, "%d", &offset); err == nil && o == 1 {
			// No else needed: optional operation (offset range validation)
			if offset < 0 {
				offset = 0
			}
		}

		// Parse admin_assisted filter
		var adminAssisted *bool
		// No else needed: optional operation (filter parsing)
		if adminAssistedStr != "" {
			val := adminAssistedStr == "true"
			adminAssisted = &val
		}

		// Parse active status filter
		var active *bool
		// No else needed: optional operation (filter parsing)
		if status != "" {
			// No else needed: optional operation (status value parsing)
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
		// No else needed: optional operation (time filter parsing)
		if startTimeFromStr != "" {
			t, err := time.Parse(time.RFC3339, startTimeFromStr)
			// No else needed: early return pattern (guard clause)
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
		// No else needed: optional operation (time filter parsing)
		if startTimeToStr != "" {
			t, err := time.Parse(time.RFC3339, startTimeToStr)
			// No else needed: early return pattern (guard clause)
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
		// No else needed: early return pattern (guard clause)
		if err != nil {
			// Log detailed error server-side
			util.LogError(logger, "http", "list sessions", err)
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(constants.StatusOK, gin.H{
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

		// No else needed: optional operation (time range parsing with default)
		if startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			// No else needed: early return pattern (guard clause)
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

		// No else needed: optional operation (time range parsing with default)
		if endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			// No else needed: early return pattern (guard clause)
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
		// No else needed: early return pattern (guard clause)
		if err != nil {
			// Log detailed error server-side
			util.LogError(logger, "http", "get session metrics", err)
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		// Get total token usage
		totalTokens, err := storageService.GetTokenUsage(startTime, endTime)
		// No else needed: early return pattern (guard clause)
		if err != nil {
			// Log detailed error server-side
			util.LogError(logger, "http", "get token usage", err)
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		// Update metrics with token usage
		metrics.TotalTokens = totalTokens

		c.JSON(constants.StatusOK, gin.H{
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

		// No else needed: early return pattern (guard clause)
		if sessionID == "" {
			httperrors.RespondBadRequest(c, constants.ErrMsgSessionIDRequired)
			return
		}

		// Get admin claims from context (set by authMiddleware)
		claimsInterface, exists := c.Get("claims")
		// No else needed: early return pattern (guard clause)
		if !exists {
			httperrors.RespondUnauthorized(c, "")
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		// No else needed: early return pattern (guard clause)
		if !ok {
			util.LogError(logger, "http", "validate claims type", fmt.Errorf("invalid claims type in context"))
			httperrors.RespondInternalError(c)
			return
		}

		// Create a mock admin connection for the takeover
		// In a real implementation, this would be a WebSocket connection
		// For now, we'll just mark the session as admin-assisted
		adminConn := websocket.NewConnection(claims.UserID, claims.Roles)
		adminConn.Name = claims.Name // Set admin name from JWT claims

		// Handle admin takeover
		// No else needed: early return pattern (guard clause)
		if err := messageRouter.HandleAdminTakeover(adminConn, sessionID); err != nil {
			// Log detailed error server-side
			util.LogError(logger, "http", "initiate admin takeover", err,
				"session_id", sessionID,
				"admin_id", claims.UserID)
			// Send generic error to client
			httperrors.RespondInternalError(c)
			return
		}

		c.JSON(constants.StatusOK, gin.H{
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
	c.JSON(constants.StatusOK, gin.H{
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
		// No else needed: optional operation (MongoDB health check)
		if mongo == nil {
			checks["mongodb"] = map[string]interface{}{
				"status": "not ready",
				"reason": "MongoDB not initialized",
			}
			allReady = false
		} else {
			// Verify MongoDB connection by pinging the server
			ctx, cancel := util.NewTimeoutContext(constants.HealthCheckTimeout)
			defer cancel()

			// Use Ping() to check MongoDB connectivity
			// This is the recommended way to verify database health
			testColl := mongo.Coll("chat", "sessions")
			err := testColl.Ping(ctx)
			// No else needed: optional operation (health check result recording)
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
		statusCode := constants.StatusOK
		// No else needed: optional operation (status code adjustment based on health)
		if !allReady {
			status = "not ready"
			statusCode = constants.StatusServiceUnavailable
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
// It respects the context deadline and will force shutdown if the deadline is exceeded.
func Shutdown(ctx context.Context) error {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()

	// No else needed: optional operation (logging during shutdown)
	if globalLogger != nil {
		globalLogger.Info("Starting graceful shutdown of chatbox service")
	}

	// Stop session cleanup goroutine
	// No else needed: optional operation (cleanup stop)
	if globalSessionMgr != nil {
		globalSessionMgr.StopCleanup()
	}

	// Stop message router cleanup goroutines
	// No else needed: optional operation (cleanup stop)
	if globalMessageRouter != nil {
		globalMessageRouter.Shutdown()
	}

	// Stop admin rate limiter cleanup
	// No else needed: optional operation (cleanup stop)
	if globalAdminLimiter != nil {
		globalAdminLimiter.StopCleanup()
	}

	// Close all WebSocket connections with context deadline
	// No else needed: optional operation (WebSocket shutdown with error handling)
	if globalWSHandler != nil {
		// No else needed: early return pattern (guard clause)
		if err := globalWSHandler.ShutdownWithContext(ctx); err != nil {
			// No else needed: optional operation (error logging)
			if globalLogger != nil {
				globalLogger.Warn("WebSocket handler shutdown error", "error", err)
			}
			return err
		}
	}

	// Flush logs
	// No else needed: optional operation (final logging)
	if globalLogger != nil {
		globalLogger.Info("Chatbox service shutdown complete")
		// Note: Logger.Close() should be called by gomain, not here
	}

	return nil
}

// validateJWTSecret validates the JWT secret strength
// Returns error if secret is empty, too short, or contains weak patterns
func validateJWTSecret(secret string) error {
	if secret == "" {
		return fmt.Errorf("JWT secret is required")
	}

	// Check minimum length (32 characters for strong security)
	if len(secret) < constants.MinJWTSecretLength {
		return fmt.Errorf(
			"JWT secret must be at least %d characters (got %d). "+
				"Generate a strong secret with: openssl rand -base64 32",
			constants.MinJWTSecretLength, len(secret))
	}

	// Check for common weak secrets
	lowerSecret := strings.ToLower(secret)
	for _, weak := range constants.WeakSecrets {
		if strings.Contains(lowerSecret, weak) {
			return fmt.Errorf(
				"JWT secret appears to be weak (contains '%s'). "+
					"Use a cryptographically random secret generated with: openssl rand -base64 32",
				weak)
		}
	}

	return nil
}
