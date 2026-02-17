package testutil_test

import (
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file demonstrates how to use the testutil package in production tests.
// These examples show common patterns used in production readiness verification tests.

// Example 1: Testing session creation with MockStorageService
func TestExample_MockStorageService_sessionCreation(t *testing.T) {
	// Create a mock storage service
	mockStorage := &testutil.MockStorageService{}

	// Create a test session
	sess := testutil.CreateTestSession("user-123", "session-abc")

	// Call the storage service
	err := mockStorage.CreateSession(sess)
	require.NoError(t, err)

	// Verify the call was tracked
	assert.True(t, mockStorage.CreateSessionCalled)
	assert.Len(t, mockStorage.CreatedSessions, 1)
	assert.Equal(t, sess.ID, mockStorage.CreatedSessions[0].ID)
}

// Example 2: Testing storage failure scenarios
func TestExample_MockStorageService_errorInjection(t *testing.T) {
	// Create a mock that returns an error
	mockStorage := &testutil.MockStorageService{
		CreateSessionError: assert.AnError,
	}

	sess := testutil.CreateTestSession("user-123", "session-abc")

	// This should fail
	err := mockStorage.CreateSession(sess)
	assert.Error(t, err)

	// Verify the call was attempted
	assert.True(t, mockStorage.CreateSessionCalled)
	// But no session was stored
	assert.Len(t, mockStorage.CreatedSessions, 0)
}

// Example 3: Testing memory growth
func TestExample_MemoryGrowth_sessionAccumulation(t *testing.T) {
	// Measure memory before
	before := testutil.MeasureMemory()

	// Create many sessions
	sessions := make([]*session.Session, 1000)
	for i := 0; i < 1000; i++ {
		sessions[i] = testutil.CreateTestSessionWithMessages("user", "session", 10)
	}

	// Measure memory after
	after := testutil.MeasureMemory()

	// Report memory growth
	testutil.AssertMemoryGrowth(t, before, after, "1000 sessions with 10 messages each")

	// Keep sessions in scope to prevent GC
	_ = sessions
}

// Example 4: Testing goroutine cleanup
func TestExample_GoroutineCount_connectionCleanup(t *testing.T) {
	// Measure goroutines before
	before := testutil.MeasureGoroutines()

	// Simulate connection lifecycle
	done := make(chan bool)
	go func() {
		time.Sleep(10 * time.Millisecond)
		done <- true
	}()
	<-done

	// Wait for goroutines to exit
	testutil.WaitForGoroutines()

	// Measure goroutines after
	after := testutil.MeasureGoroutines()

	// Verify no goroutine leak
	testutil.AssertGoroutineCount(t, before, after, "connection cleanup")
}

// Example 5: Testing concurrent access with data race detection
func TestExample_DataRace_concurrentSessionAccess(t *testing.T) {
	// Document that this test should be run with -race flag
	testutil.AssertNoDataRace(t, "concurrent session field access")

	// Create a mock storage service
	mockStorage := &testutil.MockStorageService{}

	// Launch concurrent CreateSession calls
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			sess := testutil.CreateTestSession("user", "session")
			mockStorage.CreateSession(sess)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all calls were tracked
	assert.True(t, mockStorage.CreateSessionCalled)
	assert.Len(t, mockStorage.CreatedSessions, 10)

	// If run with -race flag, this will detect any data races
}

// Example 6: Using MockLLMService to test streaming
func TestExample_MockLLMService_streaming(t *testing.T) {
	// Create logger
	logger := testutil.CreateTestLogger(t)
	defer logger.Close()

	// Create mock LLM service
	mockLLM := &testutil.MockLLMService{}

	// Use the mock in your code
	// (In real tests, you'd pass this to your router or handler)

	// Verify it was called
	assert.False(t, mockLLM.StreamMessageCalled) // Not called yet

	// After your code calls it:
	// assert.True(t, mockLLM.StreamMessageCalled)
	// assert.Equal(t, "gpt-4", mockLLM.LastStreamModelID)
}

// Example 7: Creating test connections
func TestExample_MockConnection_creation(t *testing.T) {
	// Create a regular user connection
	userConn := testutil.MockConnection("user-123", "session-abc", nil)
	assert.Equal(t, "user-123", userConn.UserID)
	assert.Equal(t, []string{"user"}, userConn.Roles)

	// Create an admin connection
	adminConn := testutil.MockConnection("admin-456", "session-xyz", []string{"admin", "user"})
	assert.Equal(t, "admin-456", adminConn.UserID)
	assert.Equal(t, []string{"admin", "user"}, adminConn.Roles)
}

// Example 8: Reusing mocks with Reset()
func TestExample_MockReset_reusingMocks(t *testing.T) {
	mock := &testutil.MockStorageService{}

	// First test
	sess1 := testutil.CreateTestSession("user-1", "session-1")
	mock.CreateSession(sess1)
	assert.True(t, mock.CreateSessionCalled)
	assert.Len(t, mock.CreatedSessions, 1)

	// Reset for next test
	mock.Reset()
	assert.False(t, mock.CreateSessionCalled)
	assert.Len(t, mock.CreatedSessions, 0)

	// Second test
	sess2 := testutil.CreateTestSession("user-2", "session-2")
	mock.CreateSession(sess2)
	assert.True(t, mock.CreateSessionCalled)
	assert.Len(t, mock.CreatedSessions, 1)
}

// Run these examples as tests
func TestAllExamples(t *testing.T) {
	t.Run("Example 1: Session Creation", TestExample_MockStorageService_sessionCreation)
	t.Run("Example 2: Error Injection", TestExample_MockStorageService_errorInjection)
	t.Run("Example 3: Memory Growth", TestExample_MemoryGrowth_sessionAccumulation)
	t.Run("Example 4: Goroutine Count", TestExample_GoroutineCount_connectionCleanup)
	t.Run("Example 5: Data Race", TestExample_DataRace_concurrentSessionAccess)
	t.Run("Example 6: LLM Streaming", TestExample_MockLLMService_streaming)
	t.Run("Example 7: Mock Connection", TestExample_MockConnection_creation)
	t.Run("Example 8: Mock Reset", TestExample_MockReset_reusingMocks)
}
