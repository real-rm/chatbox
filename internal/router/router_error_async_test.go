package router

import (
	"context"
	"testing"
	"time"

	chaterrors "github.com/real-rm/chatbox/internal/errors"
	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/real-rm/golog"
	"github.com/stretchr/testify/assert"
)

// mockLLMForAsync is a minimal mock for the async error test
type mockLLMForAsync struct{}

func (m *mockLLMForAsync) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{Content: "ok", TokensUsed: 1, Duration: time.Millisecond}, nil
}

func (m *mockLLMForAsync) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	ch := make(chan *llm.LLMChunk, 1)
	ch <- &llm.LLMChunk{Done: true}
	close(ch)
	return ch, nil
}

// mockStorageForAsync is a minimal mock
type mockStorageForAsync struct{}

func (m *mockStorageForAsync) CreateSession(sess *session.Session) error { return nil }

func TestHandleChatError_FatalDoesNotBlock(t *testing.T) {
	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            t.TempDir(),
		Level:          "error",
		StandardOutput: false,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	sm := session.NewSessionManager(15*time.Minute, logger)
	router := NewMessageRouter(sm, &mockLLMForAsync{}, nil, nil, &mockStorageForAsync{}, 0, logger)

	// Create a session and register a connection
	sess, err := sm.CreateSession("user-1")
	assert.NoError(t, err)

	conn := websocket.NewConnection("user-1", []string{"user"})
	err = router.RegisterConnection(sess.ID, conn)
	assert.NoError(t, err)

	// Create a fatal (non-recoverable) error
	fatalErr := &chaterrors.ChatError{
		Code:        chaterrors.ErrCodeServiceError,
		Message:     "fatal test error",
		Category:    chaterrors.CategoryService,
		Recoverable: false,
	}

	// handleChatError with a fatal error should return quickly (not block for 100ms+)
	start := time.Now()
	err = router.HandleError(sess.ID, fatalErr)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	// The function should return within ~10ms, not wait for the sleep+close
	assert.Less(t, elapsed, 50*time.Millisecond,
		"HandleError should not block on fatal error close; got %v", elapsed)
}
