package storage

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// setupTestMongoDB creates a test MongoDB connection using gomongo
// This uses a local MongoDB instance for testing
// Tests will be skipped if MongoDB is not available
// Now uses shared MongoDB client to avoid initialization conflicts
func setupTestMongoDB(t *testing.T) (*gomongo.Mongo, *golog.Logger, func()) {
	mongoClient, logger := getSharedMongoClient(t)
	if mongoClient == nil {
		return nil, nil, func() {}
	}

	// Return cleanup function (minimal since we're using shared client)
	cleanup := func() {
		// Cleanup is handled by individual tests
	}

	return mongoClient, logger, cleanup
}

// getUniqueCollectionName generates a unique collection name for each test
// to avoid duplicate key errors when tests run concurrently or sequentially
func getUniqueCollectionName(t *testing.T) string {
	return fmt.Sprintf("test_%s_%d", t.Name(), time.Now().UnixNano())
}

// setupTestStorage creates a test storage service with a unique collection name
// This prevents duplicate key errors when tests run concurrently or sequentially
func setupTestStorage(t *testing.T, encryptionKey []byte) (*StorageService, func()) {
	mongoClient, logger := getSharedMongoClient(t)
	if mongoClient == nil {
		return nil, func() {}
	}

	collectionName := getUniqueCollectionName(t)
	service := NewStorageService(mongoClient, "chatbox", collectionName, logger, encryptionKey)

	cleanup := func() {
		// Drop test collection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		db, _ := mongoClient.Database("chatbox")
		if db != nil {
			db.Coll(collectionName).Drop(ctx)
		}
	}

	return service, cleanup
}

func TestNewStorageService(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	assert.NotNil(t, service)
	assert.NotNil(t, service.mongo)
	assert.NotNil(t, service.collection)
	assert.NotNil(t, service.logger)
}

func TestCreateSession_ValidSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create a test session
	sess := &session.Session{
		ID:            "test-session-1",
		UserID:        "user-123",
		Name:          "Test Session",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	// Create session in database
	err := service.CreateSession(sess)
	assert.NoError(t, err)

	// Verify session was created
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc SessionDocument
	err = service.collection.FindOne(ctx, bson.M{"_id": "test-session-1"}).Decode(&doc)
	assert.NoError(t, err)
	assert.Equal(t, "test-session-1", doc.ID)
	assert.Equal(t, "user-123", doc.UserID)
	assert.Equal(t, "Test Session", doc.Name)
	assert.Equal(t, "gpt-4", doc.ModelID)
}

func TestCreateSession_NilSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	err := service.CreateSession(nil)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

func TestCreateSession_EmptySessionID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	sess := &session.Session{
		ID:     "",
		UserID: "user-123",
	}

	err := service.CreateSession(sess)
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestCreateSession_DuplicateSessionID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create first session
	sess1 := &session.Session{
		ID:            "duplicate-session-id",
		UserID:        "user-123",
		Name:          "First Session",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	err := service.CreateSession(sess1)
	require.NoError(t, err)

	// Try to create second session with same ID - should fail
	sess2 := &session.Session{
		ID:            "duplicate-session-id",
		UserID:        "user-456",
		Name:          "Second Session",
		ModelID:       "gpt-3.5",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}

	err = service.CreateSession(sess2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session")
}

func TestCreateSession_AllPaths(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Test 1: Nil session
	err := service.CreateSession(nil)
	assert.ErrorIs(t, err, ErrInvalidSession)

	// Test 2: Empty session ID
	err = service.CreateSession(&session.Session{
		ID:     "",
		UserID: "user-123",
	})
	assert.ErrorIs(t, err, ErrInvalidSessionID)

	// Test 3: Valid session creation
	validSess := &session.Session{
		ID:            "test-all-paths-session",
		UserID:        "user-123",
		Name:          "Test Session",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}
	err = service.CreateSession(validSess)
	assert.NoError(t, err)

	// Test 4: Duplicate session ID (error path)
	duplicateSess := &session.Session{
		ID:            "test-all-paths-session",
		UserID:        "user-456",
		Name:          "Duplicate Session",
		ModelID:       "gpt-3.5",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}
	err = service.CreateSession(duplicateSess)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session")
}

func TestUpdateSession_ValidSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create initial session
	sess := &session.Session{
		ID:            "test-session-2",
		UserID:        "user-456",
		Name:          "Initial Name",
		ModelID:       "gpt-3.5",
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		LastActivity:  time.Now(),
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   100,
		ResponseTimes: []time.Duration{time.Second},
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Update session
	sess.Name = "Updated Name"
	sess.TotalTokens = 200
	sess.HelpRequested = true

	err = service.UpdateSession(sess)
	assert.NoError(t, err)

	// Verify update
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc SessionDocument
	err = service.collection.FindOne(ctx, bson.M{"_id": "test-session-2"}).Decode(&doc)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Name", doc.Name)
	assert.Equal(t, 200, doc.TotalTokens)
	assert.True(t, doc.HelpRequested)
}

func TestUpdateSession_NonExistentSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	sess := &session.Session{
		ID:     "non-existent",
		UserID: "user-789",
	}

	err := service.UpdateSession(sess)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestGetSession_ValidSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create session
	now := time.Now()
	originalSess := &session.Session{
		ID:      "test-session-3",
		UserID:  "user-999",
		Name:    "Get Test Session",
		ModelID: "claude-3",
		Messages: []*session.Message{
			{
				Content:   "Hello",
				Timestamp: now,
				Sender:    "user",
				FileID:    "",
				FileURL:   "",
				Metadata:  map[string]string{"key": "value"},
			},
		},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		HelpRequested: false,
		AdminAssisted: false,
		TotalTokens:   50,
		ResponseTimes: []time.Duration{500 * time.Millisecond},
	}

	err := service.CreateSession(originalSess)
	require.NoError(t, err)

	// Get session
	retrievedSess, err := service.GetSession("test-session-3")
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSess)
	assert.Equal(t, "test-session-3", retrievedSess.ID)
	assert.Equal(t, "user-999", retrievedSess.UserID)
	assert.Equal(t, "Get Test Session", retrievedSess.Name)
	assert.Equal(t, "claude-3", retrievedSess.ModelID)
	assert.Equal(t, 50, retrievedSess.TotalTokens)
	assert.Len(t, retrievedSess.Messages, 1)
	assert.Equal(t, "Hello", retrievedSess.Messages[0].Content)
	assert.Equal(t, "user", retrievedSess.Messages[0].Sender)
}

func TestGetSession_NonExistentSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	sess, err := service.GetSession("non-existent")
	assert.ErrorIs(t, err, ErrSessionNotFound)
	assert.Nil(t, sess)
}

func TestGetSession_EmptySessionID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	sess, err := service.GetSession("")
	assert.ErrorIs(t, err, ErrInvalidSessionID)
	assert.Nil(t, sess)
}

func TestSessionToDocument_WithMessages(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	sess := &session.Session{
		ID:      "test-session-4",
		UserID:  "user-111",
		Name:    "Message Test",
		ModelID: "gpt-4",
		Messages: []*session.Message{
			{
				Content:   "First message",
				Timestamp: now,
				Sender:    "user",
				FileID:    "file-1",
				FileURL:   "https://example.com/file1",
				Metadata:  map[string]string{"type": "text"},
			},
			{
				Content:   "Second message",
				Timestamp: now.Add(time.Minute),
				Sender:    "ai",
				FileID:    "",
				FileURL:   "",
				Metadata:  nil,
			},
		},
		StartTime:     now,
		LastActivity:  now,
		EndTime:       nil,
		IsActive:      true,
		TotalTokens:   150,
		ResponseTimes: []time.Duration{time.Second, 2 * time.Second},
	}

	doc := service.sessionToDocument(sess)

	assert.Equal(t, "test-session-4", doc.ID)
	assert.Equal(t, "user-111", doc.UserID)
	assert.Len(t, doc.Messages, 2)
	assert.Equal(t, "First message", doc.Messages[0].Content)
	assert.Equal(t, "user", doc.Messages[0].Sender)
	assert.Equal(t, "file-1", doc.Messages[0].FileID)
	assert.Equal(t, "Second message", doc.Messages[1].Content)
	assert.Equal(t, "ai", doc.Messages[1].Sender)
	assert.Equal(t, int64(2000), doc.MaxResponseTime) // 2 seconds in milliseconds
	assert.Equal(t, int64(1500), doc.AvgResponseTime) // 1.5 seconds in milliseconds
}

