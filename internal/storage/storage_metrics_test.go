package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// setupMetricsTestStorage creates a test storage service for metrics testing
// This helper function sets up a real MongoDB connection for metrics unit testing
// Tests will be skipped if MongoDB is not available
// Now uses shared MongoDB client to avoid initialization conflicts
func setupMetricsTestStorage(t *testing.T) (*StorageService, func()) {
	return setupTestStorageShared(t)
}

// createTestSessionWithMetrics creates a test session with metrics data
// This helper function creates a session with specified metrics for testing
// Returns the created session for verification
func createTestSessionWithMetrics(t *testing.T, service *StorageService, opts MetricsSessionOptions) *session.Session {
	// Generate unique session ID using timestamp and user ID
	sessionID := fmt.Sprintf("metrics-test-%s-%d", opts.UserID, time.Now().UnixNano())

	// Create messages if specified
	messages := make([]*session.Message, opts.MessageCount)
	for i := 0; i < opts.MessageCount; i++ {
		messages[i] = &session.Message{
			Content:   fmt.Sprintf("Test message %d", i+1),
			Timestamp: opts.StartTime.Add(time.Duration(i) * time.Minute),
			Sender:    "user",
			FileID:    "",
			FileURL:   "",
			Metadata:  map[string]string{},
		}
	}

	// Create response times if specified
	responseTimes := make([]time.Duration, opts.ResponseTimeCount)
	for i := 0; i < opts.ResponseTimeCount; i++ {
		responseTimes[i] = opts.AvgResponseTime
	}

	sess := &session.Session{
		ID:                 sessionID,
		UserID:             opts.UserID,
		Name:               opts.Name,
		ModelID:            opts.ModelID,
		Messages:           messages,
		StartTime:          opts.StartTime,
		LastActivity:       opts.StartTime.Add(time.Duration(opts.MessageCount) * time.Minute),
		EndTime:            opts.EndTime,
		IsActive:           opts.EndTime == nil,
		HelpRequested:      opts.HelpRequested,
		AdminAssisted:      opts.AdminAssisted,
		AssistingAdminID:   opts.AssistingAdminID,
		AssistingAdminName: opts.AssistingAdminName,
		TotalTokens:        opts.TotalTokens,
		ResponseTimes:      responseTimes,
	}

	err := service.CreateSession(sess)
	require.NoError(t, err, "Failed to create test session with metrics")

	return sess
}

// MetricsSessionOptions defines options for creating test sessions with metrics
type MetricsSessionOptions struct {
	UserID             string
	Name               string
	ModelID            string
	StartTime          time.Time
	EndTime            *time.Time
	MessageCount       int
	TotalTokens        int
	ResponseTimeCount  int
	AvgResponseTime    time.Duration
	AdminAssisted      bool
	AssistingAdminID   string
	AssistingAdminName string
	HelpRequested      bool
}

// cleanupMetricsTestSession removes a test session from the database
// This helper function ensures test data is cleaned up after each test
func cleanupMetricsTestSession(t *testing.T, service *StorageService, sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := service.collection.DeleteOne(ctx, bson.M{"_id": sessionID})
	if err != nil {
		t.Logf("Warning: Failed to cleanup metrics test session %s: %v", sessionID, err)
	}
}

// cleanupMetricsTestSessions removes multiple test sessions from the database
// This helper function cleans up multiple sessions at once
func cleanupMetricsTestSessions(t *testing.T, service *StorageService, sessionIDs []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, sessionID := range sessionIDs {
		_, err := service.collection.DeleteOne(ctx, bson.M{"_id": sessionID})
		if err != nil {
			t.Logf("Warning: Failed to cleanup metrics test session %s: %v", sessionID, err)
		}
	}
}

