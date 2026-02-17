package testutil

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockStorageService verifies MockStorageService functionality
func TestMockStorageService(t *testing.T) {
	t.Run("CreateSession tracking", func(t *testing.T) {
		mock := &MockStorageService{}
		sess := CreateTestSession("user-1", "session-1")

		err := mock.CreateSession(sess)
		require.NoError(t, err)

		assert.True(t, mock.CreateSessionCalled)
		assert.Len(t, mock.CreatedSessions, 1)
		assert.Equal(t, sess, mock.CreatedSessions[0])
	})

	t.Run("CreateSession with error", func(t *testing.T) {
		mock := &MockStorageService{
			CreateSessionError: assert.AnError,
		}
		sess := CreateTestSession("user-1", "session-1")

		err := mock.CreateSession(sess)
		assert.Error(t, err)
		assert.True(t, mock.CreateSessionCalled)
		assert.Len(t, mock.CreatedSessions, 0)
	})

	t.Run("Reset clears tracking", func(t *testing.T) {
		mock := &MockStorageService{}
		sess := CreateTestSession("user-1", "session-1")

		mock.CreateSession(sess)
		assert.True(t, mock.CreateSessionCalled)

		mock.Reset()
		assert.False(t, mock.CreateSessionCalled)
		assert.Len(t, mock.CreatedSessions, 0)
	})
}

// TestMockLLMService verifies MockLLMService functionality
func TestMockLLMService(t *testing.T) {
	t.Run("StreamMessage tracking", func(t *testing.T) {
		mock := &MockLLMService{}
		ctx := context.Background()
		messages := []llm.ChatMessage{
			{Role: "user", Content: "test"},
		}

		ch, err := mock.StreamMessage(ctx, "gpt-4", messages)
		require.NoError(t, err)
		require.NotNil(t, ch)

		// Read from channel
		chunk := <-ch
		assert.Equal(t, "Mock streaming response", chunk.Content)
		assert.True(t, chunk.Done)

		assert.True(t, mock.StreamMessageCalled)
		assert.Equal(t, 1, mock.StreamCallCount)
		assert.Equal(t, "gpt-4", mock.LastStreamModelID)
	})

	t.Run("SendMessage tracking", func(t *testing.T) {
		mock := &MockLLMService{}
		ctx := context.Background()
		messages := []llm.ChatMessage{
			{Role: "user", Content: "test"},
		}

		resp, err := mock.SendMessage(ctx, "gpt-4", messages)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, "Mock response", resp.Content)
		assert.True(t, mock.SendMessageCalled)
		assert.Equal(t, 1, mock.SendCallCount)
		assert.Equal(t, "gpt-4", mock.LastSendModelID)
	})

	t.Run("Reset clears tracking", func(t *testing.T) {
		mock := &MockLLMService{}
		ctx := context.Background()
		messages := []llm.ChatMessage{
			{Role: "user", Content: "test"},
		}

		mock.StreamMessage(ctx, "gpt-4", messages)
		assert.True(t, mock.StreamMessageCalled)

		mock.Reset()
		assert.False(t, mock.StreamMessageCalled)
		assert.Equal(t, 0, mock.StreamCallCount)
	})
}

// TestMockConnection verifies MockConnection functionality
func TestMockConnection(t *testing.T) {
	t.Run("creates connection with defaults", func(t *testing.T) {
		conn := MockConnection("user-1", "session-1", nil)
		require.NotNil(t, conn)

		assert.Equal(t, "user-1", conn.UserID)
		assert.Equal(t, "session-1", conn.SessionID)
		assert.Equal(t, []string{"user"}, conn.Roles)
	})

	t.Run("creates connection with custom roles", func(t *testing.T) {
		conn := MockConnection("admin-1", "session-1", []string{"admin", "user"})
		require.NotNil(t, conn)

		assert.Equal(t, "admin-1", conn.UserID)
		assert.Equal(t, []string{"admin", "user"}, conn.Roles)
	})
}

