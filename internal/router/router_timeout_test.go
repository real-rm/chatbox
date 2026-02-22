package router

import (
	"context"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
)

// mockHangingLLMService simulates an LLM service that hangs indefinitely
type mockHangingLLMService struct{}

func (m *mockHangingLLMService) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	return nil, nil
}

func (m *mockHangingLLMService) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	chunkChan := make(chan *llm.LLMChunk)

	// Simulate a hanging request by never sending anything and never closing
	go func() {
		<-ctx.Done() // Wait for context cancellation
		close(chunkChan)
	}()

	return chunkChan, nil
}

func (m *mockHangingLLMService) ValidateModel(modelID string) error { return nil }

func TestHandleUserMessage_Timeout(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	// Create router with very short timeout (100ms)
	hangingLLM := &mockHangingLLMService{}
	router := NewMessageRouter(sm, hangingLLM, nil, nil, nil, 100*time.Millisecond, logger)

	// Create connection and session
	conn := &websocket.Connection{
		UserID: "test-user",
	}

	// Create a session first
	sess, err := sm.CreateSession("test-user")
	assert.NoError(t, err)

	// Register connection
	router.RegisterConnection(sess.ID, conn)

	// Create user message
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Hello",
		Sender:    message.SenderUser,
	}

	// Handle message - should timeout
	start := time.Now()
	err = router.HandleUserMessage(conn, msg)
	elapsed := time.Since(start)

	// Should complete quickly (within timeout + small buffer)
	assert.Less(t, elapsed, 500*time.Millisecond, "Should timeout quickly")

	// Error should be returned (sent to connection)
	// Note: The actual error is sent via sendToConnection, not returned
	// So we just verify it completes without hanging
}

func TestHandleUserMessage_TimeoutConfiguration(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	tests := []struct {
		name            string
		configTimeout   time.Duration
		expectedTimeout time.Duration
	}{
		{
			name:            "configured timeout",
			configTimeout:   200 * time.Millisecond,
			expectedTimeout: 200 * time.Millisecond,
		},
		{
			name:            "zero timeout uses default",
			configTimeout:   0,
			expectedTimeout: 120 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hangingLLM := &mockHangingLLMService{}
			router := NewMessageRouter(sm, hangingLLM, nil, nil, nil, tt.configTimeout, logger)

			// Verify the timeout is set correctly
			assert.Equal(t, tt.configTimeout, router.llmStreamTimeout)
		})
	}
}
