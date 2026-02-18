// Package constants provides centralized constant definitions for the chatbox application.
// This eliminates magic numbers and strings throughout the codebase.
package constants

import "time"

// HTTP Status Codes
const (
	StatusOK                 = 200
	StatusTooManyRequests    = 429
	StatusServiceUnavailable = 503
)

// Timeouts for various operations
const (
	DefaultContextTimeout   = 10 * time.Second  // Standard database operations
	LongContextTimeout      = 30 * time.Second  // Complex queries and index creation
	DefaultLLMStreamTimeout = 120 * time.Second // LLM streaming requests
	MongoIndexTimeout       = 30 * time.Second  // MongoDB index creation
	ShortTimeout            = 2 * time.Second   // Quick operations like health checks
	MessageAddTimeout       = 5 * time.Second   // Adding messages to sessions
	SessionEndTimeout       = 5 * time.Second   // Ending sessions
	HealthCheckTimeout      = 2 * time.Second   // Health check operations
	MetricsTimeout          = 30 * time.Second  // Metrics aggregation
	VoiceProcessTimeout     = 60 * time.Second  // Voice message processing
)

// Sizes and Limits
const (
	DefaultMaxMessageSize = 1048576 // 1MB in bytes for WebSocket messages
	EncryptionKeyLength   = 32      // AES-256 requires exactly 32 bytes
	DefaultSessionLimit   = 100     // Default number of sessions to return
	MaxSessionLimit       = 1000    // Maximum sessions per query (performance cap)
	DefaultRateLimit      = 100     // Default messages per minute per user
	DefaultAdminRateLimit = 20      // Default admin requests per minute
	MaxRetryAttempts      = 3       // Maximum retry attempts for transient errors
)

// Durations for background operations
const (
	DefaultReconnectTimeout = 15 * time.Minute // Session reconnection timeout
	DefaultRateWindow       = 1 * time.Minute  // Rate limiting window
	DefaultCleanupInterval  = 5 * time.Minute  // Cleanup goroutine interval
	DefaultSessionTTL       = 15 * time.Minute // Session time-to-live after inactivity
	InitialRetryDelay       = 100 * time.Millisecond
	MaxRetryDelay           = 2 * time.Second
	RetryMultiplier         = 2.0
)

// Role Names for authorization
const (
	RoleAdmin     = "admin"
	RoleChatAdmin = "chat_admin"
)

// Sender Types for messages
const (
	SenderUser  = "user"
	SenderAI    = "ai"
	SenderAdmin = "admin"
)

// Default Configuration Values
const (
	DefaultMongoURI   = "mongodb://localhost:27017"
	DefaultDatabase   = "chat"
	DefaultCollection = "sessions"
	DefaultModel      = "gpt-4"
	DefaultPort       = 8080
	DefaultLogLevel   = "info"
	DefaultLogDir     = "logs"
	DefaultPathPrefix = "/chatbox" // Default HTTP path prefix for all routes
)

// HTTP Headers
const (
	HeaderAuthorization = "Authorization"
	HeaderRetryAfter    = "Retry-After"
	BearerPrefix        = "Bearer "
	BearerPrefixLength  = 7
)

// Error Messages
const (
	ErrMsgInvalidAuthHeader = "Invalid or missing Authorization header"
	ErrMsgInvalidToken      = "Invalid or expired token"
	ErrMsgForbidden         = "Insufficient permissions"
	ErrMsgInternalError     = "Internal server error"
	ErrMsgRateLimitExceeded = "Too many requests. Please try again later."
	ErrMsgInvalidTimeFormat = "Invalid time format. Use RFC3339 format."
	ErrMsgSessionIDRequired = "Session ID is required"
)

// MongoDB Field Names (BSON tags)
const (
	MongoFieldID            = "_id"
	MongoFieldUserID        = "uid"
	MongoFieldTimestamp     = "ts"
	MongoFieldEndTime       = "endTs"
	MongoFieldAdminAssisted = "adminAssisted"
	MongoFieldMessages      = "msgs"
	MongoFieldDuration      = "dur"
	MongoFieldTotalTokens   = "totalTokens"
)

// MongoDB Index Names
const (
	IndexUserID        = "idx_user_id"
	IndexStartTime     = "idx_start_time"
	IndexAdminAssisted = "idx_admin_assisted"
	IndexUserStartTime = "idx_user_start_time"
)

// Token Estimation
const (
	CharsPerToken = 4 // Rough estimate: 4 characters per token for LLM usage
)

// Weak Secrets for validation (security check)
var WeakSecrets = []string{
	"secret", "test", "test123", "password", "admin",
	"changeme", "default", "example", "demo", "12345",
}

// Minimum Security Requirements
const (
	MinJWTSecretLength = 32 // Minimum length for JWT secret (256 bits)
	MinPasswordLength  = 8  // Minimum password length
)

// Sort Fields for session queries
const (
	SortByTimestamp    = "ts"
	SortByEndTime      = "endTs"
	SortByMessageCount = "message_count"
	SortByTotalTokens  = "totalTokens"
	SortByUserID       = "uid"
)

// Sort Orders
const (
	SortOrderAsc  = "asc"
	SortOrderDesc = "desc"
)

// Session Status Filters
const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

// Retry After Calculation
const (
	MillisecondsPerSecond = 1000
	MinRetryAfterSeconds  = 1 // Minimum retry-after value in seconds
)