// TestCreateTestSession verifies CreateTestSession functionality
func TestCreateTestSession(t *testing.T) {
	t.Run("creates session with defaults", func(t *testing.T) {
		sess := CreateTestSession("user-1", "")
		require.NotNil(t, sess)

		assert.Equal(t, "test-session-user-1", sess.ID)
		assert.Equal(t, "user-1", sess.UserID)
		assert.True(t, sess.IsActive)
		assert.NotNil(t, sess.Messages)
		assert.NotNil(t, sess.ResponseTimes)
	})

	t.Run("creates session with custom ID", func(t *testing.T) {
		sess := CreateTestSession("user-1", "custom-session")
		require.NotNil(t, sess)

		assert.Equal(t, "custom-session", sess.ID)
		assert.Equal(t, "user-1", sess.UserID)
	})
}

// TestCreateTestSessionWithMessages verifies CreateTestSessionWithMessages functionality
func TestCreateTestSessionWithMessages(t *testing.T) {
	sess := CreateTestSessionWithMessages("user-1", "session-1", 5)
	require.NotNil(t, sess)

	assert.Equal(t, "session-1", sess.ID)
	assert.Equal(t, "user-1", sess.UserID)
	assert.Len(t, sess.Messages, 5)

	for _, msg := range sess.Messages {
		assert.Equal(t, "Test message content", msg.Content)
		assert.Equal(t, "user", msg.Sender)
	}
}

// TestAssertMemoryGrowth verifies AssertMemoryGrowth functionality
func TestAssertMemoryGrowth(t *testing.T) {
	before := MeasureMemory()

	// Allocate some memory
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	after := MeasureMemory()

	// This should log memory growth information
	AssertMemoryGrowth(t, before, after, "test allocation")
}

// TestAssertGoroutineCount verifies AssertGoroutineCount functionality
func TestAssertGoroutineCount(t *testing.T) {
	before := MeasureGoroutines()

	// Launch a goroutine that completes quickly
	done := make(chan bool)
	go func() {
		time.Sleep(10 * time.Millisecond)
		done <- true
	}()
	<-done

	// Wait for goroutine to exit
	WaitForGoroutines()

	after := MeasureGoroutines()

	// This should log goroutine count information
	AssertGoroutineCount(t, before, after, "test goroutine")
}

// TestAssertNoDataRace verifies AssertNoDataRace functionality
func TestAssertNoDataRace(t *testing.T) {
	// This just logs a message
	AssertNoDataRace(t, "test data race check")
}

// TestMeasureMemory verifies MeasureMemory functionality
func TestMeasureMemory(t *testing.T) {
	mem := MeasureMemory()
	assert.Greater(t, mem.Alloc, uint64(0))
	assert.Greater(t, mem.TotalAlloc, uint64(0))
}

// TestMeasureGoroutines verifies MeasureGoroutines functionality
func TestMeasureGoroutines(t *testing.T) {
	count := MeasureGoroutines()
	assert.Greater(t, count, 0)
}

// TestWaitForGoroutines verifies WaitForGoroutines functionality
func TestWaitForGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	WaitForGoroutines()
	after := runtime.NumGoroutine()

	// Should not crash and goroutine count should be stable
	assert.InDelta(t, before, after, 5)
}

// TestConcurrentMockAccess verifies thread-safety of mocks
func TestConcurrentMockAccess(t *testing.T) {
	t.Run("MockStorageService concurrent access", func(t *testing.T) {
		mock := &MockStorageService{}
		done := make(chan bool)

		// Launch multiple goroutines
		for i := 0; i < 10; i++ {
			go func(id int) {
				sess := &session.Session{
					ID:     "session-" + string(rune(id)),
					UserID: "user-" + string(rune(id)),
				}
				mock.CreateSession(sess)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		assert.True(t, mock.CreateSessionCalled)
		assert.Equal(t, 10, len(mock.CreatedSessions))
	})

	t.Run("MockLLMService concurrent access", func(t *testing.T) {
		mock := &MockLLMService{}
		done := make(chan bool)
		ctx := context.Background()

		// Launch multiple goroutines
		for i := 0; i < 10; i++ {
			go func() {
				messages := []llm.ChatMessage{{Role: "user", Content: "test"}}
				mock.StreamMessage(ctx, "gpt-4", messages)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		assert.True(t, mock.StreamMessageCalled)
		assert.Equal(t, 10, mock.StreamCallCount)
	})
}
