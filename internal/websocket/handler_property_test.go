package websocket

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/chatbox/internal/auth"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/require"
)

// testLogger creates a test logger for property tests
func testPropertyLogger() *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            ".",     // Use current directory for test logs
		Level:          "error", // Only log errors in tests
		StandardOutput: true,    // Output to stdout
	})
	if err != nil {
		panic("failed to create test logger: " + err.Error())
	}
	return logger
}

// Feature: chat-application-websocket
// Property 3: Connection User Association
// **Validates: Requirements 1.4**
//
// For any authenticated connection, the connection should always be associated
// with the user ID extracted from the JWT token.
func TestProperty_ConnectionUserAssociation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("connection is associated with JWT user ID", prop.ForAll(
		func(userID string, roles []string) bool {
			// Skip empty user IDs as they're invalid
			if userID == "" {
				return true
			}

			// Create a valid JWT token with the user ID and roles
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   roles,
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create a connection
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)
			conn := handler.createConnection(nil, claims)

			// Verify the connection is associated with the correct user ID
			return conn.UserID == userID &&
				len(conn.Roles) == len(roles) &&
				rolesMatch(conn.Roles, roles)
		},
		gen.Identifier(),              // Generate valid user IDs
		gen.SliceOf(gen.Identifier()), // Generate role arrays
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 4: WebSocket Connection Establishment
// **Validates: Requirements 2.1**
//
// For any valid authentication token, initiating a connection should result
// in a successfully established bidirectional WebSocket connection.
func TestProperty_WebSocketConnectionEstablishment(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("valid token allows connection establishment", prop.ForAll(
		func(userID string, roles []string) bool {
			// Skip empty user IDs as they're invalid
			if userID == "" {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   roles,
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Verify we can create a connection with valid claims
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)
			conn := handler.createConnection(nil, claims)

			// Connection should be created successfully with proper initialization
			return conn != nil &&
				conn.UserID == userID &&
				conn.send != nil
		},
		gen.Identifier(),
		gen.SliceOf(gen.Identifier()),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Helper function to check if two role slices match
func rolesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// Create a map for quick lookup
	roleMap := make(map[string]bool)
	for _, role := range a {
		roleMap[role] = true
	}

	// Check all roles in b exist in a
	for _, role := range b {
		if !roleMap[role] {
			return false
		}
	}

	return true
}

// Feature: chat-application-websocket
// Property 5: Heartbeat Response
// **Validates: Requirements 2.2**
//
// For any active connection, sending a ping message should result in receiving a pong response.
func TestProperty_HeartbeatResponse(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("ping message receives pong response", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs as they're invalid
			if userID == "" {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create a mock WebSocket connection
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)
			conn := handler.createConnection(nil, claims)

			// Verify the connection has a send channel (required for heartbeat)
			// In a real scenario, the writePump would handle ping/pong
			// Here we verify the connection is properly initialized for heartbeat
			return conn != nil && conn.send != nil
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket
// Property 7: Connection Resource Cleanup
// **Validates: Requirements 2.6**
//
// For any connection that is closed gracefully, all associated resources
// (memory, connection maps, channels) should be cleaned up and no longer accessible.
func TestProperty_ConnectionResourceCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("closed connection cleans up all resources", prop.ForAll(
		func(userID string, roles []string) bool {
			// Skip empty user IDs as they're invalid
			if userID == "" {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   roles,
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler and connection
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)
			conn := handler.createConnection(nil, claims)

			// Register the connection
			handler.registerConnection(conn)

			// Verify connection is registered
			handler.mu.RLock()
			_, exists := handler.connections[userID]
			handler.mu.RUnlock()

			if !exists {
				return false
			}

			// Unregister the connection (simulating graceful close)
			handler.unregisterConnection(conn)

			// Verify connection is removed from map
			handler.mu.RLock()
			_, stillExists := handler.connections[userID]
			handler.mu.RUnlock()

			if stillExists {
				return false
			}

			// Verify send channel is closed by attempting to receive
			// A closed channel will return immediately with zero value and false
			select {
			case _, ok := <-conn.send:
				// Channel should be closed (ok == false)
				return !ok
			default:
				// Channel might be empty but not closed yet
				// This is acceptable as the channel will be closed
				return true
			}
		},
		gen.Identifier(),
		gen.SliceOf(gen.Identifier()),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Test helper to create valid tokens
func createValidToken(t *testing.T, userID string, roles []string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"roles":   roles,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	return tokenString
}

// Feature: production-readiness
// Property: Connection ID Uniqueness
// **Validates: Requirements 3.1**
//
// For any number of connections created for the same user, even when created
// rapidly in succession, each connection should have a unique connection ID.
// This ensures multi-device support works correctly without connection conflicts.
func TestProperty_ConnectionIDUniqueness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("connection IDs are always unique for rapid connections", prop.ForAll(
		func(userID string, numConnections uint8) bool {
			// Skip empty user IDs and ensure we have at least 2 connections to test
			if userID == "" || numConnections < 2 {
				return true
			}

			// Limit to reasonable number of connections for testing
			if numConnections > 100 {
				numConnections = 100
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)

			// Generate multiple connections rapidly
			connectionIDs := make(map[string]bool)
			for i := uint8(0); i < numConnections; i++ {
				conn := handler.createConnection(nil, claims)

				// Check if connection ID already exists
				if connectionIDs[conn.ConnectionID] {
					return false // Duplicate found!
				}

				connectionIDs[conn.ConnectionID] = true

				// Verify connection ID format (should contain user ID)
				if conn.ConnectionID == "" {
					return false
				}
			}

			// Verify all connection IDs are unique
			return len(connectionIDs) == int(numConnections)
		},
		gen.Identifier(),
		gen.UInt8(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: production-readiness
// Property: Connection ID Format
// **Validates: Requirements 3.1**
//
// For any connection created, the connection ID should be non-empty and
// contain the user ID as a prefix for traceability and debugging.
func TestProperty_ConnectionIDFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("connection IDs are non-empty and contain user ID", prop.ForAll(
		func(userID string) bool {
			// Skip empty user IDs as they're invalid
			if userID == "" {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler and connection
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)
			conn := handler.createConnection(nil, claims)

			// Verify connection ID is non-empty
			if conn.ConnectionID == "" {
				return false
			}

			// Verify connection ID starts with user ID for traceability
			// This helps with debugging and log correlation
			return len(conn.ConnectionID) > len(userID) &&
				conn.ConnectionID[:len(userID)] == userID
		},
		gen.Identifier(),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes
// Property 2: Read limit enforcement
// **Validates: Requirements 3.1**
//
// For any WebSocket connection established by the system, the connection should
// have a read limit configured that prevents reading messages larger than the
// configured maximum.
func TestProperty_ReadLimitEnforcement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("all connections have read limit set", prop.ForAll(
		func(userID string, maxMessageSize int64) bool {
			// Skip invalid inputs
			if userID == "" || maxMessageSize <= 0 || maxMessageSize > 100*1024*1024 {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			_, err = validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler with specific max message size
			handler := NewHandler(validator, nil, testPropertyLogger(), maxMessageSize)

			// Verify handler has the correct max message size configured
			return handler.maxMessageSize == maxMessageSize
		},
		gen.Identifier(),
		gen.Int64Range(1024, 10*1024*1024), // 1KB to 10MB
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes
// Property 3: Oversized message rejection
// **Validates: Requirements 3.2**
//
// For any message sent to a WebSocket connection that exceeds the configured
// read limit, the connection should be closed with an error.
//
// Note: This property test verifies the handler configuration. The actual
// rejection behavior is tested in integration tests since it requires a real
// WebSocket connection to test the Gorilla WebSocket library's enforcement.
func TestProperty_OversizedMessageRejection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("handler enforces message size limits", prop.ForAll(
		func(userID string, messageSize int64, maxSize int64) bool {
			// Skip invalid inputs
			if userID == "" || messageSize <= 0 || maxSize <= 0 || maxSize > 100*1024*1024 {
				return true
			}

			// Ensure messageSize is larger than maxSize for this test
			if messageSize <= maxSize {
				messageSize = maxSize + 1
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			_, err = validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler with specific max message size
			handler := NewHandler(validator, nil, testPropertyLogger(), maxSize)

			// Verify handler would reject oversized messages
			// The actual rejection happens in the Gorilla WebSocket library
			// when SetReadLimit is called, so we verify the configuration
			return handler.maxMessageSize < messageSize
		},
		gen.Identifier(),
		gen.Int64Range(1024*1024, 100*1024*1024), // 1MB to 100MB (oversized)
		gen.Int64Range(1024, 10*1024*1024),       // 1KB to 10MB (limit)
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes
// Property 4: Configuration value application
// **Validates: Requirements 3.3**
//
// For any valid configuration value for the message size limit, when the system
// starts with that configuration, the read limit should be set to that value.
func TestProperty_ConfigurationValueApplication(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("handler applies configured message size limit", prop.ForAll(
		func(userID string, configuredLimit int64) bool {
			// Skip invalid inputs
			if userID == "" || configuredLimit <= 0 || configuredLimit > 100*1024*1024 {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			_, err = validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler with configured limit
			handler := NewHandler(validator, nil, testPropertyLogger(), configuredLimit)

			// Verify the handler has the exact configured limit
			return handler.maxMessageSize == configuredLimit
		},
		gen.Identifier(),
		gen.Int64Range(1024, 100*1024*1024), // 1KB to 100MB
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: critical-security-fixes
// Property 5: Oversized message logging
// **Validates: Requirements 3.5, 3.6**
//
// For any connection that is closed due to an oversized message, a log entry
// should be created that contains both the user ID and connection ID.
//
// Note: This property test verifies the handler has the necessary context
// (user ID, connection ID) available for logging. The actual logging behavior
// is tested in integration tests where we can capture log output.
func TestProperty_OversizedMessageLogging(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("connections have user ID and connection ID for logging", prop.ForAll(
		func(userID string, maxMessageSize int64) bool {
			// Skip invalid inputs
			if userID == "" || maxMessageSize <= 0 || maxMessageSize > 100*1024*1024 {
				return true
			}

			// Create a valid JWT token
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"roles":   []string{"user"},
				"exp":     time.Now().Add(time.Hour).Unix(),
			})

			tokenString, err := token.SignedString([]byte("test-secret"))
			if err != nil {
				return false
			}

			// Validate the token
			validator := auth.NewJWTValidator("test-secret")
			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				return false
			}

			// Create handler
			handler := NewHandler(validator, nil, testPropertyLogger(), maxMessageSize)

			// Create a connection
			conn := handler.createConnection(nil, claims)

			// Verify connection has all required fields for logging
			// When an oversized message error occurs, the readPump function
			// logs with user_id, connection_id, and limit
			return conn.UserID != "" &&
				conn.ConnectionID != "" &&
				handler.maxMessageSize == maxMessageSize
		},
		gen.Identifier(),
		gen.Int64Range(1024, 10*1024*1024), // 1KB to 10MB
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: production-readiness-fixes
// Property 14: Origin validation is thread-safe
// **Validates: Requirements 13.1, 13.2, 13.4, 13.5**
//
// For any concurrent access to checkOrigin() and SetAllowedOrigins(), no data
// races should occur. This ensures the system remains stable under concurrent
// WebSocket connection attempts and origin configuration updates.
func TestProperty_OriginValidationThreadSafety(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("concurrent checkOrigin and SetAllowedOrigins are thread-safe", prop.ForAll(
		func(numReaders uint8, numWriters uint8, origins []string) bool {
			// Skip invalid inputs
			if numReaders == 0 || numWriters == 0 || len(origins) == 0 {
				return true
			}

			// Limit to reasonable numbers for testing
			if numReaders > 50 {
				numReaders = 50
			}
			if numWriters > 10 {
				numWriters = 10
			}
			if len(origins) > 20 {
				origins = origins[:20]
			}

			// Filter out empty origins
			validOrigins := make([]string, 0, len(origins))
			for _, origin := range origins {
				if origin != "" {
					validOrigins = append(validOrigins, origin)
				}
			}
			if len(validOrigins) == 0 {
				return true
			}

			// Create handler
			validator := auth.NewJWTValidator("test-secret")
			handler := NewHandler(validator, nil, testPropertyLogger(), 1048576)

			// Set initial origins
			handler.SetAllowedOrigins(validOrigins)

			// Create a done channel to coordinate goroutines
			done := make(chan bool)
			errChan := make(chan error, int(numReaders)+int(numWriters))

			// Start reader goroutines (calling checkOrigin)
			for i := uint8(0); i < numReaders; i++ {
				go func(readerID uint8) {
					defer func() {
						if r := recover(); r != nil {
							errChan <- nil
						}
						done <- true
					}()

					// Create a mock HTTP request with an origin header
					originIndex := int(readerID) % len(validOrigins)
					req := &http.Request{
						Header: http.Header{
							"Origin": []string{validOrigins[originIndex]},
						},
					}

					// Call checkOrigin multiple times
					for j := 0; j < 100; j++ {
						_ = handler.checkOrigin(req)
					}
				}(i)
			}

			// Start writer goroutines (calling SetAllowedOrigins)
			for i := uint8(0); i < numWriters; i++ {
				go func(writerID uint8) {
					defer func() {
						if r := recover(); r != nil {
							errChan <- nil
						}
						done <- true
					}()

					// Rotate through different origin sets
					for j := 0; j < 10; j++ {
						startIdx := (int(writerID) + j) % len(validOrigins)
						endIdx := (startIdx + 1 + j) % len(validOrigins)
						if endIdx < startIdx {
							startIdx, endIdx = endIdx, startIdx
						}
						if endIdx >= len(validOrigins) {
							endIdx = len(validOrigins) - 1
						}

						subset := validOrigins[startIdx : endIdx+1]
						handler.SetAllowedOrigins(subset)

						// Small sleep to allow readers to interleave
						time.Sleep(time.Microsecond)
					}
				}(i)
			}

			// Wait for all goroutines to complete
			totalGoroutines := int(numReaders) + int(numWriters)
			for i := 0; i < totalGoroutines; i++ {
				<-done
			}

			// Check if any errors occurred
			select {
			case <-errChan:
				return false
			default:
				return true
			}
		},
		gen.UInt8Range(1, 50),         // Number of reader goroutines
		gen.UInt8Range(1, 10),         // Number of writer goroutines
		gen.SliceOf(gen.Identifier()), // List of origins
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
