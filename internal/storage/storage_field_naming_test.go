package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// TestMongoDBFieldNaming_CreateAndRead tests that sessions are stored with camelCase field names
// and can be read back correctly
func TestMongoDBFieldNaming_CreateAndRead(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_test", logger, nil)

	now := time.Now()
	sess := &session.Session{
		ID:                 "field-test-1",
		UserID:             "user-123",
		Name:               "Field Naming Test",
		ModelID:            "gpt-4",
		Messages:           []*session.Message{},
		StartTime:          now,
		LastActivity:       now,
		EndTime:            nil,
		IsActive:           true,
		HelpRequested:      false,
		AdminAssisted:      true,
		AssistingAdminID:   "admin-456",
		AssistingAdminName: "Admin User",
		TotalTokens:        100,
		ResponseTimes:      []time.Duration{time.Second},
	}

	// Create session
	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Verify field names in MongoDB are camelCase
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawDoc bson.M
	err = service.collection.FindOne(ctx, bson.M{"_id": "field-test-1"}).Decode(&rawDoc)
	require.NoError(t, err)

	// Check that camelCase field names are used
	assert.Contains(t, rawDoc, "uid", "Should use 'uid' not 'user_id'")
	assert.Contains(t, rawDoc, "nm", "Should use 'nm' not 'name'")
	assert.Contains(t, rawDoc, "modelId", "Should use 'modelId' not 'model_id'")
	assert.Contains(t, rawDoc, "msgs", "Should use 'msgs' not 'messages'")
	assert.Contains(t, rawDoc, "ts", "Should use 'ts' not 'start_time'")
	assert.Contains(t, rawDoc, "dur", "Should use 'dur' not 'duration'")
	assert.Contains(t, rawDoc, "adminAssisted", "Should use 'adminAssisted' not 'admin_assisted'")
	assert.Contains(t, rawDoc, "assistingAdminId", "Should use 'assistingAdminId' not 'assisting_admin_id'")
	assert.Contains(t, rawDoc, "assistingAdminName", "Should use 'assistingAdminName' not 'assisting_admin_name'")
	assert.Contains(t, rawDoc, "helpRequested", "Should use 'helpRequested' not 'help_requested'")
	assert.Contains(t, rawDoc, "totalTokens", "Should use 'totalTokens' not 'total_tokens'")
	assert.Contains(t, rawDoc, "maxRespTime", "Should use 'maxRespTime' not 'max_response_time'")
	assert.Contains(t, rawDoc, "avgRespTime", "Should use 'avgRespTime' not 'avg_response_time'")

	// Verify values are correct
	assert.Equal(t, "user-123", rawDoc["uid"])
	assert.Equal(t, "Field Naming Test", rawDoc["nm"])
	assert.Equal(t, "gpt-4", rawDoc["modelId"])
	assert.Equal(t, true, rawDoc["adminAssisted"])
	assert.Equal(t, "admin-456", rawDoc["assistingAdminId"])
	assert.Equal(t, "Admin User", rawDoc["assistingAdminName"])
	assert.Equal(t, int32(100), rawDoc["totalTokens"])

	// Verify session can be read back correctly
	retrievedSess, err := service.GetSession("field-test-1")
	require.NoError(t, err)
	assert.Equal(t, "user-123", retrievedSess.UserID)
	assert.Equal(t, "Field Naming Test", retrievedSess.Name)
	assert.Equal(t, "gpt-4", retrievedSess.ModelID)
	assert.True(t, retrievedSess.AdminAssisted)
	assert.Equal(t, "admin-456", retrievedSess.AssistingAdminID)
	assert.Equal(t, "Admin User", retrievedSess.AssistingAdminName)
	assert.Equal(t, 100, retrievedSess.TotalTokens)
}

// TestMongoDBFieldNaming_QueryByUserID tests querying by uid field
func TestMongoDBFieldNaming_QueryByUserID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_query", logger, nil)

	// Create sessions for different users
	now := time.Now()
	for i := 0; i < 3; i++ {
		sess := &session.Session{
			ID:        fmt.Sprintf("query-test-%d", i),
			UserID:    fmt.Sprintf("user-%d", i),
			Name:      fmt.Sprintf("Session %d", i),
			Messages:  []*session.Message{},
			StartTime: now,
		}
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Query by UserID using ListAllSessionsWithOptions
	opts := &SessionListOptions{
		UserID: "user-1",
		Limit:  100,
	}
	sessions, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "user-1", sessions[0].UserID)
	assert.Equal(t, "query-test-1", sessions[0].ID)
}

