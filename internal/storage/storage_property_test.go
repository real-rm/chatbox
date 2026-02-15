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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, nil)

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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, nil)

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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, nil)

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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, nil)

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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			// Create 32-byte encryption key for AES-256
			encryptionKey := []byte("12345678901234567890123456789012")
			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, encryptionKey)

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

			mongoClient, logger, cleanup := setupTestMongoDB(t)
			defer cleanup()

			service := NewStorageService(mongoClient, "test_chat_db", "prop_sessions", logger, nil)

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