// TestGetSessionMetrics_SingleActiveSession tests metrics calculation for a single active session
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_SingleActiveSession(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	// Create a single active session
	startTime := time.Now().Add(-1 * time.Hour)
	sess := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Single Active Session",
		ModelID:           "gpt-4",
		StartTime:         startTime,
		EndTime:           nil, // Active session
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   500 * time.Millisecond,
		AdminAssisted:     false,
	})
	defer cleanupMetricsTestSession(t, storage, sess.ID)

	// Get metrics for the time range
	metricsStartTime := startTime.Add(-10 * time.Minute)
	metricsEndTime := time.Now().Add(10 * time.Minute)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify metrics
	require.Equal(t, 1, metrics.TotalSessions, "Should have 1 total session")
	require.Equal(t, 1, metrics.ActiveSessions, "Should have 1 active session")
	require.Equal(t, 0, metrics.AdminAssistedCount, "Should have 0 admin-assisted sessions")
	require.Equal(t, 1000, metrics.TotalTokens, "Should have 1000 total tokens")
	require.Equal(t, int64(500), metrics.AvgResponseTime, "Average response time should be 500ms")
	require.Equal(t, int64(500), metrics.MaxResponseTime, "Max response time should be 500ms")
}

// TestGetSessionMetrics_MultipleSessions tests metrics with multiple sessions in different states
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_MultipleSessions(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-2 * time.Hour)
	sessionIDs := []string{}

	// Create active session
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Active Session 1",
		ModelID:           "gpt-4",
		StartTime:         baseTime,
		EndTime:           nil, // Active
		MessageCount:      10,
		TotalTokens:       2000,
		ResponseTimeCount: 5,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Create ended session
	endTime1 := baseTime.Add(30 * time.Minute)
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Ended Session 1",
		ModelID:           "gpt-3.5",
		StartTime:         baseTime.Add(10 * time.Minute),
		EndTime:           &endTime1,
		MessageCount:      5,
		TotalTokens:       800,
		ResponseTimeCount: 3,
		AvgResponseTime:   400 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Create another active session
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user3",
		Name:              "Active Session 2",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(20 * time.Minute),
		EndTime:           nil, // Active
		MessageCount:      8,
		TotalTokens:       1500,
		ResponseTimeCount: 4,
		AvgResponseTime:   600 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get metrics for the time range
	metricsStartTime := baseTime.Add(-10 * time.Minute)
	metricsEndTime := time.Now().Add(10 * time.Minute)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify metrics
	require.Equal(t, 3, metrics.TotalSessions, "Should have 3 total sessions")
	require.Equal(t, 2, metrics.ActiveSessions, "Should have 2 active sessions")
	require.Equal(t, 0, metrics.AdminAssistedCount, "Should have 0 admin-assisted sessions")
	require.Equal(t, 4300, metrics.TotalTokens, "Should have 4300 total tokens (2000+800+1500)")

	// Average response time should be (300+400+600)/3 = 433ms (rounded)
	require.InDelta(t, 433, metrics.AvgResponseTime, 1, "Average response time should be ~433ms")

	// Max response time should be 600ms
	require.Equal(t, int64(600), metrics.MaxResponseTime, "Max response time should be 600ms")

	// Note: MaxConcurrent/AvgConcurrent are not computed by the aggregation pipeline (H3 optimization).
	// These fields remain 0 as the trade-off for avoiding OOM with unbounded in-memory processing.
	require.Equal(t, 0, metrics.MaxConcurrent, "Max concurrent not computed by aggregation pipeline")
}