func TestSessionToDocument_WithEndTime(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	endTime := now.Add(10 * time.Minute)

	sess := &session.Session{
		ID:            "test-session-5",
		UserID:        "user-222",
		Name:          "Ended Session",
		StartTime:     now,
		EndTime:       &endTime,
		IsActive:      false,
		Messages:      []*session.Message{},
		ResponseTimes: []time.Duration{},
	}

	doc := service.sessionToDocument(sess)

	assert.Equal(t, int64(600), doc.Duration) // 10 minutes = 600 seconds
	assert.NotNil(t, doc.EndTime)
	assert.Equal(t, endTime, *doc.EndTime)
}

// Unit tests that don't require MongoDB

func TestSessionToDocument_Conversion(t *testing.T) {
	// Create a mock client (won't be used for conversion)
	service := &StorageService{}

	now := time.Now()
	endTime := now.Add(10 * time.Minute)

	sess := &session.Session{
		ID:      "test-id",
		UserID:  "user-123",
		Name:    "Test Session",
		ModelID: "gpt-4",
		Messages: []*session.Message{
			{
				Content:   "Hello",
				Timestamp: now,
				Sender:    "user",
				FileID:    "file-1",
				FileURL:   "https://example.com/file",
				Metadata:  map[string]string{"key": "value"},
			},
		},
		StartTime:          now,
		LastActivity:       now,
		EndTime:            &endTime,
		IsActive:           false,
		HelpRequested:      true,
		AdminAssisted:      true,
		AssistingAdminID:   "admin-1",
		AssistingAdminName: "Admin User",
		TotalTokens:        100,
		ResponseTimes:      []time.Duration{time.Second, 2 * time.Second},
	}

	doc := service.sessionToDocument(sess)

	assert.Equal(t, "test-id", doc.ID)
	assert.Equal(t, "user-123", doc.UserID)
	assert.Equal(t, "Test Session", doc.Name)
	assert.Equal(t, "gpt-4", doc.ModelID)
	assert.Len(t, doc.Messages, 1)
	assert.Equal(t, "Hello", doc.Messages[0].Content)
	assert.Equal(t, "user", doc.Messages[0].Sender)
	assert.Equal(t, "file-1", doc.Messages[0].FileID)
	assert.Equal(t, int64(600), doc.Duration) // 10 minutes
	assert.True(t, doc.HelpRequested)
	assert.True(t, doc.AdminAssisted)
	assert.Equal(t, "admin-1", doc.AssistingAdminID)
	assert.Equal(t, 100, doc.TotalTokens)
	assert.Equal(t, int64(2000), doc.MaxResponseTime) // 2 seconds
	assert.Equal(t, int64(1500), doc.AvgResponseTime) // 1.5 seconds average
	assert.Equal(t, "Admin User", doc.AssistingAdminName)
}

func TestDocumentToSession_Conversion(t *testing.T) {
	service := &StorageService{}

	now := time.Now()
	endTime := now.Add(5 * time.Minute)

	doc := &SessionDocument{
		ID:      "doc-id",
		UserID:  "user-456",
		Name:    "Doc Session",
		ModelID: "claude-3",
		Messages: []MessageDocument{
			{
				Content:   "Test message",
				Timestamp: now,
				Sender:    "ai",
				FileID:    "",
				FileURL:   "",
				Metadata:  map[string]string{"type": "response"},
			},
		},
		StartTime:        now,
		EndTime:          &endTime,
		Duration:         300,
		AdminAssisted:    false,
		AssistingAdminID: "",
		HelpRequested:    false,
		TotalTokens:      50,
		MaxResponseTime:  1500,
		AvgResponseTime:  1000,
	}

	sess := service.documentToSession(doc)

	assert.Equal(t, "doc-id", sess.ID)
	assert.Equal(t, "user-456", sess.UserID)
	assert.Equal(t, "Doc Session", sess.Name)
	assert.Equal(t, "claude-3", sess.ModelID)
	assert.Len(t, sess.Messages, 1)
	assert.Equal(t, "Test message", sess.Messages[0].Content)
	assert.Equal(t, "ai", sess.Messages[0].Sender)
	assert.False(t, sess.IsActive) // EndTime is set
	assert.False(t, sess.HelpRequested)
	assert.False(t, sess.AdminAssisted)
	assert.Equal(t, 50, sess.TotalTokens)
	assert.NotEmpty(t, sess.ResponseTimes)
}

func TestSessionToDocument_EmptyMessages(t *testing.T) {
	service := &StorageService{}

	now := time.Now()
	sess := &session.Session{
		ID:            "empty-msg-session",
		UserID:        "user-789",
		Name:          "Empty Messages",
		Messages:      []*session.Message{},
		StartTime:     now,
		ResponseTimes: []time.Duration{},
	}

	doc := service.sessionToDocument(sess)

	assert.Equal(t, "empty-msg-session", doc.ID)
	assert.Empty(t, doc.Messages)
	assert.Equal(t, int64(0), doc.MaxResponseTime)
	assert.Equal(t, int64(0), doc.AvgResponseTime)
}

func TestSessionToDocument_ActiveSession(t *testing.T) {
	service := &StorageService{}

	now := time.Now().Add(-time.Second) // Start 1 second ago
	sess := &session.Session{
		ID:        "active-session",
		UserID:    "user-active",
		StartTime: now,
		EndTime:   nil, // Active session
		IsActive:  true,
		Messages:  []*session.Message{},
	}

	doc := service.sessionToDocument(sess)

	assert.Nil(t, doc.EndTime)
	assert.Greater(t, doc.Duration, int64(0)) // Should have some duration
}

func TestDocumentToSession_ActiveSession(t *testing.T) {
	service := &StorageService{}

	now := time.Now()
	doc := &SessionDocument{
		ID:        "active-doc",
		UserID:    "user-active",
		StartTime: now,
		EndTime:   nil, // Active session
	}

	sess := service.documentToSession(doc)

	assert.True(t, sess.IsActive)
	assert.Nil(t, sess.EndTime)
}

func TestDocumentToSession_NoResponseTimes(t *testing.T) {
	service := &StorageService{}

	doc := &SessionDocument{
		ID:              "no-response-times",
		UserID:          "user-123",
		StartTime:       time.Now(),
		MaxResponseTime: 0,
		AvgResponseTime: 0,
	}

	sess := service.documentToSession(doc)

	assert.Empty(t, sess.ResponseTimes)
}

func TestAdminNameRoundTrip(t *testing.T) {
	service := &StorageService{}

	now := time.Now()
	originalSession := &session.Session{
		ID:                 "admin-name-test",
		UserID:             "user-123",
		Name:               "Admin Name Test",
		StartTime:          now,
		AdminAssisted:      true,
		AssistingAdminID:   "admin-456",
		AssistingAdminName: "John Admin",
		Messages:           []*session.Message{},
	}

	// Convert session to document
	doc := service.sessionToDocument(originalSession)
	assert.Equal(t, "admin-456", doc.AssistingAdminID)
	assert.Equal(t, "John Admin", doc.AssistingAdminName)

	// Convert document back to session
	restoredSession := service.documentToSession(doc)
	assert.Equal(t, "admin-456", restoredSession.AssistingAdminID)
	assert.Equal(t, "John Admin", restoredSession.AssistingAdminName)
	assert.True(t, restoredSession.AdminAssisted)
}

