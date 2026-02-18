// Package testutil provides common test helpers and mock implementations
// for production readiness verification tests.
package testutil

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
)

// MockStorageService is a mock implementation of StorageService for testing.
// It tracks method calls and allows configurable behavior for testing various scenarios.
type MockStorageService struct {
	mu sync.Mutex

	// CreateSession tracking
	CreateSessionFunc   func(*session.Session) error
	CreateSessionCalled bool
	CreatedSessions     []*session.Session

	// UpdateSession tracking
	UpdateSessionFunc   func(*session.Session) error
	UpdateSessionCalled bool
	UpdatedSessions     []*session.Session

	// GetSession tracking
	GetSessionFunc   func(string) (*session.Session, error)
	GetSessionCalled bool
	GetSessionID     string

	// Error injection
	CreateSessionError error
	UpdateSessionError error
	GetSessionError    error
}

// CreateSession mocks the CreateSession method
func (m *MockStorageService) CreateSession(sess *session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateSessionCalled = true
	if m.CreateSessionError != nil {
		return m.CreateSessionError
	}
	if m.CreateSessionFunc != nil {
		return m.CreateSessionFunc(sess)
	}
	m.CreatedSessions = append(m.CreatedSessions, sess)
	return nil
}

// UpdateSession mocks the UpdateSession method
func (m *MockStorageService) UpdateSession(sess *session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateSessionCalled = true
	if m.UpdateSessionError != nil {
		return m.UpdateSessionError
	}
	if m.UpdateSessionFunc != nil {
		return m.UpdateSessionFunc(sess)
	}
	m.UpdatedSessions = append(m.UpdatedSessions, sess)
	return nil
}

// GetSession mocks the GetSession method
func (m *MockStorageService) GetSession(sessionID string) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetSessionCalled = true
	m.GetSessionID = sessionID
	if m.GetSessionError != nil {
		return nil, m.GetSessionError
	}
	if m.GetSessionFunc != nil {
		return m.GetSessionFunc(sessionID)
	}
	// Return a default session if no custom function is provided
	return &session.Session{
		ID:       sessionID,
		UserID:   "test-user",
		IsActive: true,
	}, nil
}

// Reset clears all tracking data
func (m *MockStorageService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateSessionCalled = false
	m.UpdateSessionCalled = false
	m.GetSessionCalled = false
	m.CreatedSessions = nil
	m.UpdatedSessions = nil
	m.GetSessionID = ""
	m.CreateSessionError = nil
	m.UpdateSessionError = nil
	m.GetSessionError = nil
}

// MockLLMService is a mock implementation of LLMService for testing.
// It tracks method calls and allows configurable behavior for testing various scenarios.
type MockLLMService struct {
	mu sync.Mutex

	// StreamMessage tracking
	StreamMessageFunc   func(context.Context, string, []llm.ChatMessage) (<-chan *llm.LLMChunk, error)
	StreamMessageCalled bool
	StreamCallCount     int
	LastStreamContext   context.Context
	LastStreamModelID   string
	LastStreamMessages  []llm.ChatMessage

	// SendMessage tracking
	SendMessageFunc   func(context.Context, string, []llm.ChatMessage) (*llm.LLMResponse, error)
	SendMessageCalled bool
	SendCallCount     int
	LastSendContext   context.Context
	LastSendModelID   string
	LastSendMessages  []llm.ChatMessage

	// Error injection
	StreamError error
	SendError   error
}

// StreamMessage mocks the StreamMessage method
func (m *MockLLMService) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	m.mu.Lock()
	m.StreamMessageCalled = true
	m.StreamCallCount++
	m.LastStreamContext = ctx
	m.LastStreamModelID = modelID
	m.LastStreamMessages = messages
	m.mu.Unlock()

	if m.StreamError != nil {
		return nil, m.StreamError
	}
	if m.StreamMessageFunc != nil {
		return m.StreamMessageFunc(ctx, modelID, messages)
	}

	// Default behavior: return a channel with a single chunk
	ch := make(chan *llm.LLMChunk, 1)
	ch <- &llm.LLMChunk{
		Content: "Mock streaming response",
		Done:    true,
	}
	close(ch)
	return ch, nil
}

