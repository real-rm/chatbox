package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestStorageOperations_Coverage tests all public methods in StorageService
// to ensure comprehensive coverage of storage operations
func TestStorageOperations_Coverage(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_sessions", logger, nil)

	// Test EnsureIndexes
	t.Run("EnsureIndexes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := service.EnsureIndexes(ctx)
		assert.NoError(t, err)
	})

	// Test CreateSession with various scenarios
	t.Run("CreateSession_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-create-1",
			UserID:    "user-coverage-1",
			Name:      "Coverage Test Session",
			ModelID:   "gpt-4",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
			IsActive:  true,
		}

		err := service.CreateSession(sess)
		assert.NoError(t, err)
	})

	// Test UpdateSession
	t.Run("UpdateSession_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-update-1",
			UserID:    "user-coverage-2",
			Name:      "Original Name",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		sess.Name = "Updated Name"
		err = service.UpdateSession(sess)
		assert.NoError(t, err)
	})

	// Test GetSession
	t.Run("GetSession_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-get-1",
			UserID:    "user-coverage-3",
			Name:      "Get Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		retrieved, err := service.GetSession("coverage-get-1")
		assert.NoError(t, err)
		assert.Equal(t, "coverage-get-1", retrieved.ID)
	})

	// Test AddMessage
	t.Run("AddMessage_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-addmsg-1",
			UserID:    "user-coverage-4",
			Name:      "Add Message Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		msg := &session.Message{
			Content:   "Test message",
			Timestamp: time.Now(),
			Sender:    "user",
		}

		err = service.AddMessage("coverage-addmsg-1", msg)
		assert.NoError(t, err)
	})

	// Test EndSession
	t.Run("EndSession_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-end-1",
			UserID:    "user-coverage-5",
			Name:      "End Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
			IsActive:  true,
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		err = service.EndSession("coverage-end-1", time.Now())
		assert.NoError(t, err)
	})

	// Test ListUserSessions
	t.Run("ListUserSessions_Success", func(t *testing.T) {
		sess := &session.Session{
			ID:        "coverage-list-1",
			UserID:    "user-coverage-6",
			Name:      "List Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		sessions, err := service.ListUserSessions("user-coverage-6", 10)
		assert.NoError(t, err)
		assert.Len(t, sessions, 1)
	})

	// Test ListAllSessions
	t.Run("ListAllSessions_Success", func(t *testing.T) {
		sessions, err := service.ListAllSessions(10)
		assert.NoError(t, err)
		assert.NotNil(t, sessions)
	})

	// Test ListAllSessionsWithOptions
	t.Run("ListAllSessionsWithOptions_Success", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     10,
			Offset:    0,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		sessions, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, sessions)
	})

	// Test GetSessionMetrics
	t.Run("GetSessionMetrics_Success", func(t *testing.T) {
		startTime := time.Now().Add(-24 * time.Hour)
		endTime := time.Now()

		metrics, err := service.GetSessionMetrics(startTime, endTime)
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
	})

	// Test GetTokenUsage
	t.Run("GetTokenUsage_Success", func(t *testing.T) {
		startTime := time.Now().Add(-24 * time.Hour)
		endTime := time.Now()

		tokens, err := service.GetTokenUsage(startTime, endTime)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, tokens, 0)
	})
}