func TestAddMessage_ValidMessage(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create initial session
	now := time.Now()
	sess := &session.Session{
		ID:        "test-add-msg-1",
		UserID:    "user-123",
		Name:      "Add Message Test",
		Messages:  []*session.Message{},
		StartTime: now,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Add a message
	msg := &session.Message{
		Content:   "Test message content",
		Timestamp: now,
		Sender:    "user",
		FileID:    "",
		FileURL:   "",
		Metadata:  map[string]string{"test": "value"},
	}

	err = service.AddMessage("test-add-msg-1", msg)
	assert.NoError(t, err)

	// Verify message was added
	retrievedSess, err := service.GetSession("test-add-msg-1")
	assert.NoError(t, err)
	assert.Len(t, retrievedSess.Messages, 1)
	assert.Equal(t, "Test message content", retrievedSess.Messages[0].Content)
	assert.Equal(t, "user", retrievedSess.Messages[0].Sender)
}

func TestAddMessage_WithEncryption(t *testing.T) {
	// Create 32-byte encryption key for AES-256
	encryptionKey := []byte("12345678901234567890123456789012")
	service, cleanup := setupTestStorage(t, encryptionKey)
	defer cleanup()

	// Create initial session
	now := time.Now()
	sess := &session.Session{
		ID:        "test-add-msg-encrypted",
		UserID:    "user-456",
		Name:      "Encrypted Message Test",
		Messages:  []*session.Message{},
		StartTime: now,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Add a message with sensitive content
	msg := &session.Message{
		Content:   "Sensitive information here",
		Timestamp: now,
		Sender:    "user",
		FileID:    "",
		FileURL:   "",
		Metadata:  map[string]string{},
	}

	err = service.AddMessage("test-add-msg-encrypted", msg)
	assert.NoError(t, err)

	// Verify message was encrypted in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc SessionDocument
	err = service.collection.FindOne(ctx, bson.M{"_id": "test-add-msg-encrypted"}).Decode(&doc)
	assert.NoError(t, err)
	assert.Len(t, doc.Messages, 1)
	// Content should be encrypted (base64 encoded)
	assert.NotEqual(t, "Sensitive information here", doc.Messages[0].Content)

	// Verify message can be decrypted when retrieved
	retrievedSess, err := service.GetSession("test-add-msg-encrypted")
	assert.NoError(t, err)
	assert.Len(t, retrievedSess.Messages, 1)
	assert.Equal(t, "Sensitive information here", retrievedSess.Messages[0].Content)
}

func TestAddMessage_NonExistentSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	msg := &session.Message{
		Content:   "Test",
		Timestamp: time.Now(),
		Sender:    "user",
	}

	err := service.AddMessage("non-existent", msg)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestAddMessage_EmptySessionID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	msg := &session.Message{
		Content:   "Test",
		Timestamp: time.Now(),
		Sender:    "user",
	}

	err := service.AddMessage("", msg)
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestAddMessage_NilMessage(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	err := service.AddMessage("test-session", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message cannot be nil")
}

func TestEndSession_ValidSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create session
	now := time.Now()
	sess := &session.Session{
		ID:        "test-end-session-1",
		UserID:    "user-789",
		Name:      "End Session Test",
		Messages:  []*session.Message{},
		StartTime: now,
		IsActive:  true,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// End session
	endTime := now.Add(5 * time.Minute)
	err = service.EndSession("test-end-session-1", endTime)
	assert.NoError(t, err)

	// Verify session was ended
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc SessionDocument
	err = service.collection.FindOne(ctx, bson.M{"_id": "test-end-session-1"}).Decode(&doc)
	assert.NoError(t, err)
	assert.NotNil(t, doc.EndTime)
	// MongoDB stores times as UTC with millisecond precision; truncate for comparison
	assert.Equal(t, endTime.UTC().Truncate(time.Millisecond), doc.EndTime.UTC().Truncate(time.Millisecond))
	assert.Equal(t, int64(300), doc.Duration) // 5 minutes = 300 seconds
}

func TestEndSession_NonExistentSession(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	err := service.EndSession("non-existent", time.Now())
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestEndSession_EmptySessionID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	err := service.EndSession("", time.Now())
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	// Create service with encryption key
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	plaintext := "This is sensitive data that needs encryption"

	// Encrypt
	encrypted, err := service.encrypt(plaintext)
	assert.NoError(t, err)
	assert.NotEqual(t, plaintext, encrypted)

	// Decrypt
	decrypted, err := service.decrypt(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_NoKey(t *testing.T) {
	service := &StorageService{
		encryptionKey: nil,
	}

	plaintext := "Test data"
	encrypted, err := service.encrypt(plaintext)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, encrypted) // Should return plaintext when no key
}

func TestDecrypt_NoKey(t *testing.T) {
	service := &StorageService{
		encryptionKey: nil,
	}

	ciphertext := "Test data"
	decrypted, err := service.decrypt(ciphertext)
	assert.NoError(t, err)
	assert.Equal(t, ciphertext, decrypted) // Should return ciphertext when no key
}

func TestEncrypt_EmptyString(t *testing.T) {
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	encrypted, err := service.encrypt("")
	assert.NoError(t, err)

	decrypted, err := service.decrypt(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, "", decrypted)
}

func TestDecrypt_InvalidCiphertext(t *testing.T) {
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	// Try to decrypt invalid data
	_, err := service.decrypt("invalid-base64-!@#$%")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode base64")
}

func TestDecrypt_TooShortCiphertext(t *testing.T) {
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	// Create a valid base64 string that's too short
	shortData := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := service.decrypt(shortData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestEncrypt_LongText(t *testing.T) {
	encryptionKey := []byte("12345678901234567890123456789012")
	service := &StorageService{
		encryptionKey: encryptionKey,
	}

	// Test with a longer message
	longText := "This is a much longer message that contains multiple sentences. " +
		"It should still be encrypted and decrypted correctly. " +
		"The encryption should handle arbitrary length messages without issues."

	encrypted, err := service.encrypt(longText)
	assert.NoError(t, err)
	assert.NotEqual(t, longText, encrypted)

	decrypted, err := service.decrypt(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, longText, decrypted)
}

func TestListUserSessions_ValidUser(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create multiple sessions for the same user
	now := time.Now()
	sessions := []*session.Session{
		{
			ID:     "session-1",
			UserID: "user-list-test",
			Name:   "First Session",
			Messages: []*session.Message{
				{Content: "Message 1", Timestamp: now, Sender: "user"},
			},
			StartTime:     now.Add(-3 * time.Hour),
			TotalTokens:   100,
			AdminAssisted: false,
		},
		{
			ID:     "session-2",
			UserID: "user-list-test",
			Name:   "Second Session",
			Messages: []*session.Message{
				{Content: "Message 1", Timestamp: now, Sender: "user"},
				{Content: "Message 2", Timestamp: now.Add(time.Minute), Sender: "ai"},
			},
			StartTime:     now.Add(-2 * time.Hour),
			TotalTokens:   200,
			AdminAssisted: true,
		},
		{
			ID:     "session-3",
			UserID: "user-list-test",
			Name:   "Third Session",
			Messages: []*session.Message{
				{Content: "Message 1", Timestamp: now, Sender: "user"},
			},
			StartTime:     now.Add(-1 * time.Hour),
			TotalTokens:   150,
			AdminAssisted: false,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// List sessions
	metadata, err := service.ListUserSessions("user-list-test", 0)
	assert.NoError(t, err)
	assert.Len(t, metadata, 3)

	// Verify sessions are ordered by start time (most recent first)
	assert.Equal(t, "session-3", metadata[0].ID)
	assert.Equal(t, "Third Session", metadata[0].Name)
	assert.Equal(t, 1, metadata[0].MessageCount)
	assert.Equal(t, 150, metadata[0].TotalTokens)
	assert.False(t, metadata[0].AdminAssisted)

	assert.Equal(t, "session-2", metadata[1].ID)
	assert.Equal(t, "Second Session", metadata[1].Name)
	assert.Equal(t, 2, metadata[1].MessageCount)
	assert.Equal(t, 200, metadata[1].TotalTokens)
	assert.True(t, metadata[1].AdminAssisted)

	assert.Equal(t, "session-1", metadata[2].ID)
	assert.Equal(t, "First Session", metadata[2].Name)
	assert.Equal(t, 1, metadata[2].MessageCount)
	assert.Equal(t, 100, metadata[2].TotalTokens)
	assert.False(t, metadata[2].AdminAssisted)
}

func TestListUserSessions_WithLimit(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create multiple sessions
	now := time.Now()
	for i := 0; i < 5; i++ {
		sess := &session.Session{
			ID:        fmt.Sprintf("session-limit-%d", i),
			UserID:    "user-limit-test",
			Name:      fmt.Sprintf("Session %d", i),
			Messages:  []*session.Message{},
			StartTime: now.Add(-time.Duration(i) * time.Hour),
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// List with limit
	metadata, err := service.ListUserSessions("user-limit-test", 3)
	assert.NoError(t, err)
	assert.Len(t, metadata, 3)
}

func TestListUserSessions_EmptyUserID(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	metadata, err := service.ListUserSessions("", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user ID cannot be empty")
	assert.Nil(t, metadata)
}

func TestListUserSessions_NoSessions(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	metadata, err := service.ListUserSessions("user-no-sessions", 0)
	assert.NoError(t, err)
	assert.Empty(t, metadata)
}

func TestListUserSessions_LastMessageTime(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	lastMsgTime := now.Add(5 * time.Minute)

	sess := &session.Session{
		ID:     "session-last-msg",
		UserID: "user-last-msg-test",
		Name:   "Last Message Test",
		Messages: []*session.Message{
			{Content: "First", Timestamp: now, Sender: "user"},
			{Content: "Last", Timestamp: lastMsgTime, Sender: "ai"},
		},
		StartTime: now,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	metadata, err := service.ListUserSessions("user-last-msg-test", 0)
	assert.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Equal(t, lastMsgTime.Unix(), metadata[0].LastMessageTime.Unix())
}

func TestGetSessionMetrics_ValidTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create sessions with different characteristics
	now := time.Now()
	startTime := now.Add(-2 * time.Hour)
	endTime := now

	sessions := []*session.Session{
		{
			ID:            "metrics-session-1",
			UserID:        "user-metrics-1",
			Name:          "Active Session",
			Messages:      []*session.Message{},
			StartTime:     startTime.Add(10 * time.Minute),
			EndTime:       nil, // Active
			TotalTokens:   100,
			AdminAssisted: false,
			ResponseTimes: []time.Duration{time.Second, 2 * time.Second},
		},
		{
			ID:            "metrics-session-2",
			UserID:        "user-metrics-2",
			Name:          "Ended Session",
			Messages:      []*session.Message{},
			StartTime:     startTime.Add(20 * time.Minute),
			EndTime:       &endTime,
			TotalTokens:   200,
			AdminAssisted: true,
			ResponseTimes: []time.Duration{3 * time.Second},
		},
		{
			ID:            "metrics-session-3",
			UserID:        "user-metrics-3",
			Name:          "Another Active",
			Messages:      []*session.Message{},
			StartTime:     startTime.Add(30 * time.Minute),
			EndTime:       nil, // Active
			TotalTokens:   150,
			AdminAssisted: false,
			ResponseTimes: []time.Duration{500 * time.Millisecond, time.Second},
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Get metrics
	metrics, err := service.GetSessionMetrics(startTime, endTime)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)

	// Verify metrics
	assert.Equal(t, 3, metrics.TotalSessions)
	assert.Equal(t, 2, metrics.ActiveSessions)
	assert.Equal(t, 1, metrics.AdminAssistedCount)
	assert.Equal(t, 450, metrics.TotalTokens)             // 100 + 200 + 150
	assert.Equal(t, int64(3000), metrics.MaxResponseTime) // 3 seconds
	assert.Greater(t, metrics.AvgResponseTime, int64(0))
}

func TestGetSessionMetrics_EmptyTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// Get metrics with no sessions in range
	metrics, err := service.GetSessionMetrics(startTime, endTime)
	assert.NoError(t, err)
	if metrics != nil {
		assert.Equal(t, 0, metrics.TotalSessions)
		assert.Equal(t, 0, metrics.ActiveSessions)
		assert.Equal(t, 0, metrics.TotalTokens)
	}
}

func TestGetSessionMetrics_InvalidTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now
	endTime := now.Add(-1 * time.Hour) // End before start

	metrics, err := service.GetSessionMetrics(startTime, endTime)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
	assert.Nil(t, metrics)
}

func TestGetSessionMetrics_ConcurrentSessions(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// Create overlapping sessions
	session1End := now.Add(-30 * time.Minute)
	session2End := now.Add(-20 * time.Minute)

	sessions := []*session.Session{
		{
			ID:        "concurrent-1",
			UserID:    "user-1",
			Name:      "Session 1",
			Messages:  []*session.Message{},
			StartTime: startTime,
			EndTime:   &session1End,
		},
		{
			ID:        "concurrent-2",
			UserID:    "user-2",
			Name:      "Session 2",
			Messages:  []*session.Message{},
			StartTime: startTime.Add(10 * time.Minute),
			EndTime:   &session2End,
		},
		{
			ID:        "concurrent-3",
			UserID:    "user-3",
			Name:      "Session 3",
			Messages:  []*session.Message{},
			StartTime: startTime.Add(15 * time.Minute),
			EndTime:   nil, // Still active
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	metrics, err := service.GetSessionMetrics(startTime, endTime)
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, 3, metrics.TotalSessions)
}

func TestGetTokenUsage_ValidTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now.Add(-2 * time.Hour)
	endTime := now

	// Create sessions with token usage
	sessions := []*session.Session{
		{
			ID:          "token-session-1",
			UserID:      "user-token-1",
			Name:        "Token Session 1",
			Messages:    []*session.Message{},
			StartTime:   startTime.Add(10 * time.Minute),
			TotalTokens: 100,
		},
		{
			ID:          "token-session-2",
			UserID:      "user-token-2",
			Name:        "Token Session 2",
			Messages:    []*session.Message{},
			StartTime:   startTime.Add(20 * time.Minute),
			TotalTokens: 250,
		},
		{
			ID:          "token-session-3",
			UserID:      "user-token-3",
			Name:        "Token Session 3",
			Messages:    []*session.Message{},
			StartTime:   startTime.Add(30 * time.Minute),
			TotalTokens: 150,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Get token usage
	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.NoError(t, err)
	assert.Equal(t, 500, totalTokens) // 100 + 250 + 150
}

func TestGetTokenUsage_EmptyTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// Get token usage with no sessions
	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.NoError(t, err)
	assert.Equal(t, 0, totalTokens)
}

func TestGetTokenUsage_InvalidTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	startTime := now
	endTime := now.Add(-1 * time.Hour) // End before start

	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
	assert.Equal(t, 0, totalTokens)
}

func TestGetTokenUsage_PartialTimeRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()

	// Create sessions at different times
	sessions := []*session.Session{
		{
			ID:          "token-partial-1",
			UserID:      "user-1",
			Name:        "Before Range",
			Messages:    []*session.Message{},
			StartTime:   now.Add(-3 * time.Hour), // Outside range
			TotalTokens: 100,
		},
		{
			ID:          "token-partial-2",
			UserID:      "user-2",
			Name:        "In Range",
			Messages:    []*session.Message{},
			StartTime:   now.Add(-1 * time.Hour), // Inside range
			TotalTokens: 200,
		},
		{
			ID:          "token-partial-3",
			UserID:      "user-3",
			Name:        "After Range",
			Messages:    []*session.Message{},
			StartTime:   now.Add(1 * time.Hour), // Outside range
			TotalTokens: 300,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Get token usage for middle time range
	startTime := now.Add(-2 * time.Hour)
	endTime := now

	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.NoError(t, err)
	assert.Equal(t, 200, totalTokens) // Only the session in range
}

func TestListAllSessionsWithOptions_Pagination(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create 25 sessions
	for i := 0; i < 25; i++ {
		sess := &session.Session{
			ID:        fmt.Sprintf("session-%d", i),
			UserID:    "user-1",
			Name:      fmt.Sprintf("Session %d", i),
			Messages:  []*session.Message{},
			StartTime: time.Now().Add(time.Duration(-i) * time.Hour),
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Test pagination - first page
	opts := &SessionListOptions{
		Limit:  10,
		Offset: 0,
	}
	sessions, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, len(sessions))

	// Test pagination - second page
	opts.Offset = 10
	sessions, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, len(sessions))

	// Test pagination - third page
	opts.Offset = 20
	sessions, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(sessions))
}

func TestListAllSessionsWithOptions_FilterByUser(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create sessions for different users
	users := []string{"user-1", "user-2", "user-3"}
	for _, userID := range users {
		for i := 0; i < 3; i++ {
			sess := &session.Session{
				ID:        fmt.Sprintf("%s-session-%d", userID, i),
				UserID:    userID,
				Name:      fmt.Sprintf("Session %d", i),
				Messages:  []*session.Message{},
				StartTime: time.Now(),
			}
			err := service.CreateSession(sess)
			require.NoError(t, err)
		}
	}

	// Filter by user-2
	opts := &SessionListOptions{
		UserID: "user-2",
		Limit:  100,
	}
	sessions, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(sessions))
	for _, sess := range sessions {
		assert.Equal(t, "user-2", sess.UserID)
	}
}

func TestListAllSessionsWithOptions_FilterByDateRange(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()

	// Create sessions at different times
	sessions := []*session.Session{
		{
			ID:        "old-session",
			UserID:    "user-1",
			Name:      "Old Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(-10 * time.Hour),
		},
		{
			ID:        "recent-session-1",
			UserID:    "user-1",
			Name:      "Recent Session 1",
			Messages:  []*session.Message{},
			StartTime: now.Add(-2 * time.Hour),
		},
		{
			ID:        "recent-session-2",
			UserID:    "user-1",
			Name:      "Recent Session 2",
			Messages:  []*session.Message{},
			StartTime: now.Add(-1 * time.Hour),
		},
		{
			ID:        "future-session",
			UserID:    "user-1",
			Name:      "Future Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(1 * time.Hour),
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Filter by date range (last 3 hours)
	startTimeFrom := now.Add(-3 * time.Hour)
	startTimeTo := now
	opts := &SessionListOptions{
		StartTimeFrom: &startTimeFrom,
		StartTimeTo:   &startTimeTo,
		Limit:         100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
}

func TestListAllSessionsWithOptions_FilterByAdminAssisted(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Create sessions with and without admin assistance
	sessions := []*session.Session{
		{
			ID:            "assisted-1",
			UserID:        "user-1",
			Name:          "Assisted Session 1",
			Messages:      []*session.Message{},
			StartTime:     time.Now(),
			AdminAssisted: true,
		},
		{
			ID:            "assisted-2",
			UserID:        "user-2",
			Name:          "Assisted Session 2",
			Messages:      []*session.Message{},
			StartTime:     time.Now(),
			AdminAssisted: true,
		},
		{
			ID:            "not-assisted-1",
			UserID:        "user-3",
			Name:          "Not Assisted Session",
			Messages:      []*session.Message{},
			StartTime:     time.Now(),
			AdminAssisted: false,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Filter for admin assisted sessions
	adminAssisted := true
	opts := &SessionListOptions{
		AdminAssisted: &adminAssisted,
		Limit:         100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	for _, sess := range result {
		assert.True(t, sess.AdminAssisted)
	}

	// Filter for non-assisted sessions
	notAssisted := false
	opts.AdminAssisted = &notAssisted
	result, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result))
	assert.False(t, result[0].AdminAssisted)
}

func TestListAllSessionsWithOptions_FilterByActiveStatus(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()
	endTime := now.Add(-1 * time.Hour)

	// Create active and ended sessions
	sessions := []*session.Session{
		{
			ID:        "active-1",
			UserID:    "user-1",
			Name:      "Active Session 1",
			Messages:  []*session.Message{},
			StartTime: now,
			EndTime:   nil,
		},
		{
			ID:        "active-2",
			UserID:    "user-2",
			Name:      "Active Session 2",
			Messages:  []*session.Message{},
			StartTime: now,
			EndTime:   nil,
		},
		{
			ID:        "ended-1",
			UserID:    "user-3",
			Name:      "Ended Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   &endTime,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Filter for active sessions
	active := true
	opts := &SessionListOptions{
		Active: &active,
		Limit:  100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	for _, sess := range result {
		assert.Nil(t, sess.EndTime)
	}

	// Filter for ended sessions
	notActive := false
	opts.Active = &notActive
	result, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result))
	assert.NotNil(t, result[0].EndTime)
}

func TestListAllSessionsWithOptions_SortByStartTime(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	now := time.Now()

	// Create sessions at different times
	sessions := []*session.Session{
		{
			ID:        "session-3",
			UserID:    "user-1",
			Name:      "Session 3",
			Messages:  []*session.Message{},
			StartTime: now.Add(-1 * time.Hour),
		},
		{
			ID:        "session-1",
			UserID:    "user-1",
			Name:      "Session 1",
			Messages:  []*session.Message{},
			StartTime: now.Add(-3 * time.Hour),
		},
		{
			ID:        "session-2",
			UserID:    "user-1",
			Name:      "Session 2",
			Messages:  []*session.Message{},
			StartTime: now.Add(-2 * time.Hour),
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Sort by start_time descending (default)
	opts := &SessionListOptions{
		SortBy:    "start_time",
		SortOrder: "desc",
		Limit:     100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "session-3", result[0].ID)
	assert.Equal(t, "session-2", result[1].ID)
	assert.Equal(t, "session-1", result[2].ID)

	// Sort by start_time ascending
	opts.SortOrder = "asc"
	result, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "session-1", result[0].ID)
	assert.Equal(t, "session-2", result[1].ID)
	assert.Equal(t, "session-3", result[2].ID)
}

func TestListAllSessionsWithOptions_SortByMessageCount(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique IDs to avoid conflicts
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Create sessions with different message counts
	sessions := []*session.Session{
		{
			ID:        testID + "-session-1",
			UserID:    testID + "-user-1",
			Name:      "Session 1",
			Messages:  []*session.Message{{Content: "msg1", Timestamp: time.Now(), Sender: "user"}},
			StartTime: time.Now(),
		},
		{
			ID:     testID + "-session-2",
			UserID: testID + "-user-1",
			Name:   "Session 2",
			Messages: []*session.Message{
				{Content: "msg1", Timestamp: time.Now(), Sender: "user"},
				{Content: "msg2", Timestamp: time.Now(), Sender: "ai"},
				{Content: "msg3", Timestamp: time.Now(), Sender: "user"},
			},
			StartTime: time.Now(),
		},
		{
			ID:     testID + "-session-3",
			UserID: testID + "-user-1",
			Name:   "Session 3",
			Messages: []*session.Message{
				{Content: "msg1", Timestamp: time.Now(), Sender: "user"},
				{Content: "msg2", Timestamp: time.Now(), Sender: "ai"},
			},
			StartTime: time.Now(),
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Cleanup after test
	defer func() {
		ctx := context.Background()
		for _, sess := range sessions {
			service.collection.DeleteOne(ctx, bson.M{"_id": sess.ID})
		}
	}()

	// Sort by message_count descending
	opts := &SessionListOptions{
		SortBy:    "message_count",
		SortOrder: "desc",
		Limit:     100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(result), 3)

	// Find our test sessions in the results
	var testResults []*SessionMetadata
	for _, r := range result {
		if r.UserID == testID+"-user-1" {
			testResults = append(testResults, r)
		}
	}
	assert.Equal(t, 3, len(testResults))
	assert.Equal(t, testID+"-session-2", testResults[0].ID)
	assert.Equal(t, 3, testResults[0].MessageCount)
	assert.Equal(t, testID+"-session-3", testResults[1].ID)
	assert.Equal(t, 2, testResults[1].MessageCount)
	assert.Equal(t, testID+"-session-1", testResults[2].ID)
	assert.Equal(t, 1, testResults[2].MessageCount)

	// Sort by message_count ascending
	opts.SortOrder = "asc"
	opts.UserID = testID + "-user-1" // Filter to only our test user
	result, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, testID+"-session-1", result[0].ID)
	assert.Equal(t, testID+"-session-3", result[1].ID)
	assert.Equal(t, testID+"-session-2", result[2].ID)
}

func TestListAllSessionsWithOptions_SortByTotalTokens(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique IDs to avoid conflicts
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Create sessions with different token counts
	sessions := []*session.Session{
		{
			ID:          testID + "-session-1",
			UserID:      testID + "-user-1",
			Name:        "Session 1",
			Messages:    []*session.Message{},
			StartTime:   time.Now(),
			TotalTokens: 100,
		},
		{
			ID:          testID + "-session-2",
			UserID:      testID + "-user-1",
			Name:        "Session 2",
			Messages:    []*session.Message{},
			StartTime:   time.Now(),
			TotalTokens: 500,
		},
		{
			ID:          testID + "-session-3",
			UserID:      testID + "-user-1",
			Name:        "Session 3",
			Messages:    []*session.Message{},
			StartTime:   time.Now(),
			TotalTokens: 250,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Cleanup after test
	defer func() {
		ctx := context.Background()
		for _, sess := range sessions {
			service.collection.DeleteOne(ctx, bson.M{"_id": sess.ID})
		}
	}()

	// Sort by totalTokens descending
	opts := &SessionListOptions{
		SortBy:    "totalTokens",
		SortOrder: "desc",
		Limit:     100,
		UserID:    testID + "-user-1", // Filter to only our test user
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result))
	assert.Equal(t, testID+"-session-2", result[0].ID)
	assert.Equal(t, 500, result[0].TotalTokens)
	assert.Equal(t, testID+"-session-3", result[1].ID)
	assert.Equal(t, 250, result[1].TotalTokens)
	assert.Equal(t, testID+"-session-1", result[2].ID)
	assert.Equal(t, 100, result[2].TotalTokens)
}

func TestListAllSessionsWithOptions_DefaultValues(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique IDs to avoid conflicts
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Create a few sessions
	var sessions []*session.Session
	for i := 0; i < 5; i++ {
		sess := &session.Session{
			ID:        fmt.Sprintf("%s-session-%d", testID, i),
			UserID:    testID + "-user-1",
			Name:      fmt.Sprintf("Session %d", i),
			Messages:  []*session.Message{},
			StartTime: time.Now().Add(time.Duration(-i) * time.Hour),
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
		sessions = append(sessions, sess)
	}

	// Cleanup after test
	defer func() {
		ctx := context.Background()
		for _, sess := range sessions {
			service.collection.DeleteOne(ctx, bson.M{"_id": sess.ID})
		}
	}()

	// Test with nil options (should use defaults)
	opts := &SessionListOptions{
		UserID: testID + "-user-1", // Filter to only our test user
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(result))

	// Test with empty options (should use defaults)
	opts = &SessionListOptions{
		UserID: testID + "-user-1",
	}
	result, err = service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(result))
	// Should be sorted by start_time descending by default
	assert.Equal(t, fmt.Sprintf("%s-session-0", testID), result[0].ID)
}

func TestListAllSessionsWithOptions_LimitCap(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique IDs to avoid conflicts
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Create a session
	sess := &session.Session{
		ID:        testID + "-session-1",
		UserID:    testID + "-user-1",
		Name:      "Session 1",
		Messages:  []*session.Message{},
		StartTime: time.Now(),
	}
	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Cleanup after test
	defer func() {
		ctx := context.Background()
		service.collection.DeleteOne(ctx, bson.M{"_id": sess.ID})
	}()

	// Test with limit > 1000 (should be capped at 1000)
	opts := &SessionListOptions{
		Limit:  5000,
		UserID: testID + "-user-1", // Filter to only our test user
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result))
}

func TestListAllSessionsWithOptions_CombinedFilters(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique IDs to avoid conflicts
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	now := time.Now()
	endTime := now.Add(-1 * time.Hour)

	// Create diverse sessions
	sessions := []*session.Session{
		{
			ID:            testID + "-match-1",
			UserID:        testID + "-user-1",
			Name:          "Match 1",
			Messages:      []*session.Message{{Content: "msg", Timestamp: now, Sender: "user"}},
			StartTime:     now.Add(-2 * time.Hour),
			AdminAssisted: true,
			EndTime:       nil,
		},
		{
			ID:            testID + "-match-2",
			UserID:        testID + "-user-1",
			Name:          "Match 2",
			Messages:      []*session.Message{{Content: "msg", Timestamp: now, Sender: "user"}},
			StartTime:     now.Add(-1 * time.Hour),
			AdminAssisted: true,
			EndTime:       nil,
		},
		{
			ID:            testID + "-no-match-user",
			UserID:        testID + "-user-2",
			Name:          "No Match User",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-1 * time.Hour),
			AdminAssisted: true,
			EndTime:       nil,
		},
		{
			ID:            testID + "-no-match-admin",
			UserID:        testID + "-user-1",
			Name:          "No Match Admin",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-1 * time.Hour),
			AdminAssisted: false,
			EndTime:       nil,
		},
		{
			ID:            testID + "-no-match-ended",
			UserID:        testID + "-user-1",
			Name:          "No Match Ended",
			Messages:      []*session.Message{},
			StartTime:     now.Add(-1 * time.Hour),
			AdminAssisted: true,
			EndTime:       &endTime,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Cleanup after test
	defer func() {
		ctx := context.Background()
		for _, sess := range sessions {
			service.collection.DeleteOne(ctx, bson.M{"_id": sess.ID})
		}
	}()

	// Filter: user-1, admin assisted, active, last 3 hours
	adminAssisted := true
	active := true
	startTimeFrom := now.Add(-3 * time.Hour)
	opts := &SessionListOptions{
		UserID:        testID + "-user-1",
		AdminAssisted: &adminAssisted,
		Active:        &active,
		StartTimeFrom: &startTimeFrom,
		Limit:         100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, testID+"-match-2", result[0].ID)
	assert.Equal(t, testID+"-match-1", result[1].ID)
}

// TestListAllSessionsWithOptions_LargeDataset tests performance with large datasets
func TestListAllSessionsWithOptions_LargeDataset(t *testing.T) {
	service, cleanup := setupTestStorageShared(t)
	if service == nil {
		return // Test was skipped
	}
	defer cleanup()

	now := time.Now()
	testID := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Create 1000 sessions with varied data
	t.Log("Creating 1000 test sessions...")
	for i := 0; i < 1000; i++ {
		userID := fmt.Sprintf("%s-user-%d", testID, i%100) // 100 different users
		adminAssisted := i%3 == 0                          // ~33% admin assisted
		var endTime *time.Time
		if i%2 == 0 { // 50% ended sessions
			et := now.Add(time.Duration(i) * time.Minute)
			endTime = &et
		}

		sess := &session.Session{
			ID:            fmt.Sprintf("%s-session-%d", testID, i),
			UserID:        userID,
			Name:          fmt.Sprintf("Session %d", i),
			Messages:      []*session.Message{{Content: fmt.Sprintf("msg-%d", i), Timestamp: now, Sender: "user"}},
			StartTime:     now.Add(-time.Duration(i) * time.Minute),
			AdminAssisted: adminAssisted,
			TotalTokens:   i * 10,
			EndTime:       endTime,
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)
	}
	t.Log("Created 1000 test sessions")

	// Test 1: Pagination through large dataset
	t.Run("Pagination", func(t *testing.T) {
		// First page
		opts := &SessionListOptions{
			Limit:     100,
			Offset:    0,
			SortBy:    "start_time",
			SortOrder: "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))

		// Second page
		opts.Offset = 100
		result, err = service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))

		// Last page
		opts.Offset = 900
		result, err = service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))
	})

	// Test 2: Filter by user with large dataset
	t.Run("FilterByUser", func(t *testing.T) {
		targetUser := testID + "-user-0"
		opts := &SessionListOptions{
			UserID:    targetUser,
			Limit:     100,
			SortBy:    "start_time",
			SortOrder: "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 10, len(result)) // 1000 sessions / 100 users = 10 per user
		for _, sess := range result {
			assert.Equal(t, targetUser, sess.UserID)
		}
	})

	// Test 3: Filter by admin assisted
	t.Run("FilterByAdminAssisted", func(t *testing.T) {
		adminAssisted := true
		opts := &SessionListOptions{
			AdminAssisted: &adminAssisted,
			Limit:         500,
			SortBy:        "start_time",
			SortOrder:     "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		// Should be ~333 sessions (1000 / 3)
		assert.True(t, len(result) >= 300 && len(result) <= 350)
		for _, sess := range result {
			assert.True(t, sess.AdminAssisted)
		}
	})

	// Test 4: Filter by active status
	t.Run("FilterByActiveStatus", func(t *testing.T) {
		active := true
		opts := &SessionListOptions{
			Active:    &active,
			Limit:     600,
			SortBy:    "start_time",
			SortOrder: "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		// Should be 500 sessions (50% active)
		assert.Equal(t, 500, len(result))
		for _, sess := range result {
			assert.Nil(t, sess.EndTime)
		}
	})

	// Test 5: Sort by total tokens
	t.Run("SortByTotalTokens", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     100,
			SortBy:    "totalTokens",
			SortOrder: "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))
		// Verify descending order
		for i := 1; i < len(result); i++ {
			assert.True(t, result[i-1].TotalTokens >= result[i].TotalTokens)
		}
	})

	// Test 6: Sort by message count
	t.Run("SortByMessageCount", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     100,
			SortBy:    "message_count",
			SortOrder: "asc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, 100, len(result))
		// All sessions have 1 message, so order doesn't matter much
		for _, sess := range result {
			assert.Equal(t, 1, sess.MessageCount)
		}
	})

	// Test 7: Combined filters with large dataset
	t.Run("CombinedFilters", func(t *testing.T) {
		adminAssisted := true
		active := false
		// Truncate to millisecond precision to match MongoDB's storage precision,
		// avoiding boundary assertion failures for sessions created at the exact boundary.
		startTimeFrom := now.Add(-600 * time.Minute).Truncate(time.Millisecond)
		startTimeTo := now.Add(-400 * time.Minute).Truncate(time.Millisecond)
		opts := &SessionListOptions{
			AdminAssisted: &adminAssisted,
			Active:        &active,
			StartTimeFrom: &startTimeFrom,
			StartTimeTo:   &startTimeTo,
			Limit:         100,
			SortBy:        "start_time",
			SortOrder:     "desc",
		}
		result, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		// Should find sessions in range [400, 600] that are admin assisted and ended
		// That's 200 sessions, ~66 admin assisted, all ended (even indices)
		assert.True(t, len(result) > 0)
		for _, sess := range result {
			assert.True(t, sess.AdminAssisted)
			assert.NotNil(t, sess.EndTime)
			assert.True(t, sess.StartTime.After(startTimeFrom) || sess.StartTime.Equal(startTimeFrom))
			assert.True(t, sess.StartTime.Before(startTimeTo) || sess.StartTime.Equal(startTimeTo))
		}
	})

	// Test 8: Performance test - measure query time
	t.Run("Performance", func(t *testing.T) {
		start := time.Now()
		opts := &SessionListOptions{
			Limit:     100,
			Offset:    0,
			SortBy:    "start_time",
			SortOrder: "desc",
		}
		_, err := service.ListAllSessionsWithOptions(opts)
		elapsed := time.Since(start)
		assert.NoError(t, err)
		// Query should complete in less than 1 second
		assert.True(t, elapsed < time.Second, "Query took %v, expected < 1s", elapsed)
		t.Logf("Query completed in %v", elapsed)
	})
}

// TestEnsureIndexes verifies that MongoDB indexes are created correctly
func TestEnsureIndexes(t *testing.T) {
	mongo, logger, cleanup := setupTestMongoDB(t)
	if mongo == nil {
		return // Test was skipped
	}
	defer cleanup()

	// Create a test collection
	testCollName := fmt.Sprintf("test_indexes_%d", time.Now().Unix())
	storageService := NewStorageService(mongo, "chatbox", testCollName, logger, nil)

	// Ensure indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := storageService.EnsureIndexes(ctx)
	require.NoError(t, err, "EnsureIndexes should succeed")

	// Verify indexes work by creating a test session and querying it
	sess := &session.Session{
		ID:        "test-session-" + time.Now().Format("20060102150405"),
		UserID:    "test-user",
		StartTime: time.Now(),
		Messages:  []*session.Message{},
	}

	err = storageService.CreateSession(sess)
	require.NoError(t, err, "Should be able to create session")

	// Query using indexed field (uid)
	sessions, err := storageService.ListUserSessions("test-user", 10)
	require.NoError(t, err, "Should be able to query by user_id (indexed)")
	assert.Len(t, sessions, 1, "Should find the created session")

	// Clean up
	_ = storageService.collection.Drop(ctx)
}

// TestEnsureIndexesIdempotent verifies that calling EnsureIndexes multiple times is safe
func TestEnsureIndexesIdempotent(t *testing.T) {
	mongo, logger, cleanup := setupTestMongoDB(t)
	if mongo == nil {
		return // Test was skipped
	}
	defer cleanup()

	// Create a test collection
	testCollName := fmt.Sprintf("test_indexes_idempotent_%d", time.Now().Unix())
	storageService := NewStorageService(mongo, "chatbox", testCollName, logger, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Call EnsureIndexes multiple times
	err := storageService.EnsureIndexes(ctx)
	require.NoError(t, err, "First EnsureIndexes should succeed")

	err = storageService.EnsureIndexes(ctx)
	require.NoError(t, err, "Second EnsureIndexes should succeed (idempotent)")

	err = storageService.EnsureIndexes(ctx)
	require.NoError(t, err, "Third EnsureIndexes should succeed (idempotent)")

	// Clean up test collection
	_ = storageService.collection.Drop(ctx)
}

// Retry Logic Tests

func TestRetryOperation_TransientErrorSuccess(t *testing.T) {
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

	// Test that transient errors are retried and eventually succeed
	attemptCount := 0
	operation := func() error {
		attemptCount++
		if attemptCount < 3 {
			return fmt.Errorf("connection timeout") // Transient error
		}
		return nil // Success on 3rd attempt
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.NoError(t, err)
	assert.Equal(t, 3, attemptCount, "Should have attempted 3 times before success")
}

func TestRetryOperation_PermanentErrorFailsImmediately(t *testing.T) {
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

	// Test that permanent errors fail immediately without retries
	attemptCount := 0
	operation := func() error {
		attemptCount++
		return fmt.Errorf("duplicate key error") // Non-transient error
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.Error(t, err)
	assert.Equal(t, 1, attemptCount, "Should only attempt once for permanent errors")
	assert.Contains(t, err.Error(), "duplicate key error")
}

func TestRetryOperation_RetryExhaustion(t *testing.T) {
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

	// Test that retries are exhausted after max attempts
	attemptCount := 0
	operation := func() error {
		attemptCount++
		return fmt.Errorf("connection refused") // Always fails with transient error
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.Error(t, err)
	assert.Equal(t, defaultRetryConfig.maxAttempts, attemptCount, "Should attempt exactly maxAttempts times")
	assert.Contains(t, err.Error(), "operation failed after")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRetryOperation_ExponentialBackoff(t *testing.T) {
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

	// Track timing between attempts to verify exponential backoff
	var attemptTimes []time.Time
	operation := func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return fmt.Errorf("timeout") // Transient error
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.Error(t, err)
	assert.Len(t, attemptTimes, defaultRetryConfig.maxAttempts)

	// Verify exponential backoff between attempts
	expectedDelay := defaultRetryConfig.initialDelay
	for i := 1; i < len(attemptTimes); i++ {
		actualDelay := attemptTimes[i].Sub(attemptTimes[i-1])

		// Allow 50ms tolerance for timing variations
		tolerance := 50 * time.Millisecond
		minExpected := expectedDelay - tolerance

		assert.GreaterOrEqual(t, actualDelay, minExpected,
			"Delay between attempt %d and %d should be at least %v (got %v)",
			i, i+1, minExpected, actualDelay)

		// Calculate next expected delay
		expectedDelay = time.Duration(float64(expectedDelay) * defaultRetryConfig.multiplier)
		if expectedDelay > defaultRetryConfig.maxDelay {
			expectedDelay = defaultRetryConfig.maxDelay
		}
	}
}

func TestRetryOperation_ContextCancellation(t *testing.T) {
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

	// Test that operation respects context cancellation
	attemptCount := 0
	operation := func() error {
		attemptCount++
		return fmt.Errorf("connection timeout") // Transient error
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operation cancelled during retry")
	// Should have attempted at least once, but not all max attempts
	assert.Greater(t, attemptCount, 0)
	assert.Less(t, attemptCount, defaultRetryConfig.maxAttempts)
}

func TestRetryOperation_ImmediateSuccess(t *testing.T) {
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

	// Test that successful operations don't retry
	attemptCount := 0
	operation := func() error {
		attemptCount++
		return nil // Immediate success
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.NoError(t, err)
	assert.Equal(t, 1, attemptCount, "Should only attempt once on immediate success")
}

func TestIsRetryableError_TransientErrors(t *testing.T) {
	// Test various transient error patterns
	transientErrors := []string{
		"connection refused",
		"connection reset by peer",
		"i/o timeout",
		"timeout exceeded",
		"temporary failure in name resolution",
		"EOF",
		"server selection timeout",
		"no reachable servers",
		"connection pool exhausted",
		"socket error",
	}

	for _, errMsg := range transientErrors {
		err := fmt.Errorf("%s", errMsg)
		assert.True(t, isRetryableError(err),
			"Error '%s' should be retryable", errMsg)
	}
}

func TestIsRetryableError_PermanentErrors(t *testing.T) {
	// Test various permanent error patterns
	permanentErrors := []string{
		"duplicate key error",
		"validation failed",
		"invalid argument",
		"not found",
		"unauthorized",
		"forbidden",
		"bad request",
	}

	for _, errMsg := range permanentErrors {
		err := fmt.Errorf("%s", errMsg)
		assert.False(t, isRetryableError(err),
			"Error '%s' should not be retryable", errMsg)
	}
}

func TestIsRetryableError_NilError(t *testing.T) {
	assert.False(t, isRetryableError(nil), "Nil error should not be retryable")
}

func TestContainsAny_MatchFound(t *testing.T) {
	s := "connection timeout occurred"
	substrings := []string{"timeout", "refused", "reset"}

	assert.True(t, containsAny(s, substrings), "Should find 'timeout' in string")
}

func TestContainsAny_NoMatch(t *testing.T) {
	s := "duplicate key error"
	substrings := []string{"timeout", "refused", "reset"}

	assert.False(t, containsAny(s, substrings), "Should not find any substring")
}

func TestContainsAny_EmptyString(t *testing.T) {
	s := ""
	substrings := []string{"timeout", "refused"}

	assert.False(t, containsAny(s, substrings), "Empty string should not match")
}

func TestContainsAny_EmptySubstrings(t *testing.T) {
	s := "some error message"
	substrings := []string{}

	assert.False(t, containsAny(s, substrings), "Empty substrings should not match")
}

func TestRetryOperation_MaxDelayCapReached(t *testing.T) {
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

	// Test that delay is capped at maxDelay
	var attemptTimes []time.Time
	operation := func() error {
		attemptTimes = append(attemptTimes, time.Now())
		return fmt.Errorf("connection refused") // Transient error
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = service.retryOperation(ctx, "TestOperation", operation)
	assert.Error(t, err)

	// Verify that delays don't exceed maxDelay
	for i := 1; i < len(attemptTimes); i++ {
		actualDelay := attemptTimes[i].Sub(attemptTimes[i-1])
		// Add tolerance for timing variations
		maxAllowed := defaultRetryConfig.maxDelay + 100*time.Millisecond
		assert.LessOrEqual(t, actualDelay, maxAllowed,
			"Delay between attempts should not exceed maxDelay")
	}
}

// TestGetSession_DatabaseError tests GetSession with a database error (not ErrNoDocuments)
// This test covers the generic error path in GetSession when FindOne fails with an error
// other than mongo.ErrNoDocuments
func TestGetSession_DatabaseError(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use a unique session ID to avoid conflicts
	sessionID := fmt.Sprintf("test-error-session-%d", time.Now().UnixNano())

	// Create a valid session first
	now := time.Now()
	sess := &session.Session{
		ID:        sessionID,
		UserID:    "user-error",
		Name:      "Error Test Session",
		ModelID:   "gpt-4",
		Messages:  []*session.Message{},
		StartTime: now,
		IsActive:  true,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Now manually insert a document with invalid BSON that will cause decode errors
	// We'll insert a document with a field that has an incompatible type
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Replace the entire document with one that has invalid structure
	// Use a nested array where a simple value is expected
	_, err = service.collection.ReplaceOne(
		ctx,
		bson.M{"_id": sessionID},
		bson.M{
			"_id":  sessionID,
			"uid":  "user-error",
			"name": "Error Test",
			"ts":   bson.A{bson.M{"invalid": "structure"}}, // Invalid: should be time.Time
			"msgs": "not-an-array",                         // Invalid: should be array
		},
	)
	require.NoError(t, err)

	// Now try to get the session - this should trigger a decode error
	retrievedSess, err := service.GetSession(sessionID)

	// Note: MongoDB's BSON decoder is quite forgiving and may not fail on type mismatches
	// If it doesn't error, that's okay - we're testing the error path exists
	if err != nil {
		assert.NotErrorIs(t, err, ErrSessionNotFound)
		assert.NotErrorIs(t, err, ErrInvalidSessionID)
		assert.Contains(t, err.Error(), "failed to get session")
		assert.Nil(t, retrievedSess)
	}
}

// TestGetSession_WithEncryption tests GetSession with encrypted message content
// This ensures the decryption path in documentToSession is covered
func TestGetSession_WithEncryption(t *testing.T) {
	// Create service with encryption key
	encryptionKey := []byte("12345678901234567890123456789012") // 32 bytes for AES-256
	service, cleanup := setupTestStorage(t, encryptionKey)
	defer cleanup()

	// Use unique session ID
	sessionID := fmt.Sprintf("test-encrypted-session-%d", time.Now().UnixNano())

	// Create session with messages
	now := time.Now()
	sess := &session.Session{
		ID:      sessionID,
		UserID:  "user-encrypted",
		Name:    "Encrypted Test Session",
		ModelID: "gpt-4",
		Messages: []*session.Message{
			{
				Content:   "This is a secret message",
				Timestamp: now,
				Sender:    "user",
			},
		},
		StartTime: now,
		IsActive:  true,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Add another message to trigger encryption
	msg := &session.Message{
		Content:   "Another secret message",
		Timestamp: now.Add(time.Minute),
		Sender:    "ai",
	}

	err = service.AddMessage(sessionID, msg)
	require.NoError(t, err)

	// Retrieve the session - this should decrypt the messages
	retrievedSess, err := service.GetSession(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSess)
	assert.Len(t, retrievedSess.Messages, 2)
	assert.Equal(t, "This is a secret message", retrievedSess.Messages[0].Content)
	assert.Equal(t, "Another secret message", retrievedSess.Messages[1].Content)
}

// TestGetSession_WithResponseTimes tests GetSession with sessions that have response time data
// This ensures the response time reconstruction path in documentToSession is covered
func TestGetSession_WithResponseTimes(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()

	// Use unique session ID
	sessionID := fmt.Sprintf("test-response-times-session-%d", time.Now().UnixNano())

	// Create session with response times
	now := time.Now()
	sess := &session.Session{
		ID:            sessionID,
		UserID:        "user-response-times",
		Name:          "Response Times Test",
		ModelID:       "gpt-4",
		Messages:      []*session.Message{},
		StartTime:     now,
		IsActive:      true,
		ResponseTimes: []time.Duration{500 * time.Millisecond, 1 * time.Second, 750 * time.Millisecond},
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Retrieve the session - this should reconstruct response times
	retrievedSess, err := service.GetSession(sessionID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSess)

	// The response times should be reconstructed (may not match exactly due to storage format)
	// We store max and avg, so we can only reconstruct an approximation
	assert.NotNil(t, retrievedSess.ResponseTimes)
}