// TestGetSessionMetrics_AdminAssisted tests metrics with admin-assisted sessions
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_AdminAssisted(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-1 * time.Hour)
	sessionIDs := []string{}

	// Create admin-assisted session
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:             "user1",
		Name:               "Admin Assisted Session",
		ModelID:            "gpt-4",
		StartTime:          baseTime,
		EndTime:            nil,
		MessageCount:       10,
		TotalTokens:        2000,
		ResponseTimeCount:  5,
		AvgResponseTime:    300 * time.Millisecond,
		AdminAssisted:      true,
		AssistingAdminID:   "admin1",
		AssistingAdminName: "Admin User",
		HelpRequested:      true,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Create non-assisted session
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Regular Session",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(10 * time.Minute),
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   400 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Create another admin-assisted session
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:             "user3",
		Name:               "Another Admin Assisted",
		ModelID:            "gpt-3.5",
		StartTime:          baseTime.Add(20 * time.Minute),
		EndTime:            nil,
		MessageCount:       7,
		TotalTokens:        1200,
		ResponseTimeCount:  4,
		AvgResponseTime:    350 * time.Millisecond,
		AdminAssisted:      true,
		AssistingAdminID:   "admin2",
		AssistingAdminName: "Another Admin",
		HelpRequested:      true,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get metrics for the time range
	metricsStartTime := baseTime.Add(-10 * time.Minute)
	metricsEndTime := time.Now().Add(10 * time.Minute)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify metrics
	require.Equal(t, 3, metrics.TotalSessions, "Should have 3 total sessions")
	require.Equal(t, 3, metrics.ActiveSessions, "Should have 3 active sessions")
	require.Equal(t, 2, metrics.AdminAssistedCount, "Should have 2 admin-assisted sessions")
	require.Equal(t, 4200, metrics.TotalTokens, "Should have 4200 total tokens")
}

// TestGetSessionMetrics_ResponseTimeCalculation tests response time metrics calculation
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_ResponseTimeCalculation(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-1 * time.Hour)
	sessionIDs := []string{}

	// Create session with fast response times
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Fast Session",
		ModelID:           "gpt-4",
		StartTime:         baseTime,
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 5,
		AvgResponseTime:   200 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Create session with slow response times
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Slow Session",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(10 * time.Minute),
		EndTime:           nil,
		MessageCount:      3,
		TotalTokens:       800,
		ResponseTimeCount: 3,
		AvgResponseTime:   1000 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Create session with medium response times
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user3",
		Name:              "Medium Session",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(20 * time.Minute),
		EndTime:           nil,
		MessageCount:      4,
		TotalTokens:       900,
		ResponseTimeCount: 4,
		AvgResponseTime:   500 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get metrics for the time range
	metricsStartTime := baseTime.Add(-10 * time.Minute)
	metricsEndTime := time.Now().Add(10 * time.Minute)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify response time calculations
	// Average: (200 + 1000 + 500) / 3 = 566.67ms
	require.InDelta(t, 566, metrics.AvgResponseTime, 1, "Average response time should be ~566ms")

	// Max should be 1000ms (from slow session)
	require.Equal(t, int64(1000), metrics.MaxResponseTime, "Max response time should be 1000ms")
}

// TestGetSessionMetrics_ConcurrentTracking tests concurrent session tracking
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_ConcurrentTracking(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-2 * time.Hour)
	sessionIDs := []string{}

	// Create overlapping sessions to test concurrent tracking
	// Session 1: starts at T+0, ends at T+30min
	endTime1 := baseTime.Add(30 * time.Minute)
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Session 1",
		ModelID:           "gpt-4",
		StartTime:         baseTime,
		EndTime:           &endTime1,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Session 2: starts at T+10min, ends at T+40min (overlaps with session 1)
	endTime2 := baseTime.Add(40 * time.Minute)
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Session 2",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(10 * time.Minute),
		EndTime:           &endTime2,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Session 3: starts at T+15min, ends at T+25min (overlaps with both)
	endTime3 := baseTime.Add(25 * time.Minute)
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user3",
		Name:              "Session 3",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(15 * time.Minute),
		EndTime:           &endTime3,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get metrics for the time range
	metricsStartTime := baseTime.Add(-10 * time.Minute)
	metricsEndTime := baseTime.Add(1 * time.Hour)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify concurrent session tracking
	require.Equal(t, 3, metrics.TotalSessions, "Should have 3 total sessions")
	require.Equal(t, 0, metrics.ActiveSessions, "Should have 0 active sessions (all ended)")

	// Note: MaxConcurrent/AvgConcurrent are not computed by the aggregation pipeline (H3 optimization).
	// These fields remain 0 as the trade-off for avoiding OOM with unbounded in-memory processing.
	require.Equal(t, 0, metrics.MaxConcurrent, "Max concurrent not computed by aggregation pipeline")
	require.Equal(t, 0.0, metrics.AvgConcurrent, "Avg concurrent not computed by aggregation pipeline")
}

