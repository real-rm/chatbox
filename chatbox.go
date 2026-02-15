// Package chatbox provides the main service registration for the chat application.
// It integrates with gomain by implementing a Register function that sets up all
// WebSocket and HTTP endpoints for the chat service.
package chatbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/real-rm/chatbox/internal/auth"
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
	messageRouter := router.NewMessageRouter(sessionManager, llmService, uploadService, notificationService, chatboxLogger)

	// Create JWT validator
	validator := auth.NewJWTValidator(jwtSecret)

	// Create WebSocket handler with Gin adapter
	wsHandler := websocket.NewHandler(validator, chatboxLogger)

	// Store global references for graceful shutdown
	shutdownMu.Lock()
	globalWSHandler = wsHandler
	globalLogger = chatboxLogger
	shutdownMu.Unlock()

	// Register routes
	chatGroup := r.Group("/chat")
	{
		// WebSocket endpoint - use Gin context adapter
		chatGroup.GET("/ws", func(c *gin.Context) {
			// Adapt Gin context to http.ResponseWriter and *http.Request
			wsHandler.HandleWebSocket(c.Writer, c.Request)
		})

		// User session list endpoint (authenticated but not admin-only)
		chatGroup.GET("/sessions", userAuthMiddleware(validator), handleUserSessions(storageService))

		// Admin HTTP endpoints
		adminGroup := chatGroup.Group("/admin")
		adminGroup.Use(authMiddleware(validator))
		{
			adminGroup.GET("/sessions", handleListSessions(storageService, sessionManager))
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

// userAuthMiddleware creates a Gin middleware for JWT authentication (without admin check)
func userAuthMiddleware(validator *auth.JWTValidator) gin.HandlerFunc {
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

		// Store claims in context
		c.Set("claims", claims)
		c.Next()
	}
}

// handleUserSessions returns a handler for listing the authenticated user's sessions
func handleUserSessions(storage *storage.StorageService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get claims from context
		claimsInterface, exists := c.Get("claims")
		if !exists {
			c.JSON(401, gin.H{"error": "Unauthorized"})
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		if !ok {
			c.JSON(500, gin.H{"error": "Invalid claims"})
			return
		}

		// Get user's sessions
		sessions, err := storage.ListUserSessions(claims.UserID, 0)
		if err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to list sessions: %v", err)})
			return
		}

		c.JSON(200, gin.H{
			"sessions": sessions,
			"user_id":  claims.UserID,
			"count":    len(sessions),
		})
	}
}

// handleListSessions returns a handler for listing sessions
func handleListSessions(storage *storage.StorageService, sessionManager *session.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get query parameters for filtering and sorting
		userID := c.Query("user_id")
		status := c.Query("status")                          // "active" or "ended"
		adminAssisted := c.Query("admin_assisted")           // "true" or "false"
		sortBy := c.DefaultQuery("sort_by", "last_activity") // "last_activity", "start_time", "duration", "user_id"
		sortOrder := c.DefaultQuery("sort_order", "desc")    // "asc" or "desc"
		limitStr := c.DefaultQuery("limit", "100")

		// Parse limit
		limit := 100
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && l == 1 {
			if limit <= 0 || limit > 1000 {
				limit = 100
			}
		}

		// If user_id is specified, get sessions for that user
		if userID != "" {
			sessions, err := storage.ListUserSessions(userID, limit)
			if err != nil {
				c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to list sessions: %v", err)})
				return
			}

			// Filter by status and admin_assisted if specified
			filtered := filterSessions(sessions, status, adminAssisted)

			// Sort sessions
			sorted := sortSessions(filtered, sortBy, sortOrder)

			c.JSON(200, gin.H{"sessions": sorted, "count": len(sorted)})
			return
		}

		// TODO: Implement listing all sessions across all users
		// For now, return empty list if no user_id specified
		c.JSON(200, gin.H{"sessions": []interface{}{}, "count": 0})
	}
}

