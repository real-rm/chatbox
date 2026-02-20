package testutil

import (
	"context"
	"errors"
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

	t.Run("CreateSession with custom func", func(t *testing.T) {
		customErr := assert.AnError
		mock := &MockStorageService{
			CreateSessionFunc: func(s *session.Session) error {
				return customErr
			},
		}
		sess := CreateTestSession("user-1", "session-1")
		err := mock.CreateSession(sess)
		assert.ErrorIs(t, err, customErr)
	})

	t.Run("UpdateSession tracking", func(t *testing.T) {
		mock := &MockStorageService{}
		sess := CreateTestSession("user-2", "session-2")

		err := mock.UpdateSession(sess)
		require.NoError(t, err)

		assert.True(t, mock.UpdateSessionCalled)
		assert.Len(t, mock.UpdatedSessions, 1)
		assert.Equal(t, sess, mock.UpdatedSessions[0])
	})

	t.Run("UpdateSession with error", func(t *testing.T) {
		mock := &MockStorageService{
			UpdateSessionError: assert.AnError,
		}
		sess := CreateTestSession("user-2", "session-2")

		err := mock.UpdateSession(sess)
		assert.Error(t, err)
		assert.True(t, mock.UpdateSessionCalled)
		assert.Len(t, mock.UpdatedSessions, 0)
	})

	t.Run("UpdateSession with custom func", func(t *testing.T) {
		called := false
		mock := &MockStorageService{
			UpdateSessionFunc: func(s *session.Session) error {
				called = true
				return nil
			},
		}
		sess := CreateTestSession("user-2", "session-2")
		err := mock.UpdateSession(sess)
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("GetSession tracking", func(t *testing.T) {
		mock := &MockStorageService{}

		retrieved, err := mock.GetSession("session-3")
		require.NoError(t, err)
		require.NotNil(t, retrieved)

		assert.True(t, mock.GetSessionCalled)
		assert.Equal(t, "session-3", mock.GetSessionID)
		assert.Equal(t, "session-3", retrieved.ID)
	})

	t.Run("GetSession with error", func(t *testing.T) {
		mock := &MockStorageService{
			GetSessionError: assert.AnError,
		}

		retrieved, err := mock.GetSession("session-4")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
		assert.True(t, mock.GetSessionCalled)
	})

	t.Run("GetSession with custom func", func(t *testing.T) {
		customSess := &session.Session{ID: "custom-id", UserID: "custom-user"}
		mock := &MockStorageService{
			GetSessionFunc: func(id string) (*session.Session, error) {
				return customSess, nil
			},
		}

		retrieved, err := mock.GetSession("any-id")
		require.NoError(t, err)
		assert.Equal(t, customSess, retrieved)
	})

	t.Run("Reset clears tracking", func(t *testing.T) {
		mock := &MockStorageService{}
		sess := CreateTestSession("user-1", "session-1")

		mock.CreateSession(sess)
		mock.UpdateSession(sess)
		mock.GetSession("session-1")

		assert.True(t, mock.CreateSessionCalled)
		assert.True(t, mock.UpdateSessionCalled)
		assert.True(t, mock.GetSessionCalled)

		mock.Reset()
		assert.False(t, mock.CreateSessionCalled)
		assert.False(t, mock.UpdateSessionCalled)
		assert.False(t, mock.GetSessionCalled)
		assert.Len(t, mock.CreatedSessions, 0)
		assert.Len(t, mock.UpdatedSessions, 0)
		assert.Empty(t, mock.GetSessionID)
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

	t.Run("StreamMessage error injection", func(t *testing.T) {
		mock := &MockLLMService{
			StreamError: errors.New("stream error"),
		}
		ctx := context.Background()
		messages := []llm.ChatMessage{{Role: "user", Content: "test"}}

		ch, err := mock.StreamMessage(ctx, "gpt-4", messages)
		assert.Error(t, err)
		assert.Nil(t, ch)
		assert.True(t, mock.StreamMessageCalled)
	})

	t.Run("StreamMessage custom func", func(t *testing.T) {
		customCh := make(chan *llm.LLMChunk, 1)
		customCh <- &llm.LLMChunk{Content: "custom", Done: true}
		close(customCh)

		mock := &MockLLMService{
			StreamMessageFunc: func(_ context.Context, _ string, _ []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
				return customCh, nil
			},
		}
		ctx := context.Background()
		messages := []llm.ChatMessage{{Role: "user", Content: "test"}}

		ch, err := mock.StreamMessage(ctx, "gpt-4", messages)
		require.NoError(t, err)
		chunk := <-ch
		assert.Equal(t, "custom", chunk.Content)
	})

	t.Run("SendMessage error injection", func(t *testing.T) {
		mock := &MockLLMService{
			SendError: errors.New("send error"),
		}
		ctx := context.Background()
		messages := []llm.ChatMessage{{Role: "user", Content: "test"}}

		resp, err := mock.SendMessage(ctx, "gpt-4", messages)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.True(t, mock.SendMessageCalled)
	})

	t.Run("SendMessage custom func", func(t *testing.T) {
		mock := &MockLLMService{
			SendMessageFunc: func(_ context.Context, _ string, _ []llm.ChatMessage) (*llm.LLMResponse, error) {
				return &llm.LLMResponse{Content: "custom-send"}, nil
			},
		}
		ctx := context.Background()
		messages := []llm.ChatMessage{{Role: "user", Content: "test"}}

		resp, err := mock.SendMessage(ctx, "gpt-4", messages)
		require.NoError(t, err)
		assert.Equal(t, "custom-send", resp.Content)
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

// TestAssertMemoryGrowth_NoGrowth covers the branch where memory growth is
// zero or negative (before.Alloc >= after.Alloc), so only the outer log line
// is executed and the inner "if growth > 0" block is skipped.
func TestAssertMemoryGrowth_NoGrowth(t *testing.T) {
	// Create stats where after.Alloc is less than before.Alloc to simulate
	// no growth (e.g., after GC). This exercises the false branch of "if growth > 0".
	var before, after runtime.MemStats
	before.Alloc = 1000
	before.TotalAlloc = 5000
	before.HeapAlloc = 900
	after.Alloc = 800 // Less than before — negative growth
	after.TotalAlloc = 5200
	after.HeapAlloc = 700

	// Must not panic; simply logs that there is no significant growth.
	AssertMemoryGrowth(t, before, after, "no growth scenario")
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