// TestGetSessionMetrics_InvalidTimeRangeEndBeforeStart tests error handling for invalid time range
// Validates: Requirements 3.1 - Error paths are tested
func TestGetSessionMetrics_InvalidTimeRangeEndBeforeStart(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	// Test with end time before start time
	startTime := time.Now()
	endTime := startTime.Add(-1 * time.Hour) // End before start

	metrics, err := storage.GetSessionMetrics(startTime, endTime)
	require.Error(t, err, "Should return error for invalid time range")
	require.Nil(t, metrics, "Metrics should be nil on error")
	require.Contains(t, err.Error(), "end time must be after start time", "Error message should indicate invalid time range")
}

// TestGetSessionMetrics_NoSessionsInRange tests metrics with no sessions in time range
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetSessionMetrics_NoSessionsInRange(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	// Create a session outside the query time range
	oldTime := time.Now().Add(-10 * time.Hour)
	sess := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Old Session",
		ModelID:           "gpt-4",
		StartTime:         oldTime,
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	defer cleanupMetricsTestSession(t, storage, sess.ID)

	// Query for a time range that doesn't include the session
	metricsStartTime := time.Now().Add(-2 * time.Hour)
	metricsEndTime := time.Now().Add(-1 * time.Hour)

	metrics, err := storage.GetSessionMetrics(metricsStartTime, metricsEndTime)
	require.NoError(t, err, "GetSessionMetrics should not return error")
	require.NotNil(t, metrics, "Metrics should not be nil")

	// Verify all metrics are zero
	require.Equal(t, 0, metrics.TotalSessions, "Should have 0 total sessions")
	require.Equal(t, 0, metrics.ActiveSessions, "Should have 0 active sessions")
	require.Equal(t, 0, metrics.AdminAssistedCount, "Should have 0 admin-assisted sessions")
	require.Equal(t, 0, metrics.TotalTokens, "Should have 0 total tokens")
	require.Equal(t, int64(0), metrics.AvgResponseTime, "Average response time should be 0")
	require.Equal(t, int64(0), metrics.MaxResponseTime, "Max response time should be 0")
	require.Equal(t, 0, metrics.MaxConcurrent, "Max concurrent should be 0")
	require.Equal(t, 0.0, metrics.AvgConcurrent, "Average concurrent should be 0")
}

// TestGetTokenUsageMetrics_MultipleSessions tests token usage calculation for multiple sessions
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetTokenUsageMetrics_MultipleSessions(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-2 * time.Hour)
	sessionIDs := []string{}

	// Create session 1 with 1000 tokens
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Session 1",
		ModelID:           "gpt-4",
		StartTime:         baseTime,
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Create session 2 with 2500 tokens
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Session 2",
		ModelID:           "gpt-3.5",
		StartTime:         baseTime.Add(10 * time.Minute),
		EndTime:           nil,
		MessageCount:      8,
		TotalTokens:       2500,
		ResponseTimeCount: 5,
		AvgResponseTime:   400 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Create session 3 with 1500 tokens
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user3",
		Name:              "Session 3",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(20 * time.Minute),
		EndTime:           nil,
		MessageCount:      6,
		TotalTokens:       1500,
		ResponseTimeCount: 4,
		AvgResponseTime:   350 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get token usage for the time range that includes all sessions
	startTime := baseTime.Add(-10 * time.Minute)
	endTime := time.Now().Add(10 * time.Minute)

	totalTokens, err := storage.GetTokenUsage(startTime, endTime)
	require.NoError(t, err, "GetTokenUsage should not return error")

	// Verify total tokens: 1000 + 2500 + 1500 = 5000
	require.Equal(t, 5000, totalTokens, "Total tokens should be 5000")
}