// TestStorageOperations_ErrorHandling tests error handling for MongoDB operations
func TestStorageOperations_ErrorHandling(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_sessions", logger, nil)

	// Test context timeout scenarios
	t.Run("ContextTimeout_GetSession", func(t *testing.T) {
		// Create a session first
		sess := &session.Session{
			ID:        "timeout-test-1",
			UserID:    "user-timeout-1",
			Name:      "Timeout Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		// Normal retrieval should work
		retrieved, err := service.GetSession("timeout-test-1")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
	})

	// Test empty result sets
	t.Run("EmptyResults_ListUserSessions", func(t *testing.T) {
		sessions, err := service.ListUserSessions("non-existent-user", 10)
		assert.NoError(t, err)
		assert.Empty(t, sessions)
	})

	// Test invalid session IDs
	t.Run("InvalidSessionID_GetSession", func(t *testing.T) {
		_, err := service.GetSession("non-existent-session-id")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	// Test GetSessionMetrics with invalid time range
	t.Run("InvalidTimeRange_GetSessionMetrics", func(t *testing.T) {
		startTime := time.Now()
		endTime := time.Now().Add(-24 * time.Hour) // End before start

		_, err := service.GetSessionMetrics(startTime, endTime)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "end time must be after start time")
	})

	// Test GetTokenUsage with invalid time range
	t.Run("InvalidTimeRange_GetTokenUsage", func(t *testing.T) {
		startTime := time.Now()
		endTime := time.Now().Add(-24 * time.Hour) // End before start

		_, err := service.GetTokenUsage(startTime, endTime)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "end time must be after start time")
	})
}

// TestStorageOperations_EmptyResults tests handling of empty result sets
func TestStorageOperations_EmptyResults(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_empty", logger, nil)

	t.Run("ListUserSessions_NoSessions", func(t *testing.T) {
		sessions, err := service.ListUserSessions("user-no-sessions", 10)
		assert.NoError(t, err)
		assert.Empty(t, sessions)
	})

	t.Run("ListAllSessions_EmptyDatabase", func(t *testing.T) {
		sessions, err := service.ListAllSessions(10)
		assert.NoError(t, err)
		assert.NotNil(t, sessions)
	})

	t.Run("GetSessionMetrics_NoSessions", func(t *testing.T) {
		startTime := time.Now().Add(-24 * time.Hour)
		endTime := time.Now()

		metrics, err := service.GetSessionMetrics(startTime, endTime)
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.Equal(t, 0, metrics.TotalSessions)
	})
}

// TestStorageOperations_InvalidSessionIDs tests handling of invalid session IDs
func TestStorageOperations_InvalidSessionIDs(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_invalid", logger, nil)

	t.Run("GetSession_EmptyID", func(t *testing.T) {
		_, err := service.GetSession("")
		assert.ErrorIs(t, err, ErrInvalidSessionID)
	})

	t.Run("AddMessage_EmptyID", func(t *testing.T) {
		msg := &session.Message{
			Content:   "Test",
			Timestamp: time.Now(),
			Sender:    "user",
		}

		err := service.AddMessage("", msg)
		assert.ErrorIs(t, err, ErrInvalidSessionID)
	})

	t.Run("EndSession_EmptyID", func(t *testing.T) {
		err := service.EndSession("", time.Now())
		assert.ErrorIs(t, err, ErrInvalidSessionID)
	})

	t.Run("UpdateSession_EmptyID", func(t *testing.T) {
		sess := &session.Session{
			ID:     "",
			UserID: "user-test",
		}

		err := service.UpdateSession(sess)
		assert.ErrorIs(t, err, ErrInvalidSessionID)
	})
}

// TestIsRetryableError tests the retry logic error classification
func TestIsRetryableError_Coverage(t *testing.T) {
	tests := []struct {
		name      string
		errMsg    string
		retryable bool
	}{
		{"NilError", "", false},
		{"ConnectionRefused", "connection refused", true},
		{"ConnectionReset", "connection reset", true},
		{"Timeout", "timeout", true},
		{"TemporaryFailure", "temporary failure", true},
		{"IOTimeout", "i/o timeout", true},
		{"EOF", "EOF", true},
		{"ServerSelection", "server selection timeout", true},
		{"NoReachableServers", "no reachable servers", true},
		{"ConnectionPool", "connection pool", true},
		{"Socket", "socket", true},
		{"PermanentError", "duplicate key error", false},
		{"ValidationError", "validation failed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = assert.AnError
				// Create a custom error with the message
				err = &customError{msg: tt.errMsg}
			}

			result := isRetryableError(err)
			assert.Equal(t, tt.retryable, result, "Error: %v", tt.errMsg)
		})
	}
}