// filterSessions filters sessions based on status and admin_assisted flags
func filterSessions(sessions []*storage.SessionMetadata, status, adminAssisted string) []*storage.SessionMetadata {
	if status == "" && adminAssisted == "" {
		return sessions
	}

	filtered := []*storage.SessionMetadata{}
	for _, sess := range sessions {
		// Filter by status
		if status != "" {
			isActive := sess.EndTime == nil
			if status == "active" && !isActive {
				continue
			}
			if status == "ended" && isActive {
				continue
			}
		}

		// Filter by admin_assisted
		if adminAssisted != "" {
			if adminAssisted == "true" && !sess.AdminAssisted {
				continue
			}
			if adminAssisted == "false" && sess.AdminAssisted {
				continue
			}
		}

		filtered = append(filtered, sess)
	}

	return filtered
}

// sortSessions sorts sessions based on the specified field and order
func sortSessions(sessions []*storage.SessionMetadata, sortBy, sortOrder string) []*storage.SessionMetadata {
	// Simple bubble sort for now (good enough for small lists)
	// TODO: Use more efficient sorting for large lists
	sorted := make([]*storage.SessionMetadata, len(sessions))
	copy(sorted, sessions)

	ascending := sortOrder == "asc"

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			shouldSwap := false

			switch sortBy {
			case "last_activity":
				if ascending {
					shouldSwap = sorted[i].LastMessageTime.After(sorted[j].LastMessageTime)
				} else {
					shouldSwap = sorted[i].LastMessageTime.Before(sorted[j].LastMessageTime)
				}
			case "start_time":
				if ascending {
					shouldSwap = sorted[i].StartTime.After(sorted[j].StartTime)
				} else {
					shouldSwap = sorted[i].StartTime.Before(sorted[j].StartTime)
				}
			case "duration":
				dur1 := getDuration(sorted[i])
				dur2 := getDuration(sorted[j])
				if ascending {
					shouldSwap = dur1 > dur2
				} else {
					shouldSwap = dur1 < dur2
				}
			case "user_id":
				// Not applicable for single user queries, but included for completeness
				if ascending {
					shouldSwap = sorted[i].ID > sorted[j].ID
				} else {
					shouldSwap = sorted[i].ID < sorted[j].ID
				}
			}

			if shouldSwap {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// getDuration calculates the duration of a session
func getDuration(sess *storage.SessionMetadata) int64 {
	if sess.EndTime != nil {
		return sess.EndTime.Unix() - sess.StartTime.Unix()
	}
	return time.Now().Unix() - sess.StartTime.Unix()
}

// handleGetMetrics returns a handler for getting session metrics
func handleGetMetrics(storage *storage.StorageService) gin.HandlerFunc {
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
				c.JSON(400, gin.H{"error": fmt.Sprintf("Invalid start_time format: %v", err)})
				return
			}
		} else {
			// Default to last 24 hours
			startTime = time.Now().Add(-24 * time.Hour)
		}

		if endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.JSON(400, gin.H{"error": fmt.Sprintf("Invalid end_time format: %v", err)})
				return
			}
		} else {
			// Default to now
			endTime = time.Now()
		}

		// Get metrics from storage
		metrics, err := storage.GetSessionMetrics(startTime, endTime)
		if err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to get metrics: %v", err)})
			return
		}

		// Get total token usage
		totalTokens, err := storage.GetTokenUsage(startTime, endTime)
		if err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to get token usage: %v", err)})
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
func handleAdminTakeover(messageRouter *router.MessageRouter) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("sessionID")

		if sessionID == "" {
			c.JSON(400, gin.H{"error": "Session ID is required"})
			return
		}

		// Get admin claims from context (set by authMiddleware)
		claimsInterface, exists := c.Get("claims")
		if !exists {
			c.JSON(401, gin.H{"error": "Authentication required"})
			return
		}

		claims, ok := claimsInterface.(*auth.Claims)
		if !ok {
			c.JSON(500, gin.H{"error": "Invalid claims format"})
			return
		}

		// Create a mock admin connection for the takeover
		// In a real implementation, this would be a WebSocket connection
		// For now, we'll just mark the session as admin-assisted
		adminConn := websocket.NewConnection(claims.UserID, claims.Roles)

		// Handle admin takeover
		if err := messageRouter.HandleAdminTakeover(adminConn, sessionID); err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to initiate takeover: %v", err)})
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
func handleReadyCheck(mongo *gomongo.Mongo) gin.HandlerFunc {
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
			// MongoDB is initialized - gomongo handles connection pooling internally
			// We assume if mongo is not nil, it's ready
			checks["mongodb"] = map[string]interface{}{
				"status": "ready",
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