// TestGetTokenUsageMetrics_WithTimeRangeFilter tests token usage with time range filter
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetTokenUsageMetrics_WithTimeRangeFilter(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	baseTime := time.Now().Add(-3 * time.Hour)
	sessionIDs := []string{}

	// Create session 1 at T+0 (1000 tokens) - should be included
	sess1 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Session 1",
		ModelID:           "gpt-4",
		StartTime:         baseTime,
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       1000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess1.ID)

	// Create session 2 at T+30min (2000 tokens) - should be included
	sess2 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user2",
		Name:              "Session 2",
		ModelID:           "gpt-3.5",
		StartTime:         baseTime.Add(30 * time.Minute),
		EndTime:           nil,
		MessageCount:      8,
		TotalTokens:       2000,
		ResponseTimeCount: 5,
		AvgResponseTime:   400 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess2.ID)

	// Create session 3 at T+2hours (3000 tokens) - should NOT be included
	sess3 := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user3",
		Name:              "Session 3",
		ModelID:           "gpt-4",
		StartTime:         baseTime.Add(2 * time.Hour),
		EndTime:           nil,
		MessageCount:      10,
		TotalTokens:       3000,
		ResponseTimeCount: 6,
		AvgResponseTime:   350 * time.Millisecond,
		AdminAssisted:     false,
	})
	sessionIDs = append(sessionIDs, sess3.ID)

	defer cleanupMetricsTestSessions(t, storage, sessionIDs)

	// Get token usage for time range that includes only first two sessions
	startTime := baseTime.Add(-10 * time.Minute)
	endTime := baseTime.Add(1 * time.Hour) // Excludes session 3

	totalTokens, err := storage.GetTokenUsage(startTime, endTime)
	require.NoError(t, err, "GetTokenUsage should not return error")

	// Verify total tokens: 1000 + 2000 = 3000 (session 3 excluded)
	require.Equal(t, 3000, totalTokens, "Total tokens should be 3000 (excluding session 3)")
}

// TestGetTokenUsageMetrics_NoSessionsInRange tests with no sessions in time range (zero tokens)
// Validates: Requirements 3.1 - Metrics aggregation functions have tests with sample data
func TestGetTokenUsageMetrics_NoSessionsInRange(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	// Create a session outside the query time range
	oldTime := time.Now().Add(-10 * time.Hour)
	sess := createTestSessionWithMetrics(t, storage, MetricsSessionOptions{
		UserID:            "user1",
		Name:              "Old Session",
		ModelID:           "gpt-4",
		StartTime:         oldTime,
		EndTime:           nil,
		MessageCount:      5,
		TotalTokens:       5000,
		ResponseTimeCount: 3,
		AvgResponseTime:   300 * time.Millisecond,
		AdminAssisted:     false,
	})
	defer cleanupMetricsTestSession(t, storage, sess.ID)

	// Query for a time range that doesn't include the session
	startTime := time.Now().Add(-2 * time.Hour)
	endTime := time.Now().Add(-1 * time.Hour)

	totalTokens, err := storage.GetTokenUsage(startTime, endTime)
	require.NoError(t, err, "GetTokenUsage should not return error")

	// Verify zero tokens
	require.Equal(t, 0, totalTokens, "Total tokens should be 0 when no sessions in range")
}

// TestGetTokenUsageMetrics_InvalidTimeRange tests with invalid time range (error case)
// Validates: Requirements 3.1 - Error paths are tested
func TestGetTokenUsageMetrics_InvalidTimeRange(t *testing.T) {
	storage, cleanup := setupMetricsTestStorage(t)
	defer cleanup()

	// Test with end time before start time
	startTime := time.Now()
	endTime := startTime.Add(-1 * time.Hour) // End before start

	totalTokens, err := storage.GetTokenUsage(startTime, endTime)
	require.Error(t, err, "Should return error for invalid time range")
	require.Equal(t, 0, totalTokens, "Total tokens should be 0 on error")
	require.Contains(t, err.Error(), "end time must be after start time", "Error message should indicate invalid time range")
}