// customError is a helper type for testing error messages
type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

// TestContainsAny tests the containsAny helper function
func TestContainsAny_Coverage(t *testing.T) {
	tests := []struct {
		name       string
		str        string
		substrings []string
		expected   bool
	}{
		{"EmptyString", "", []string{"test"}, false},
		{"EmptySubstrings", "test", []string{}, false},
		{"SingleMatch", "connection refused", []string{"refused"}, true},
		{"MultipleSubstrings_FirstMatch", "timeout error", []string{"timeout", "refused"}, true},
		{"MultipleSubstrings_SecondMatch", "connection refused", []string{"timeout", "refused"}, true},
		{"NoMatch", "some error", []string{"timeout", "refused"}, false},
		{"PartialMatch", "test", []string{"testing"}, false},
		{"ExactMatch", "timeout", []string{"timeout"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.str, tt.substrings)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSessionListOptions_Filtering tests filtering options for session listing
func TestSessionListOptions_Filtering(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_filter", logger, nil)

	// Create test sessions with different attributes
	now := time.Now()
	sessions := []*session.Session{
		{
			ID:            "filter-1",
			UserID:        "user-filter-1",
			Name:          "Active Session",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-2 * time.Hour),
			IsActive:      true,
			AdminAssisted: false,
		},
		{
			ID:            "filter-2",
			UserID:        "user-filter-2",
			Name:          "Ended Session",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-3 * time.Hour),
			IsActive:      false,
			AdminAssisted: true,
		},
		{
			ID:            "filter-3",
			UserID:        "user-filter-1",
			Name:          "Another Active",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-1 * time.Hour),
			IsActive:      true,
			AdminAssisted: false,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)

		// End the second session
		if sess.ID == "filter-2" {
			err = service.EndSession(sess.ID, now.Add(-2*time.Hour))
			require.NoError(t, err)
		}
	}

	// Test filtering by user ID
	t.Run("FilterByUserID", func(t *testing.T) {
		opts := &SessionListOptions{
			UserID:    "user-filter-1",
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Len(t, results, 2)
	})

	// Test filtering by admin assisted
	t.Run("FilterByAdminAssisted", func(t *testing.T) {
		adminAssisted := true
		opts := &SessionListOptions{
			AdminAssisted: &adminAssisted,
			Limit:         10,
			SortBy:        "ts",
			SortOrder:     "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)
	})

	// Test filtering by active status
	t.Run("FilterByActive", func(t *testing.T) {
		active := true
		opts := &SessionListOptions{
			Active:    &active,
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	// Test filtering by time range
	t.Run("FilterByTimeRange", func(t *testing.T) {
		startTimeFrom := now.Add(-2*time.Hour - 30*time.Minute)
		startTimeTo := now.Add(-30 * time.Minute)

		opts := &SessionListOptions{
			StartTimeFrom: &startTimeFrom,
			StartTimeTo:   &startTimeTo,
			Limit:         10,
			SortBy:        "ts",
			SortOrder:     "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
	})
}

// TestSessionListOptions_Sorting tests sorting options for session listing
func TestSessionListOptions_Sorting(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_sort", logger, nil)

	// Create test sessions with different attributes
	now := time.Now()
	sessions := []*session.Session{
		{
			ID:          "sort-1",
			UserID:      "user-sort-1",
			Name:        "Session A",
			Messages:    []*session.Message{},
			StartTime:   now.Add(-3 * time.Hour),
			TotalTokens: 100,
		},
		{
			ID:          "sort-2",
			UserID:      "user-sort-2",
			Name:        "Session B",
			Messages:    []*session.Message{},
			StartTime:   now.Add(-2 * time.Hour),
			TotalTokens: 200,
		},
		{
			ID:          "sort-3",
			UserID:      "user-sort-3",
			Name:        "Session C",
			Messages:    []*session.Message{},
			StartTime:   now.Add(-1 * time.Hour),
			TotalTokens: 150,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Test sorting by timestamp descending
	t.Run("SortByTimestamp_Desc", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	// Test sorting by timestamp ascending
	t.Run("SortByTimestamp_Asc", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "asc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	// Test sorting by total tokens
	t.Run("SortByTotalTokens", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     10,
			SortBy:    "totalTokens",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
	})

	// Test sorting by user ID
	t.Run("SortByUserID", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     10,
			SortBy:    "uid",
			SortOrder: "asc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
	})
}

// TestSessionListOptions_Pagination tests pagination options
func TestSessionListOptions_Pagination(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "coverage_page", logger, nil)

	// Create multiple test sessions
	now := time.Now()
	for i := 0; i < 10; i++ {
		sess := &session.Session{
			ID:        primitive.NewObjectID().Hex(),
			UserID:    "user-page-1",
			Name:      "Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(-time.Duration(i) * time.Hour),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Test limit
	t.Run("Limit", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     5,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.LessOrEqual(t, len(results), 5)
	})

	// Test offset
	t.Run("Offset", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     5,
			Offset:    5,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
	})

	// Test default limit
	t.Run("DefaultLimit", func(t *testing.T) {
		opts := &SessionListOptions{
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
	})
}


// TestEncryption_Coverage tests encryption functionality with various scenarios
func TestEncryption_Coverage(t *testing.T) {
	// Test encryption with various key sizes
	t.Run("EncryptionWithVariousKeySizes", func(t *testing.T) {
		testCases := []struct {
			name      string
			keySize   int
			shouldErr bool
		}{
			{"16ByteKey", 16, false}, // AES-128
			{"24ByteKey", 24, false}, // AES-192
			{"32ByteKey", 32, false}, // AES-256
			{"InvalidKey_8Bytes", 8, true},
			{"InvalidKey_15Bytes", 15, true},
			{"InvalidKey_33Bytes", 33, true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				key := make([]byte, tc.keySize)
				for i := range key {
					key[i] = byte(i)
				}

				service := &StorageService{
					encryptionKey: key,
				}

				encrypted, err := service.encrypt("test content")
				if tc.shouldErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.NotEqual(t, "test content", encrypted)
				}
			})
		}
	})

	// Test decryption with wrong keys
	t.Run("DecryptionWithWrongKey", func(t *testing.T) {
		// Encrypt with one key
		key1 := []byte("12345678901234567890123456789012")
		service1 := &StorageService{
			encryptionKey: key1,
		}

		encrypted, err := service1.encrypt("sensitive data")
		require.NoError(t, err)

		// Try to decrypt with different key
		key2 := []byte("abcdefghijklmnopqrstuvwxyz123456")
		service2 := &StorageService{
			encryptionKey: key2,
		}

		_, err = service2.decrypt(encrypted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decrypt")
	})

	// Test encryption error scenarios
	t.Run("EncryptionErrorScenarios", func(t *testing.T) {
		// Test with empty key
		service := &StorageService{
			encryptionKey: []byte{},
		}

		encrypted, err := service.encrypt("test")
		assert.NoError(t, err)
		assert.Equal(t, "test", encrypted) // Should return plaintext when no key

		// Test with nil key
		service = &StorageService{
			encryptionKey: nil,
		}

		encrypted, err = service.encrypt("test")
		assert.NoError(t, err)
		assert.Equal(t, "test", encrypted) // Should return plaintext when no key
	})

	// Test decryption error scenarios
	t.Run("DecryptionErrorScenarios", func(t *testing.T) {
		key := []byte("12345678901234567890123456789012")
		service := &StorageService{
			encryptionKey: key,
		}

		// Test with invalid base64
		_, err := service.decrypt("not-valid-base64!@#$")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode base64")

		// Test with too short ciphertext
		shortData := "YWJj" // "abc" in base64
		_, err = service.decrypt(shortData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ciphertext too short")

		// Test with corrupted ciphertext
		encrypted, err := service.encrypt("test data")
		require.NoError(t, err)

		// Corrupt the encrypted data
		corrupted := encrypted[:len(encrypted)-4] + "XXXX"
		_, err = service.decrypt(corrupted)
		assert.Error(t, err)
	})

	// Test encryption with special characters
	t.Run("EncryptionWithSpecialCharacters", func(t *testing.T) {
		key := []byte("12345678901234567890123456789012")
		service := &StorageService{
			encryptionKey: key,
		}

		specialContent := "Hello 世界! 🌍 Special chars: @#$%^&*()"
		encrypted, err := service.encrypt(specialContent)
		assert.NoError(t, err)

		decrypted, err := service.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, specialContent, decrypted)
	})

	// Test encryption with very long content
	t.Run("EncryptionWithLongContent", func(t *testing.T) {
		key := []byte("12345678901234567890123456789012")
		service := &StorageService{
			encryptionKey: key,
		}

		// Create a long string (10KB)
		longContent := ""
		for i := 0; i < 1000; i++ {
			longContent += "0123456789"
		}

		encrypted, err := service.encrypt(longContent)
		assert.NoError(t, err)

		decrypted, err := service.decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, longContent, decrypted)
	})
}

// TestEncryption_Integration tests encryption with MongoDB storage
func TestEncryption_Integration(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	encryptionKey := []byte("12345678901234567890123456789012")
	service := NewStorageService(mongoClient, "chatbox", "encryption_test", logger, encryptionKey)

	t.Run("MessageEncryptionInStorage", func(t *testing.T) {
		// Create session
		sess := &session.Session{
			ID:        "encrypt-integration-1",
			UserID:    "user-encrypt-1",
			Name:      "Encryption Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		// Add message with sensitive content
		sensitiveContent := "This is sensitive information that should be encrypted"
		msg := &session.Message{
			Content:   sensitiveContent,
			Timestamp: time.Now(),
			Sender:    "user",
		}

		err = service.AddMessage("encrypt-integration-1", msg)
		assert.NoError(t, err)

		// Verify content is encrypted in database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var doc SessionDocument
		err = service.collection.FindOne(ctx, bson.M{"_id": "encrypt-integration-1"}).Decode(&doc)
		assert.NoError(t, err)
		assert.Len(t, doc.Messages, 1)
		assert.NotEqual(t, sensitiveContent, doc.Messages[0].Content)

		// Verify content is decrypted when retrieved
		retrieved, err := service.GetSession("encrypt-integration-1")
		assert.NoError(t, err)
		assert.Len(t, retrieved.Messages, 1)
		assert.Equal(t, sensitiveContent, retrieved.Messages[0].Content)
	})

	t.Run("MultipleMessagesEncryption", func(t *testing.T) {
		// Create session
		sess := &session.Session{
			ID:        "encrypt-integration-2",
			UserID:    "user-encrypt-2",
			Name:      "Multiple Messages Test",
			Messages:  []*session.Message{},
			StartTime: time.Now(),
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		// Add multiple messages
		messages := []string{
			"First sensitive message",
			"Second sensitive message",
			"Third sensitive message",
		}

		for i, content := range messages {
			msg := &session.Message{
				Content:   content,
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
				Sender:    "user",
			}

			err = service.AddMessage("encrypt-integration-2", msg)
			assert.NoError(t, err)
		}

		// Retrieve and verify all messages are decrypted correctly
		retrieved, err := service.GetSession("encrypt-integration-2")
		assert.NoError(t, err)
		assert.Len(t, retrieved.Messages, 3)

		for i, msg := range retrieved.Messages {
			assert.Equal(t, messages[i], msg.Content)
		}
	})
}


// TestRetryLogic_Coverage tests retry logic with various scenarios
func TestRetryLogic_Coverage(t *testing.T) {
	// Test transient error retry behavior
	t.Run("TransientErrorRetry", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		attemptCount := 0
		operation := func() error {
			attemptCount++
			if attemptCount < 3 {
				return &customError{msg: "connection timeout"}
			}
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.NoError(t, err)
		assert.Equal(t, 3, attemptCount)
	})

	// Test permanent error immediate failure
	t.Run("PermanentErrorNoRetry", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		attemptCount := 0
		operation := func() error {
			attemptCount++
			return &customError{msg: "duplicate key error"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.Error(t, err)
		assert.Equal(t, 1, attemptCount) // Should not retry
	})

	// Test exponential backoff timing
	t.Run("ExponentialBackoff", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		var delays []time.Duration
		var lastTime time.Time
		firstCall := true

		operation := func() error {
			now := time.Now()
			if !firstCall {
				delays = append(delays, now.Sub(lastTime))
			}
			firstCall = false
			lastTime = now
			return &customError{msg: "connection refused"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.Error(t, err)

		// Verify we have delays recorded
		assert.NotEmpty(t, delays)

		// Verify delays are increasing (exponential backoff)
		for i := 1; i < len(delays); i++ {
			// Each delay should be at least as long as the previous
			// (allowing for some timing variance)
			assert.GreaterOrEqual(t, delays[i], delays[i-1]-50*time.Millisecond)
		}
	})

	// Test maximum retry limit
	t.Run("MaximumRetryLimit", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		attemptCount := 0
		operation := func() error {
			attemptCount++
			return &customError{msg: "timeout"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.Error(t, err)
		assert.Equal(t, defaultRetryConfig.maxAttempts, attemptCount)
		assert.Contains(t, err.Error(), "operation failed after")
	})

	// Test context cancellation during retry
	t.Run("ContextCancellation", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		operation := func() error {
			return &customError{msg: "connection timeout"}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operation cancelled during retry")
	})

	// Test successful operation on first attempt
	t.Run("SuccessOnFirstAttempt", func(t *testing.T) {
		logger, err := golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            t.TempDir(),
			InfoFile:       "info.log",
			WarnFile:       "warn.log",
			ErrorFile:      "error.log",
		})
		require.NoError(t, err)
		defer logger.Close()

		service := &StorageService{
			logger: logger,
		}

		attemptCount := 0
		operation := func() error {
			attemptCount++
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = service.retryOperation(ctx, "TestOp", operation)
		assert.NoError(t, err)
		assert.Equal(t, 1, attemptCount)
	})
}


// TestMongoDBIntegration_Coverage tests MongoDB integration functionality
func TestMongoDBIntegration_Coverage(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "mongo_integration", logger, nil)

	// Test index creation verification
	t.Run("IndexCreation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := service.EnsureIndexes(ctx)
		assert.NoError(t, err)

		// Verify indexes were created by attempting to use them in queries
		// MongoDB will use indexes automatically for matching queries
		sess := &session.Session{
			ID:            "index-test-1",
			UserID:        "user-index-1",
			Name:          "Index Test",
			Messages:      []*session.Message{},
			StartTime:     time.Now(),
			AdminAssisted: true,
		}

		err = service.CreateSession(sess)
		require.NoError(t, err)

		// Query using indexed fields
		opts := &SessionListOptions{
			UserID:    "user-index-1",
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
	})

	// Test BSON field mapping
	t.Run("BSONFieldMapping", func(t *testing.T) {
		now := time.Now()
		endTime := now.Add(10 * time.Minute)

		sess := &session.Session{
			ID:                 "bson-test-1",
			UserID:             "user-bson-1",
			Name:               "BSON Test",
			ModelID:            "gpt-4",
			Messages:           []*session.Message{},
			StartTime:          now,
			EndTime:            &endTime,
			AdminAssisted:      true,
			AssistingAdminID:   "admin-123",
			AssistingAdminName: "Admin User",
			HelpRequested:      true,
			TotalTokens:        500,
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		// Retrieve and verify all BSON fields are mapped correctly
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var doc SessionDocument
		err = service.collection.FindOne(ctx, bson.M{"_id": "bson-test-1"}).Decode(&doc)
		assert.NoError(t, err)

		// Verify BSON field mappings
		assert.Equal(t, "bson-test-1", doc.ID)
		assert.Equal(t, "user-bson-1", doc.UserID)
		assert.Equal(t, "BSON Test", doc.Name)
		assert.Equal(t, "gpt-4", doc.ModelID)
		assert.True(t, doc.AdminAssisted)
		assert.Equal(t, "admin-123", doc.AssistingAdminID)
		assert.Equal(t, "Admin User", doc.AssistingAdminName)
		assert.True(t, doc.HelpRequested)
		assert.Equal(t, 500, doc.TotalTokens)
		assert.NotNil(t, doc.EndTime)
		assert.Equal(t, endTime.Unix(), doc.EndTime.Unix())
	})

	// Test query construction
	t.Run("QueryConstruction", func(t *testing.T) {
		now := time.Now()

		// Create test sessions with different attributes
		sessions := []struct {
			id            string
			userID        string
			startTime     time.Time
			adminAssisted bool
			active        bool
		}{
			{"query-1", "user-query-1", now.Add(-3 * time.Hour), true, true},
			{"query-2", "user-query-2", now.Add(-2 * time.Hour), false, true},
			{"query-3", "user-query-1", now.Add(-1 * time.Hour), true, false},
		}

		for _, s := range sessions {
			sess := &session.Session{
				ID:            s.id,
				UserID:        s.userID,
				Name:          "Query Test",
				Messages:      []*session.Message{},
				StartTime:     s.startTime,
				AdminAssisted: s.adminAssisted,
				IsActive:      s.active,
			}

			err := service.CreateSession(sess)
			require.NoError(t, err)

			if !s.active {
				err = service.EndSession(s.id, s.startTime.Add(30*time.Minute))
				require.NoError(t, err)
			}
		}

		// Test query with user ID filter
		opts := &SessionListOptions{
			UserID:    "user-query-1",
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.Equal(t, "user-query-1", r.UserID)
		}

		// Test query with admin assisted filter
		adminAssisted := true
		opts = &SessionListOptions{
			AdminAssisted: &adminAssisted,
			Limit:         10,
			SortBy:        "ts",
			SortOrder:     "desc",
		}

		results, err = service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
		for _, r := range results {
			assert.True(t, r.AdminAssisted)
		}

		// Test query with active filter
		active := true
		opts = &SessionListOptions{
			Active:    &active,
			Limit:     10,
			SortBy:    "ts",
			SortOrder: "desc",
		}

		results, err = service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
		for _, r := range results {
			assert.Nil(t, r.EndTime)
		}

		// Test query with time range filter
		startTimeFrom := now.Add(-2*time.Hour - 30*time.Minute)
		startTimeTo := now.Add(-30 * time.Minute)
		opts = &SessionListOptions{
			StartTimeFrom: &startTimeFrom,
			StartTimeTo:   &startTimeTo,
			Limit:         10,
			SortBy:        "ts",
			SortOrder:     "desc",
		}

		results, err = service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, results)
		for _, r := range results {
			assert.True(t, r.StartTime.After(startTimeFrom) || r.StartTime.Equal(startTimeFrom))
			assert.True(t, r.StartTime.Before(startTimeTo) || r.StartTime.Equal(startTimeTo))
		}
	})

	// Test result parsing
	t.Run("ResultParsing", func(t *testing.T) {
		now := time.Now()

		// Create session with complex data
		sess := &session.Session{
			ID:     "parse-test-1",
			UserID: "user-parse-1",
			Name:   "Parse Test",
			Messages: []*session.Message{
				{
					Content:   "Message 1",
					Timestamp: now,
					Sender:    "user",
					FileID:    "file-123",
					FileURL:   "https://example.com/file.pdf",
					Metadata:  map[string]string{"key1": "value1", "key2": "value2"},
				},
				{
					Content:   "Message 2",
					Timestamp: now.Add(time.Minute),
					Sender:    "ai",
					Metadata:  map[string]string{"key3": "value3"},
				},
			},
			StartTime:     now,
			TotalTokens:   250,
			ResponseTimes: []time.Duration{time.Second, 2 * time.Second},
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)

		// Retrieve and verify parsing
		retrieved, err := service.GetSession("parse-test-1")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)

		// Verify messages are parsed correctly
		assert.Len(t, retrieved.Messages, 2)

		msg1 := retrieved.Messages[0]
		assert.Equal(t, "Message 1", msg1.Content)
		assert.Equal(t, "user", msg1.Sender)
		assert.Equal(t, "file-123", msg1.FileID)
		assert.Equal(t, "https://example.com/file.pdf", msg1.FileURL)
		assert.Equal(t, "value1", msg1.Metadata["key1"])
		assert.Equal(t, "value2", msg1.Metadata["key2"])

		msg2 := retrieved.Messages[1]
		assert.Equal(t, "Message 2", msg2.Content)
		assert.Equal(t, "ai", msg2.Sender)
		assert.Equal(t, "value3", msg2.Metadata["key3"])

		// Verify session metadata is parsed correctly
		assert.Equal(t, 250, retrieved.TotalTokens)
		assert.Equal(t, now.Unix(), retrieved.StartTime.Unix())
	})

	// Test aggregation pipeline for token usage
	t.Run("AggregationPipeline", func(t *testing.T) {
		now := time.Now()

		// Create sessions with different token counts
		tokenCounts := []int{100, 200, 300, 400, 500}
		for i, tokens := range tokenCounts {
			sess := &session.Session{
				ID:          fmt.Sprintf("agg-test-%d", i),
				UserID:      "user-agg-1",
				Name:        "Aggregation Test",
				Messages:    []*session.Message{},
				StartTime:   now.Add(-time.Duration(i) * time.Hour),
				TotalTokens: tokens,
			}

			err := service.CreateSession(sess)
			require.NoError(t, err)
		}

		// Test token usage aggregation
		startTime := now.Add(-24 * time.Hour)
		endTime := now

		totalTokens, err := service.GetTokenUsage(startTime, endTime)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, totalTokens, 1500) // Sum of tokenCounts
	})

	// Test metrics calculation
	t.Run("MetricsCalculation", func(t *testing.T) {
		now := time.Now()

		// Create sessions for metrics
		for i := 0; i < 5; i++ {
			sess := &session.Session{
				ID:            fmt.Sprintf("metrics-test-%d", i),
				UserID:        fmt.Sprintf("user-metrics-%d", i),
				Name:          "Metrics Test",
				Messages:      []*session.Message{},
				StartTime:     now.Add(-time.Duration(i) * time.Hour),
				TotalTokens:   100 * (i + 1),
				AdminAssisted: i%2 == 0,
				ResponseTimes: []time.Duration{time.Duration(i+1) * time.Second},
			}

			err := service.CreateSession(sess)
			require.NoError(t, err)

			// End some sessions
			if i%2 == 1 {
				err = service.EndSession(sess.ID, now.Add(-time.Duration(i)*time.Hour+30*time.Minute))
				require.NoError(t, err)
			}
		}

		// Get metrics
		startTime := now.Add(-24 * time.Hour)
		endTime := now

		metrics, err := service.GetSessionMetrics(startTime, endTime)
		assert.NoError(t, err)
		assert.NotNil(t, metrics)
		assert.GreaterOrEqual(t, metrics.TotalSessions, 5)
		assert.GreaterOrEqual(t, metrics.TotalTokens, 1500)
		assert.GreaterOrEqual(t, metrics.AdminAssistedCount, 3)
	})
}
