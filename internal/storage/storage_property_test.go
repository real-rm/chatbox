package storage

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/real-rm/chatbox/internal/session"
	"go.mongodb.org/mongo-driver/bson"
)

// Feature: chat-application-websocket, Property 14: Session Creation and Persistence
// **Validates: Requirements 4.1**
//
// For any new session, the Storage_Service should create a session record in MongoDB
// with a unique identifier, user ID, start timestamp, and all required metadata fields.
func TestProperty_SessionCreationAndPersistence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("session is created and persisted with all required fields", prop.ForAll(
		func(sessionID, userID, name, modelID string) bool {
			// Skip if required fields are empty
			if sessionID == "" || userID == "" {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create session
			now := time.Now()
			sess := &session.Session{
				ID:            sessionID,
				UserID:        userID,
				Name:          name,
				ModelID:       modelID,
				Messages:      []*session.Message{},
				StartTime:     now,
				LastActivity:  now,
				EndTime:       nil,
				IsActive:      true,
				HelpRequested: false,
				AdminAssisted: false,
				TotalTokens:   0,
				ResponseTimes: []time.Duration{},
			}

			// Create session in database
			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Retrieve session
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			// Verify all required fields are persisted
			if retrievedSess.ID != sessionID {
				t.Logf("Session ID mismatch: expected %s, got %s", sessionID, retrievedSess.ID)
				return false
			}
			if retrievedSess.UserID != userID {
				t.Logf("User ID mismatch: expected %s, got %s", userID, retrievedSess.UserID)
				return false
			}
			if retrievedSess.Name != name {
				t.Logf("Name mismatch: expected %s, got %s", name, retrievedSess.Name)
				return false
			}
			if retrievedSess.ModelID != modelID {
				t.Logf("Model ID mismatch: expected %s, got %s", modelID, retrievedSess.ModelID)
				return false
			}
			if retrievedSess.StartTime.Unix() != now.Unix() {
				t.Logf("Start time mismatch")
				return false
			}

			return true
		},
		gen.Identifier(),  // sessionID
		gen.Identifier(),  // userID
		gen.AlphaString(), // name
		gen.AlphaString(), // modelID
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 15: Message Persistence
// **Validates: Requirements 4.2, 4.6**
//
// For any message sent or received in a session, the Storage_Service should immediately
// persist it to the session record with all required fields (content, timestamp, sender type, metadata).
func TestProperty_MessagePersistence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("messages are persisted immediately with all required fields", prop.ForAll(
		func(sessionID, userID, content, sender string, metadata map[string]string) bool {
			// Skip if required fields are empty
			if sessionID == "" || userID == "" || content == "" || sender == "" {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create session first
			now := time.Now()
			sess := &session.Session{
				ID:        sessionID,
				UserID:    userID,
				Name:      "Test Session",
				Messages:  []*session.Message{},
				StartTime: now,
			}

			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Add message
			msg := &session.Message{
				Content:   content,
				Timestamp: now,
				Sender:    sender,
				FileID:    "",
				FileURL:   "",
				Metadata:  metadata,
			}

			err = service.AddMessage(sessionID, msg)
			if err != nil {
				t.Logf("Failed to add message: %v", err)
				return false
			}

			// Retrieve session and verify message
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			if len(retrievedSess.Messages) != 1 {
				t.Logf("Expected 1 message, got %d", len(retrievedSess.Messages))
				return false
			}

			retrievedMsg := retrievedSess.Messages[0]
			if retrievedMsg.Content != content {
				t.Logf("Content mismatch: expected %s, got %s", content, retrievedMsg.Content)
				return false
			}
			if retrievedMsg.Sender != sender {
				t.Logf("Sender mismatch: expected %s, got %s", sender, retrievedMsg.Sender)
				return false
			}
			if retrievedMsg.Timestamp.Unix() != now.Unix() {
				t.Logf("Timestamp mismatch")
				return false
			}

			// Verify metadata
			if len(metadata) > 0 {
				if retrievedMsg.Metadata == nil {
					t.Logf("Metadata is nil")
					return false
				}
				for key, value := range metadata {
					if retrievedMsg.Metadata[key] != value {
						t.Logf("Metadata mismatch for key %s: expected %s, got %s", key, value, retrievedMsg.Metadata[key])
						return false
					}
				}
			}

			return true
		},
		gen.Identifier(), // sessionID
		gen.Identifier(), // userID
		gen.AlphaString().SuchThat(func(s string) bool { // content
			return len(s) > 0
		}),
		gen.OneConstOf("user", "ai", "admin"),          // sender
		gen.MapOf(gen.Identifier(), gen.AlphaString()), // metadata
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 16: Conversation History Retrieval
// **Validates: Requirements 4.3**
//
// For any session with stored messages, retrieving the conversation history should
// return all messages ordered by timestamp.
func TestProperty_ConversationHistoryRetrieval(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("conversation history is retrieved with messages ordered by timestamp", prop.ForAll(
		func(sessionID, userID string, messageCount uint8) bool {
			// Skip if required fields are empty or message count is too large
			if sessionID == "" || userID == "" || messageCount == 0 || messageCount > 10 {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create session
			now := time.Now()
			sess := &session.Session{
				ID:        sessionID,
				UserID:    userID,
				Name:      "Test Session",
				Messages:  []*session.Message{},
				StartTime: now,
			}

			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Add multiple messages with increasing timestamps
			for i := uint8(0); i < messageCount; i++ {
				msg := &session.Message{
					Content:   "Message " + string(rune('A'+i)),
					Timestamp: now.Add(time.Duration(i) * time.Second),
					Sender:    "user",
					Metadata:  map[string]string{"index": string(rune('0' + i))},
				}

				err = service.AddMessage(sessionID, msg)
				if err != nil {
					t.Logf("Failed to add message %d: %v", i, err)
					return false
				}
			}

			// Retrieve session
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			// Verify message count
			if len(retrievedSess.Messages) != int(messageCount) {
				t.Logf("Expected %d messages, got %d", messageCount, len(retrievedSess.Messages))
				return false
			}

			// Verify messages are ordered by timestamp
			for i := 0; i < len(retrievedSess.Messages)-1; i++ {
				if retrievedSess.Messages[i].Timestamp.After(retrievedSess.Messages[i+1].Timestamp) {
					t.Logf("Messages not ordered by timestamp at index %d", i)
					return false
				}
			}

			return true
		},
		gen.Identifier(),      // sessionID
		gen.Identifier(),      // userID
		gen.UInt8Range(1, 10), // messageCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 18: Session Lifecycle Tracking
// **Validates: Requirements 4.7**
//
// For any session that ends, the Storage_Service should update the session record
// with end timestamp and calculated duration.
func TestProperty_SessionLifecycleTracking(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("session end time and duration are tracked correctly", prop.ForAll(
		func(sessionID, userID string, durationMinutes uint8) bool {
			// Skip if required fields are empty or duration is too large
			if sessionID == "" || userID == "" || durationMinutes == 0 || durationMinutes > 60 {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create session
			startTime := time.Now()
			sess := &session.Session{
				ID:        sessionID,
				UserID:    userID,
				Name:      "Test Session",
				Messages:  []*session.Message{},
				StartTime: startTime,
				IsActive:  true,
			}

			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// End session after specified duration
			endTime := startTime.Add(time.Duration(durationMinutes) * time.Minute)
			err = service.EndSession(sessionID, endTime)
			if err != nil {
				t.Logf("Failed to end session: %v", err)
				return false
			}

			// Retrieve session and verify end time and duration
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			if retrievedSess.EndTime == nil {
				t.Logf("End time is nil")
				return false
			}

			if retrievedSess.EndTime.Unix() != endTime.Unix() {
				t.Logf("End time mismatch: expected %v, got %v", endTime, retrievedSess.EndTime)
				return false
			}

			// Verify session is marked as inactive
			if retrievedSess.IsActive {
				t.Logf("Session should be inactive after ending")
				return false
			}

			// Calculate expected duration in seconds
			expectedDuration := int64(durationMinutes) * 60

			// Get the document directly to check duration field
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var doc SessionDocument
			err = service.collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
			if err != nil {
				t.Logf("Failed to get document: %v", err)
				return false
			}

			// Allow small tolerance for duration calculation (1 second)
			if doc.Duration < expectedDuration-1 || doc.Duration > expectedDuration+1 {
				t.Logf("Duration mismatch: expected ~%d seconds, got %d seconds", expectedDuration, doc.Duration)
				return false
			}

			return true
		},
		gen.Identifier(),      // sessionID
		gen.Identifier(),      // userID
		gen.UInt8Range(1, 60), // durationMinutes
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 44: Data Encryption at Rest
// **Validates: Requirements 13.4**
//
// For any sensitive data stored in MongoDB, the Storage_Service should encrypt it at rest.
func TestProperty_DataEncryptionAtRest(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("sensitive message content is encrypted at rest", prop.ForAll(
		func(sessionID, userID, content string) bool {
			// Skip if required fields are empty
			if sessionID == "" || userID == "" || content == "" {
				return true
			}

			// Create 32-byte encryption key for AES-256
			encryptionKey := []byte("12345678901234567890123456789012")
			service, cleanup := setupTestStorage(t, encryptionKey)
			defer cleanup()

			// Create session
			now := time.Now()
			sess := &session.Session{
				ID:        sessionID,
				UserID:    userID,
				Name:      "Test Session",
				Messages:  []*session.Message{},
				StartTime: now,
			}

			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Add message with sensitive content
			msg := &session.Message{
				Content:   content,
				Timestamp: now,
				Sender:    "user",
			}

			err = service.AddMessage(sessionID, msg)
			if err != nil {
				t.Logf("Failed to add message: %v", err)
				return false
			}

			// Verify content is encrypted in database (raw document)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var doc SessionDocument
			err = service.collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
			if err != nil {
				t.Logf("Failed to get document: %v", err)
				return false
			}

			if len(doc.Messages) != 1 {
				t.Logf("Expected 1 message in document")
				return false
			}

			// Content should be encrypted (not equal to plaintext)
			if doc.Messages[0].Content == content {
				t.Logf("Content is not encrypted in database")
				return false
			}

			// Verify content can be decrypted when retrieved through service
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			if len(retrievedSess.Messages) != 1 {
				t.Logf("Expected 1 message in retrieved session")
				return false
			}

			if retrievedSess.Messages[0].Content != content {
				t.Logf("Decrypted content mismatch: expected %s, got %s", content, retrievedSess.Messages[0].Content)
				return false
			}

			return true
		},
		gen.Identifier(), // sessionID
		gen.Identifier(), // userID
		gen.AlphaString().SuchThat(func(s string) bool { // content
			return len(s) > 0
		}),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 46: Session List Ordering
// **Validates: Requirements 15.1**
//
// For any user's session list request, the sessions should be returned ordered by
// most recent activity (last message timestamp).
func TestProperty_SessionListOrdering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("user sessions are ordered by most recent activity", prop.ForAll(
		func(userID string, sessionCount uint8) bool {
			// Skip if required fields are empty or session count is too large
			if userID == "" || sessionCount == 0 || sessionCount > 5 {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create multiple sessions with different start times
			now := time.Now()
			sessionIDs := make([]string, sessionCount)

			for i := uint8(0); i < sessionCount; i++ {
				sessionID := userID + "-session-" + string(rune('A'+i))
				sessionIDs[i] = sessionID

				// Create sessions with decreasing start times (older sessions first)
				startTime := now.Add(-time.Duration(sessionCount-i) * time.Hour)

				sess := &session.Session{
					ID:        sessionID,
					UserID:    userID,
					Name:      "Session " + string(rune('A'+i)),
					Messages:  []*session.Message{},
					StartTime: startTime,
				}

				err := service.CreateSession(sess)
				if err != nil {
					t.Logf("Failed to create session %s: %v", sessionID, err)
					return false
				}
			}

			// List user sessions
			metadata, err := service.ListUserSessions(userID, 0)
			if err != nil {
				t.Logf("Failed to list user sessions: %v", err)
				return false
			}

			// Verify session count
			if len(metadata) != int(sessionCount) {
				t.Logf("Expected %d sessions, got %d", sessionCount, len(metadata))
				return false
			}

			// Verify sessions are ordered by start time (most recent first)
			// Since we created sessions with decreasing start times, the order should be reversed
			for i := 0; i < len(metadata)-1; i++ {
				if metadata[i].StartTime.Before(metadata[i+1].StartTime) {
					t.Logf("Sessions not ordered by most recent activity at index %d", i)
					t.Logf("Session %d start time: %v", i, metadata[i].StartTime)
					t.Logf("Session %d start time: %v", i+1, metadata[i+1].StartTime)
					return false
				}
			}

			// Verify the most recent session is first
			expectedFirstSessionID := userID + "-session-" + string(rune('A'+sessionCount-1))
			if metadata[0].ID != expectedFirstSessionID {
				t.Logf("Expected first session to be %s, got %s", expectedFirstSessionID, metadata[0].ID)
				return false
			}

			return true
		},
		gen.Identifier(),     // userID
		gen.UInt8Range(1, 5), // sessionCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 4: Storage Operations Maintain Data Integrity
// **Validates: Requirements 2.2, 8.4**
//
// For any valid session object, storing it to MongoDB and then retrieving it
// should return an equivalent session with all fields preserved.
func TestProperty_StorageDataIntegrity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("storing and retrieving a session preserves all fields", prop.ForAll(
		func(sessionID, userID, name, modelID string, totalTokens int) bool {
			// Skip if required fields are empty
			if sessionID == "" || userID == "" {
				return true
			}

			// Ensure totalTokens is non-negative
			if totalTokens < 0 {
				totalTokens = -totalTokens
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create session with various fields
			now := time.Now()
			endTime := now.Add(10 * time.Minute)
			originalSession := &session.Session{
				ID:                 sessionID,
				UserID:             userID,
				Name:               name,
				ModelID:            modelID,
				Messages:           []*session.Message{},
				StartTime:          now,
				LastActivity:       now,
				EndTime:            &endTime,
				IsActive:           false,
				HelpRequested:      true,
				AdminAssisted:      true,
				AssistingAdminID:   "admin-123",
				AssistingAdminName: "Admin User",
				TotalTokens:        totalTokens,
				ResponseTimes:      []time.Duration{time.Second, 2 * time.Second},
			}

			// Store session
			err := service.CreateSession(originalSession)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Retrieve session
			retrievedSession, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			// Verify all fields are preserved
			if retrievedSession.ID != originalSession.ID {
				t.Logf("ID mismatch: expected %s, got %s", originalSession.ID, retrievedSession.ID)
				return false
			}
			if retrievedSession.UserID != originalSession.UserID {
				t.Logf("UserID mismatch: expected %s, got %s", originalSession.UserID, retrievedSession.UserID)
				return false
			}
			if retrievedSession.Name != originalSession.Name {
				t.Logf("Name mismatch: expected %s, got %s", originalSession.Name, retrievedSession.Name)
				return false
			}
			if retrievedSession.ModelID != originalSession.ModelID {
				t.Logf("ModelID mismatch: expected %s, got %s", originalSession.ModelID, retrievedSession.ModelID)
				return false
			}
			if retrievedSession.TotalTokens != originalSession.TotalTokens {
				t.Logf("TotalTokens mismatch: expected %d, got %d", originalSession.TotalTokens, retrievedSession.TotalTokens)
				return false
			}
			if retrievedSession.HelpRequested != originalSession.HelpRequested {
				t.Logf("HelpRequested mismatch")
				return false
			}
			if retrievedSession.AdminAssisted != originalSession.AdminAssisted {
				t.Logf("AdminAssisted mismatch")
				return false
			}
			if retrievedSession.AssistingAdminID != originalSession.AssistingAdminID {
				t.Logf("AssistingAdminID mismatch")
				return false
			}
			if retrievedSession.AssistingAdminName != originalSession.AssistingAdminName {
				t.Logf("AssistingAdminName mismatch")
				return false
			}
			if retrievedSession.StartTime.Unix() != originalSession.StartTime.Unix() {
				t.Logf("StartTime mismatch")
				return false
			}
			if retrievedSession.EndTime == nil || retrievedSession.EndTime.Unix() != originalSession.EndTime.Unix() {
				t.Logf("EndTime mismatch")
				return false
			}
			if retrievedSession.IsActive != originalSession.IsActive {
				t.Logf("IsActive mismatch")
				return false
			}

			return true
		},
		gen.Identifier(),       // sessionID
		gen.Identifier(),       // userID
		gen.AlphaString(),      // name
		gen.AlphaString(),      // modelID
		gen.IntRange(0, 10000), // totalTokens
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 5: Encryption Round-Trip Preserves Data
// **Validates: Requirements 2.4, 6.1**
//
// For any valid encryption key and any message content, encrypting then decrypting
// the content should return the original message unchanged.
func TestProperty_EncryptionRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("encryption and decryption round-trip preserves data", prop.ForAll(
		func(content string) bool {
			// Skip empty content
			if content == "" {
				return true
			}

			// Create 32-byte encryption key for AES-256
			encryptionKey := []byte("12345678901234567890123456789012")
			service := &StorageService{
				encryptionKey: encryptionKey,
			}

			// Encrypt
			encrypted, err := service.encrypt(content)
			if err != nil {
				t.Logf("Failed to encrypt: %v", err)
				return false
			}

			// Verify encrypted is different from original
			if encrypted == content {
				t.Logf("Encrypted content is same as original")
				return false
			}

			// Decrypt
			decrypted, err := service.decrypt(encrypted)
			if err != nil {
				t.Logf("Failed to decrypt: %v", err)
				return false
			}

			// Verify decrypted matches original
			if decrypted != content {
				t.Logf("Decrypted content mismatch: expected %s, got %s", content, decrypted)
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool {
			return len(s) > 0 && len(s) < 1000 // Reasonable size for testing
		}),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 12: Invalid Encryption Keys Are Rejected
// **Validates: Requirements 6.2**
//
// For any encryption key that is not exactly 32 bytes, the encryption function
// should return an error indicating the key is invalid.
func TestProperty_InvalidEncryptionKeys(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 50
	properties := gopter.NewProperties(parameters)

	properties.Property("invalid encryption keys are rejected", prop.ForAll(
		func(keySize uint8) bool {
			// Skip valid key sizes (0 = no encryption, 16/24/32 = valid AES)
			if keySize == 0 || keySize == 16 || keySize == 24 || keySize == 32 {
				return true
			}

			// Create key with invalid size
			invalidKey := make([]byte, keySize)
			for i := range invalidKey {
				invalidKey[i] = byte(i)
			}

			service := &StorageService{
				encryptionKey: invalidKey,
			}

			// Try to encrypt with invalid key
			_, err := service.encrypt("test content")

			// Should get an error for invalid key sizes
			// Note: AES accepts 16, 24, or 32 byte keys
			// keySize 0 is treated as "no encryption" and is valid
			if err == nil {
				t.Logf("Expected error for key size %d, but got none", keySize)
				return false
			}

			return true
		},
		gen.UInt8Range(0, 64), // Test various key sizes
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 6: Transient Errors Trigger Retry Logic
// **Validates: Requirements 2.3, 5.1, 8.3**
//
// For any storage operation that fails with a transient error (network timeout,
// connection reset), the system should retry the operation with exponential backoff
// up to the maximum retry limit.
func TestProperty_TransientErrorRetryClassification(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("transient errors trigger retry logic", prop.ForAll(
		func(errorType string) bool {
			// Test various transient error types
			transientErrors := []string{
				"connection refused",
				"connection reset",
				"timeout",
				"temporary failure",
				"i/o timeout",
				"EOF",
				"server selection timeout",
				"no reachable servers",
				"connection pool",
				"socket",
			}

			// Check if error type is in transient errors list
			isTransient := false
			for _, te := range transientErrors {
				if errorType == te {
					isTransient = true
					break
				}
			}

			if !isTransient {
				return true // Skip non-transient errors
			}

			// Create error with transient message
			err := &customError{msg: errorType}

			// Verify it's classified as retryable
			result := isRetryableError(err)
			if !result {
				t.Logf("Error '%s' should be retryable but was not", errorType)
				return false
			}

			return true
		},
		gen.OneConstOf(
			"connection refused",
			"connection reset",
			"timeout",
			"temporary failure",
			"i/o timeout",
			"EOF",
			"server selection timeout",
			"no reachable servers",
			"connection pool",
			"socket",
			"duplicate key", // permanent error for contrast
		),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 7: Permanent Errors Fail Immediately
// **Validates: Requirements 5.2**
//
// For any storage operation that fails with a permanent error (invalid document,
// duplicate key), the system should return the error immediately without retrying.
func TestProperty_PermanentErrorFailure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("permanent errors fail immediately without retry", prop.ForAll(
		func(errorType string) bool {
			// Test various permanent error types
			permanentErrors := []string{
				"duplicate key error",
				"validation failed",
				"document too large",
				"invalid document",
				"unauthorized",
			}

			// Check if error type is in permanent errors list
			isPermanent := false
			for _, pe := range permanentErrors {
				if errorType == pe {
					isPermanent = true
					break
				}
			}

			if !isPermanent {
				return true // Skip non-permanent errors
			}

			// Create error with permanent message
			err := &customError{msg: errorType}

			// Verify it's NOT classified as retryable
			result := isRetryableError(err)
			if result {
				t.Logf("Error '%s' should not be retryable but was", errorType)
				return false
			}

			return true
		},
		gen.OneConstOf(
			"duplicate key error",
			"validation failed",
			"document too large",
			"invalid document",
			"unauthorized",
			"timeout", // transient error for contrast
		),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 8: Concurrent Operations Maintain Consistency
// **Validates: Requirements 2.5, 7.1, 7.2, 7.3**
//
// For any set of concurrent storage operations (session creation, message addition,
// session updates), all operations should complete successfully without data loss,
// corruption, or race conditions.
func TestProperty_ConcurrentOperations(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("concurrent operations maintain data consistency", prop.ForAll(
		func(userID string, operationCount uint8) bool {
			// Skip if required fields are empty or operation count is too large
			if userID == "" || operationCount == 0 || operationCount > 10 {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create initial session
			sessionID := userID + "-concurrent-session"
			now := time.Now()
			sess := &session.Session{
				ID:        sessionID,
				UserID:    userID,
				Name:      "Concurrent Test",
				Messages:  []*session.Message{},
				StartTime: now,
				IsActive:  true,
			}

			err := service.CreateSession(sess)
			if err != nil {
				t.Logf("Failed to create session: %v", err)
				return false
			}

			// Perform concurrent message additions
			done := make(chan bool, operationCount)
			errors := make(chan error, operationCount)

			for i := uint8(0); i < operationCount; i++ {
				go func(index uint8) {
					msg := &session.Message{
						Content:   "Message " + string(rune('A'+index)),
						Timestamp: now.Add(time.Duration(index) * time.Second),
						Sender:    "user",
						Metadata:  map[string]string{"index": string(rune('0' + index))},
					}

					err := service.AddMessage(sessionID, msg)
					if err != nil {
						errors <- err
					}
					done <- true
				}(i)
			}

			// Wait for all operations to complete
			for i := uint8(0); i < operationCount; i++ {
				<-done
			}
			close(errors)

			// Check for errors
			for err := range errors {
				t.Logf("Concurrent operation failed: %v", err)
				return false
			}

			// Retrieve session and verify all messages were added
			retrievedSess, err := service.GetSession(sessionID)
			if err != nil {
				t.Logf("Failed to retrieve session: %v", err)
				return false
			}

			if len(retrievedSess.Messages) != int(operationCount) {
				t.Logf("Expected %d messages, got %d", operationCount, len(retrievedSess.Messages))
				return false
			}

			return true
		},
		gen.Identifier(),      // userID
		gen.UInt8Range(1, 10), // operationCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Property 15: Query Operations Return Correct Results
// **Validates: Requirements 8.2**
//
// For any valid query parameters (filters, sorting, pagination), the storage query
// operations should return results that match the query criteria and are correctly
// sorted and paginated.
func TestProperty_QueryOperations(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 20
	properties := gopter.NewProperties(parameters)

	properties.Property("query operations return correctly filtered and sorted results", prop.ForAll(
		func(userID string, sessionCount uint8, limit uint8) bool {
			// Skip if required fields are empty or counts are invalid
			if userID == "" || sessionCount == 0 || sessionCount > 10 || limit == 0 || limit > 20 {
				return true
			}

			service, cleanup := setupTestStorage(t, nil)
			defer cleanup()

			// Create multiple sessions
			now := time.Now()
			for i := uint8(0); i < sessionCount; i++ {
				sess := &session.Session{
					ID:          userID + "-session-" + string(rune('A'+i)),
					UserID:      userID,
					Name:        "Session " + string(rune('A'+i)),
					Messages:    []*session.Message{},
					StartTime:   now.Add(-time.Duration(i) * time.Hour),
					TotalTokens: int(i) * 100,
				}

				err := service.CreateSession(sess)
				if err != nil {
					t.Logf("Failed to create session: %v", err)
					return false
				}
			}

			// Query with filters and sorting
			opts := &SessionListOptions{
				UserID:    userID,
				Limit:     int(limit),
				SortBy:    "ts",
				SortOrder: "desc",
			}

			results, err := service.ListAllSessionsWithOptions(opts)
			if err != nil {
				t.Logf("Failed to query sessions: %v", err)
				return false
			}

			// Verify results match filter (all should have correct userID)
			for _, result := range results {
				if result.UserID != userID {
					t.Logf("Result has wrong userID: expected %s, got %s", userID, result.UserID)
					return false
				}
			}

			// Verify results are sorted by start time (descending)
			for i := 0; i < len(results)-1; i++ {
				if results[i].StartTime.Before(results[i+1].StartTime) {
					t.Logf("Results not sorted correctly at index %d", i)
					return false
				}
			}

			// Verify limit is respected
			expectedCount := int(sessionCount)
			if int(limit) < expectedCount {
				expectedCount = int(limit)
			}
			if len(results) > expectedCount {
				t.Logf("Results exceed limit: expected max %d, got %d", expectedCount, len(results))
				return false
			}

			return true
		},
		gen.Identifier(),      // userID
		gen.UInt8Range(1, 10), // sessionCount
		gen.UInt8Range(1, 20), // limit
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