// SendMessage mocks the SendMessage method
func (m *MockLLMService) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	m.mu.Lock()
	m.SendMessageCalled = true
	m.SendCallCount++
	m.LastSendContext = ctx
	m.LastSendModelID = modelID
	m.LastSendMessages = messages
	m.mu.Unlock()

	if m.SendError != nil {
		return nil, m.SendError
	}
	if m.SendMessageFunc != nil {
		return m.SendMessageFunc(ctx, modelID, messages)
	}

	// Default behavior: return a mock response
	return &llm.LLMResponse{
		Content:    "Mock response",
		TokensUsed: 10,
		Duration:   100 * time.Millisecond,
	}, nil
}

// Reset clears all tracking data
func (m *MockLLMService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.StreamMessageCalled = false
	m.StreamCallCount = 0
	m.SendMessageCalled = false
	m.SendCallCount = 0
	m.LastStreamContext = nil
	m.LastStreamModelID = ""
	m.LastStreamMessages = nil
	m.LastSendContext = nil
	m.LastSendModelID = ""
	m.LastSendMessages = nil
	m.StreamError = nil
	m.SendError = nil
}

// MockConnection creates a mock WebSocket connection for testing
func MockConnection(userID string, sessionID string, roles []string) *websocket.Connection {
	if roles == nil {
		roles = []string{"user"}
	}
	conn := websocket.NewConnection(userID, roles)
	conn.SessionID = sessionID
	return conn
}

// CreateTestSession creates a session with test data for testing
func CreateTestSession(userID string, sessionID string) *session.Session {
	if sessionID == "" {
		sessionID = "test-session-" + userID
	}
	return &session.Session{
		ID:            sessionID,
		UserID:        userID,
		Messages:      []*session.Message{},
		StartTime:     time.Now(),
		IsActive:      true,
		TotalTokens:   0,
		ResponseTimes: []time.Duration{},
	}
}

// CreateTestSessionWithMessages creates a session with predefined messages
func CreateTestSessionWithMessages(userID string, sessionID string, messageCount int) *session.Session {
	sess := CreateTestSession(userID, sessionID)
	for i := 0; i < messageCount; i++ {
		sess.Messages = append(sess.Messages, &session.Message{
			Content:   "Test message content",
			Timestamp: time.Now(),
			Sender:    "user",
		})
	}
	return sess
}

// CreateTestLogger creates a logger for testing that writes to a temporary directory
func CreateTestLogger(t *testing.T) *golog.Logger {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}
	return logger
}

// AssertNoDataRace is a helper that documents the need to run tests with -race flag.
// It doesn't actually detect races itself, but serves as documentation.
func AssertNoDataRace(t *testing.T, description string) {
	t.Helper()
	t.Logf("Data race check: %s", description)
	t.Log("NOTE: Run with 'go test -race' to detect data races")
}

// AssertMemoryGrowth measures and reports memory growth between two points
func AssertMemoryGrowth(t *testing.T, before, after runtime.MemStats, description string) {
	t.Helper()
	growth := int64(after.Alloc) - int64(before.Alloc)
	growthMB := float64(growth) / (1024 * 1024)

	t.Logf("Memory growth (%s): %d bytes (%.2f MB)", description, growth, growthMB)

	if growth > 0 {
		t.Logf("  Alloc: %d → %d", before.Alloc, after.Alloc)
		t.Logf("  TotalAlloc: %d → %d", before.TotalAlloc, after.TotalAlloc)
		t.Logf("  HeapAlloc: %d → %d", before.HeapAlloc, after.HeapAlloc)
	}
}

// AssertGoroutineCount measures and reports goroutine count changes
func AssertGoroutineCount(t *testing.T, before, after int, description string) {
	t.Helper()
	delta := after - before

	t.Logf("Goroutine count (%s): %d → %d (delta: %d)", description, before, after, delta)

	// Allow for small variations due to test framework and GC
	tolerance := 5
	if delta > tolerance {
		t.Logf("  ⚠ WARNING: Goroutine count increased by %d (tolerance: %d)", delta, tolerance)
		t.Logf("  This may indicate a goroutine leak")
	} else if delta < -tolerance {
		t.Logf("  ✓ Goroutine count decreased by %d (cleanup occurred)", -delta)
	} else {
		t.Logf("  ✓ Goroutine count stable (within tolerance of ±%d)", tolerance)
	}

	assert.InDelta(t, before, after, float64(tolerance),
		"Goroutine count should not increase significantly")
}

// MeasureMemory captures current memory statistics
func MeasureMemory() runtime.MemStats {
	runtime.GC() // Force GC to get accurate baseline
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// MeasureGoroutines returns the current goroutine count
func MeasureGoroutines() int {
	return runtime.NumGoroutine()
}

// WaitForGoroutines waits for goroutines to stabilize
func WaitForGoroutines() {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
}