// TestMongoDBFieldNaming_QueryByStartTime tests querying by ts field
func TestMongoDBFieldNaming_QueryByStartTime(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_time", logger, nil)

	now := time.Now()

	// Create sessions at different times
	sessions := []*session.Session{
		{
			ID:        "time-test-1",
			UserID:    "user-1",
			Name:      "Old Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(-3 * time.Hour),
		},
		{
			ID:        "time-test-2",
			UserID:    "user-1",
			Name:      "Recent Session",
			Messages:  []*session.Message{},
			StartTime: now.Add(-1 * time.Hour),
		},
		{
			ID:        "time-test-3",
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

	// Query by start time range
	startTimeFrom := now.Add(-2 * time.Hour)
	startTimeTo := now
	opts := &SessionListOptions{
		StartTimeFrom: &startTimeFrom,
		StartTimeTo:   &startTimeTo,
		Limit:         100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "time-test-2", result[0].ID)
}

// TestMongoDBFieldNaming_QueryByAdminAssisted tests querying by adminAssisted field
func TestMongoDBFieldNaming_QueryByAdminAssisted(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_admin", logger, nil)

	now := time.Now()

	// Create sessions with different admin assistance status
	sessions := []*session.Session{
		{
			ID:            "admin-test-1",
			UserID:        "user-1",
			Name:          "Assisted Session",
			Messages:      []*session.Message{},
			StartTime:     now,
			AdminAssisted: true,
		},
		{
			ID:            "admin-test-2",
			UserID:        "user-1",
			Name:          "Not Assisted Session",
			Messages:      []*session.Message{},
			StartTime:     now,
			AdminAssisted: false,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Query for admin assisted sessions
	adminAssisted := true
	opts := &SessionListOptions{
		AdminAssisted: &adminAssisted,
		Limit:         100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "admin-test-1", result[0].ID)
	assert.True(t, result[0].AdminAssisted)
}

// TestMongoDBFieldNaming_QueryByActiveStatus tests querying by endTs field
func TestMongoDBFieldNaming_QueryByActiveStatus(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_active", logger, nil)

	now := time.Now()
	endTime := now.Add(-1 * time.Hour)

	// Create active and ended sessions
	sessions := []*session.Session{
		{
			ID:        "active-test-1",
			UserID:    "user-1",
			Name:      "Active Session",
			Messages:  []*session.Message{},
			StartTime: now,
			EndTime:   nil,
		},
		{
			ID:        "active-test-2",
			UserID:    "user-1",
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

	// Query for active sessions (no endTs)
	active := true
	opts := &SessionListOptions{
		Active: &active,
		Limit:  100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "active-test-1", result[0].ID)
	assert.Nil(t, result[0].EndTime)

	// Query for ended sessions (has endTs)
	notActive := false
	opts.Active = &notActive
	result, err = service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "active-test-2", result[0].ID)
	assert.NotNil(t, result[0].EndTime)
}

// TestMongoDBFieldNaming_SortByStartTime tests sorting by ts field
func TestMongoDBFieldNaming_SortByStartTime(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_sort_ts", logger, nil)

	now := time.Now()

	// Create sessions at different times
	sessions := []*session.Session{
		{
			ID:        "sort-ts-3",
			UserID:    "user-1",
			Name:      "Session 3",
			Messages:  []*session.Message{},
			StartTime: now.Add(-1 * time.Hour),
		},
		{
			ID:        "sort-ts-1",
			UserID:    "user-1",
			Name:      "Session 1",
			Messages:  []*session.Message{},
			StartTime: now.Add(-3 * time.Hour),
		},
		{
			ID:        "sort-ts-2",
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

	// Sort by start time descending
	opts := &SessionListOptions{
		SortBy:    "ts",
		SortOrder: "desc",
		Limit:     100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "sort-ts-3", result[0].ID)
	assert.Equal(t, "sort-ts-2", result[1].ID)
	assert.Equal(t, "sort-ts-1", result[2].ID)

	// Sort by start time ascending
	opts.SortOrder = "asc"
	result, err = service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "sort-ts-1", result[0].ID)
	assert.Equal(t, "sort-ts-2", result[1].ID)
	assert.Equal(t, "sort-ts-3", result[2].ID)
}

// TestMongoDBFieldNaming_SortByTotalTokens tests sorting by totalTokens field
func TestMongoDBFieldNaming_SortByTotalTokens(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_sort_tokens", logger, nil)

	now := time.Now()

	// Create sessions with different token counts
	sessions := []*session.Session{
		{
			ID:          "sort-tokens-1",
			UserID:      "user-1",
			Name:        "Session 1",
			Messages:    []*session.Message{},
			StartTime:   now,
			TotalTokens: 100,
		},
		{
			ID:          "sort-tokens-2",
			UserID:      "user-1",
			Name:        "Session 2",
			Messages:    []*session.Message{},
			StartTime:   now,
			TotalTokens: 500,
		},
		{
			ID:          "sort-tokens-3",
			UserID:      "user-1",
			Name:        "Session 3",
			Messages:    []*session.Message{},
			StartTime:   now,
			TotalTokens: 250,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Sort by total tokens descending
	opts := &SessionListOptions{
		SortBy:    "totalTokens",
		SortOrder: "desc",
		Limit:     100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "sort-tokens-2", result[0].ID)
	assert.Equal(t, 500, result[0].TotalTokens)
	assert.Equal(t, "sort-tokens-3", result[1].ID)
	assert.Equal(t, 250, result[1].TotalTokens)
	assert.Equal(t, "sort-tokens-1", result[2].ID)
	assert.Equal(t, 100, result[2].TotalTokens)
}

// TestMongoDBFieldNaming_SortByUserID tests sorting by uid field
func TestMongoDBFieldNaming_SortByUserID(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_sort_uid", logger, nil)

	now := time.Now()

	// Create sessions for different users
	sessions := []*session.Session{
		{
			ID:        "sort-uid-1",
			UserID:    "user-charlie",
			Name:      "Session Charlie",
			Messages:  []*session.Message{},
			StartTime: now,
		},
		{
			ID:        "sort-uid-2",
			UserID:    "user-alice",
			Name:      "Session Alice",
			Messages:  []*session.Message{},
			StartTime: now,
		},
		{
			ID:        "sort-uid-3",
			UserID:    "user-bob",
			Name:      "Session Bob",
			Messages:  []*session.Message{},
			StartTime: now,
		},
	}

	for _, sess := range sessions {
		err := service.CreateSession(sess)
		require.NoError(t, err)
	}

	// Sort by user ID ascending
	opts := &SessionListOptions{
		SortBy:    "uid",
		SortOrder: "asc",
		Limit:     100,
	}
	result, err := service.ListAllSessionsWithOptions(opts)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "user-alice", result[0].UserID)
	assert.Equal(t, "user-bob", result[1].UserID)
	assert.Equal(t, "user-charlie", result[2].UserID)
}

// TestMongoDBFieldNaming_UpdateSession tests updating with new field names
func TestMongoDBFieldNaming_UpdateSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_update", logger, nil)

	now := time.Now()

	// Create initial session
	sess := &session.Session{
		ID:            "update-test-1",
		UserID:        "user-123",
		Name:          "Initial Name",
		Messages:      []*session.Message{},
		StartTime:     now,
		TotalTokens:   100,
		AdminAssisted: false,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Update session
	sess.Name = "Updated Name"
	sess.TotalTokens = 200
	sess.AdminAssisted = true
	sess.AssistingAdminID = "admin-789"
	sess.AssistingAdminName = "Admin Smith"

	err = service.UpdateSession(sess)
	require.NoError(t, err)

	// Verify update in MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawDoc bson.M
	err = service.collection.FindOne(ctx, bson.M{"_id": "update-test-1"}).Decode(&rawDoc)
	require.NoError(t, err)

	assert.Equal(t, "Updated Name", rawDoc["nm"])
	assert.Equal(t, int32(200), rawDoc["totalTokens"])
	assert.Equal(t, true, rawDoc["adminAssisted"])
	assert.Equal(t, "admin-789", rawDoc["assistingAdminId"])
	assert.Equal(t, "Admin Smith", rawDoc["assistingAdminName"])
}

// TestMongoDBFieldNaming_AddMessage tests adding messages with new field names
func TestMongoDBFieldNaming_AddMessage(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_msg", logger, nil)

	now := time.Now()

	// Create session
	sess := &session.Session{
		ID:        "msg-test-1",
		UserID:    "user-123",
		Name:      "Message Test",
		Messages:  []*session.Message{},
		StartTime: now,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// Add message
	msg := &session.Message{
		Content:   "Test message",
		Timestamp: now,
		Sender:    "user",
		FileID:    "file-123",
		FileURL:   "https://example.com/file",
		Metadata:  map[string]string{"key": "value"},
	}

	err = service.AddMessage("msg-test-1", msg)
	require.NoError(t, err)

	// Verify message in MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawDoc bson.M
	err = service.collection.FindOne(ctx, bson.M{"_id": "msg-test-1"}).Decode(&rawDoc)
	require.NoError(t, err)

	// Check messages array uses 'msgs' field
	assert.Contains(t, rawDoc, "msgs")
	msgs, ok := rawDoc["msgs"].(bson.A)
	require.True(t, ok)
	require.Len(t, msgs, 1)

	// Check message fields
	msgDoc, ok := msgs[0].(bson.M)
	require.True(t, ok)
	assert.Equal(t, "Test message", msgDoc["content"])
	assert.Equal(t, "user", msgDoc["sender"])
	assert.Equal(t, "file-123", msgDoc["fileId"])
	assert.Equal(t, "https://example.com/file", msgDoc["fileUrl"])
	assert.Contains(t, msgDoc, "meta")
}

// TestMongoDBFieldNaming_EndSession tests ending session with endTs field
func TestMongoDBFieldNaming_EndSession(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_end", logger, nil)

	now := time.Now()

	// Create session
	sess := &session.Session{
		ID:        "end-test-1",
		UserID:    "user-123",
		Name:      "End Test",
		Messages:  []*session.Message{},
		StartTime: now,
		IsActive:  true,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// End session
	endTime := now.Add(5 * time.Minute)
	err = service.EndSession("end-test-1", endTime)
	require.NoError(t, err)

	// Verify endTs field in MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawDoc bson.M
	err = service.collection.FindOne(ctx, bson.M{"_id": "end-test-1"}).Decode(&rawDoc)
	require.NoError(t, err)

	assert.Contains(t, rawDoc, "endTs")
	assert.Contains(t, rawDoc, "dur")
	assert.Equal(t, int64(300), rawDoc["dur"]) // 5 minutes = 300 seconds
}

// TestMongoDBFieldNaming_CombinedOperations tests a realistic workflow with all operations
func TestMongoDBFieldNaming_CombinedOperations(t *testing.T) {
	mongoClient, logger, cleanup := setupTestMongoDB(t)
	defer cleanup()

	service := NewStorageService(mongoClient, "chatbox", "field_naming_combined", logger, nil)

	now := time.Now()

	// 1. Create session
	sess := &session.Session{
		ID:        "combined-test-1",
		UserID:    "user-123",
		Name:      "Combined Test",
		ModelID:   "gpt-4",
		Messages:  []*session.Message{},
		StartTime: now,
		IsActive:  true,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err)

	// 2. Add messages
	for i := 0; i < 3; i++ {
		msg := &session.Message{
			Content:   fmt.Sprintf("Message %d", i),
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			Sender:    "user",
		}
		err = service.AddMessage("combined-test-1", msg)
		require.NoError(t, err)
	}

	// 3. Update session (admin takeover)
	sess.AdminAssisted = true
	sess.AssistingAdminID = "admin-456"
	sess.AssistingAdminName = "Admin User"
	sess.TotalTokens = 150
	err = service.UpdateSession(sess)
	require.NoError(t, err)

	// 4. End session
	endTime := now.Add(10 * time.Minute)
	err = service.EndSession("combined-test-1", endTime)
	require.NoError(t, err)

	// 5. Query and verify
	retrievedSess, err := service.GetSession("combined-test-1")
	require.NoError(t, err)
	assert.Equal(t, "user-123", retrievedSess.UserID)
	assert.Equal(t, "Combined Test", retrievedSess.Name)
	assert.Equal(t, "gpt-4", retrievedSess.ModelID)
	assert.Len(t, retrievedSess.Messages, 3)
	assert.True(t, retrievedSess.AdminAssisted)
	assert.Equal(t, "admin-456", retrievedSess.AssistingAdminID)
	assert.Equal(t, "Admin User", retrievedSess.AssistingAdminName)
	assert.Equal(t, 150, retrievedSess.TotalTokens)
	assert.NotNil(t, retrievedSess.EndTime)
	assert.False(t, retrievedSess.IsActive)

	// 6. Verify in raw MongoDB document
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rawDoc bson.M
	err = service.collection.FindOne(ctx, bson.M{"_id": "combined-test-1"}).Decode(&rawDoc)
	require.NoError(t, err)

	// Verify all camelCase field names
	assert.Equal(t, "user-123", rawDoc["uid"])
	assert.Equal(t, "Combined Test", rawDoc["nm"])
	assert.Equal(t, "gpt-4", rawDoc["modelId"])
	assert.Equal(t, true, rawDoc["adminAssisted"])
	assert.Equal(t, "admin-456", rawDoc["assistingAdminId"])
	assert.Equal(t, "Admin User", rawDoc["assistingAdminName"])
	assert.Equal(t, int32(150), rawDoc["totalTokens"])
	assert.Contains(t, rawDoc, "endTs")
	assert.Contains(t, rawDoc, "dur")
	assert.Contains(t, rawDoc, "msgs")

	msgs, ok := rawDoc["msgs"].(bson.A)
	require.True(t, ok)
	assert.Len(t, msgs, 3)
}
