package storage

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// setupTestMongoDB creates a test MongoDB connection using gomongo
// This uses a local MongoDB instance for testing
// Tests will be skipped if MongoDB is not available
func setupTestMongoDB(t *testing.T) (*gomongo.Mongo, *golog.Logger, func()) {
	// Check if we should skip MongoDB tests
	if os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB tests (SKIP_MONGO_TESTS is set)")
	}

	// Create a test config file
	mongoURI := os.Getenv("MONGO_TEST_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	configContent := fmt.Sprintf(`
[dbs]
verbose = 1
slowThreshold = 2

[dbs.test_chat_db]
uri = "%s/test_chat_db"
`, mongoURI)

	// Write config to temp file
	tmpFile, err := os.CreateTemp("", "test_config_*.toml")
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to create temp config: %v", err)
		return nil, nil, func() {}
	}
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(configContent)
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to write config: %v", err)
		return nil, nil, func() {}
	}

	// Set config file path via environment or command line flag
	os.Setenv("CONFIG_FILE", tmpFile.Name())

	// Reset config state before loading
	goconfig.ResetConfig()

	// Load config
	err = goconfig.LoadConfig()
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to load config: %v", err)
		return nil, nil, func() {}
	}

	configAccessor, err := goconfig.Default()
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to get config accessor: %v", err)
		return nil, nil, func() {}
	}

	// Initialize logger
	logger, err := golog.InitLog(golog.LogConfig{
		Level:          "info",
		StandardOutput: false,
	})
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to initialize logger: %v", err)
		return nil, nil, func() {}
	}

	// Initialize gomongo
	mongoClient, err := gomongo.InitMongoDB(logger, configAccessor)
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to initialize gomongo: %v", err)
		return nil, nil, func() {}
	}

	// Test connection by getting a collection
	testColl := mongoClient.Coll("test_chat_db", "test_connection")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try a simple operation to verify connection
	_, err = testColl.InsertOne(ctx, bson.M{"test": "connection"})
	if err != nil {
		t.Skipf("Skipping MongoDB tests: failed to verify connection: %v", err)
		return nil, nil, func() {}
	}

	// Return cleanup function
	cleanup := func() {
		// Clean up temp config file
		os.Remove(tmpFile.Name())
		os.Unsetenv("CONFIG_FILE")

		// Drop test collections
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Get the underlying database to drop collections
		db, _ := mongoClient.Database("test_chat_db")
		if db != nil {
			db.Coll("sessions").Drop(ctx)
			db.Coll("prop_sessions").Drop(ctx)
			db.Coll("test_connection").Drop(ctx)
		}

		logger.Close()
		goconfig.ResetConfig()
	}

	return mongoClient, logger, cleanup
}

func TestNewStorageService(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	assert.NotNil(t, service)
	assert.NotNil(t, service.mongo)
	assert.NotNil(t, service.collection)
	assert.NotNil(t, service.logger)
}

func TestCreateSession_ValidSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	err := service.CreateSession(nil)
	assert.ErrorIs(t, err, ErrInvalidSession)
}

func TestCreateSession_EmptySessionID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	sess := &session.Session{
		ID:     "",
		UserID: "user-123",
	}

	err := service.CreateSession(sess)
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestUpdateSession_ValidSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	sess := &session.Session{
		ID:     "non-existent",
		UserID: "user-789",
	}

	err := service.UpdateSession(sess)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestGetSession_ValidSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	sess, err := service.GetSession("non-existent")
	assert.ErrorIs(t, err, ErrSessionNotFound)
	assert.Nil(t, sess)
}

func TestGetSession_EmptySessionID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	sess, err := service.GetSession("")
	assert.ErrorIs(t, err, ErrInvalidSessionID)
	assert.Nil(t, sess)
}

func TestSessionToDocument_WithMessages(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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

func TestAddMessage_ValidMessage(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	// Create 32-byte encryption key for AES-256
	encryptionKey := []byte("12345678901234567890123456789012")
	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, encryptionKey)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	msg := &session.Message{
		Content:   "Test",
		Timestamp: time.Now(),
		Sender:    "user",
	}

	err := service.AddMessage("non-existent", msg)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestAddMessage_EmptySessionID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	msg := &session.Message{
		Content:   "Test",
		Timestamp: time.Now(),
		Sender:    "user",
	}

	err := service.AddMessage("", msg)
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}

func TestAddMessage_NilMessage(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	err := service.AddMessage("test-session", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message cannot be nil")
}

func TestEndSession_ValidSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	assert.Equal(t, endTime, *doc.EndTime)
	assert.Equal(t, int64(300), doc.Duration) // 5 minutes = 300 seconds
}

func TestEndSession_NonExistentSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	err := service.EndSession("non-existent", time.Now())
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestEndSession_EmptySessionID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	metadata, err := service.ListUserSessions("", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user ID cannot be empty")
	assert.Nil(t, metadata)
}

func TestListUserSessions_NoSessions(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	metadata, err := service.ListUserSessions("user-no-sessions", 0)
	assert.NoError(t, err)
	assert.Empty(t, metadata)
}

func TestListUserSessions_LastMessageTime(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	now := time.Now()
	startTime := now
	endTime := now.Add(-1 * time.Hour) // End before start

	metrics, err := service.GetSessionMetrics(startTime, endTime)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
	assert.Nil(t, metrics)
}

func TestGetSessionMetrics_ConcurrentSessions(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	assert.Greater(t, metrics.MaxConcurrent, 0)
}

func TestGetTokenUsage_ValidTimeRange(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	endTime := now

	// Get token usage with no sessions
	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.NoError(t, err)
	assert.Equal(t, 0, totalTokens)
}

func TestGetTokenUsage_InvalidTimeRange(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

	now := time.Now()
	startTime := now
	endTime := now.Add(-1 * time.Hour) // End before start

	totalTokens, err := service.GetTokenUsage(startTime, endTime)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "end time must be after start time")
	assert.Equal(t, 0, totalTokens)
}

func TestGetTokenUsage_PartialTimeRange(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "test_chat_db", "sessions", logger, nil)

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
