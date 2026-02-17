package router

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/llm"
	"github.com/real-rm/chatbox/internal/message"
	"github.com/real-rm/chatbox/internal/session"
	"github.com/real-rm/chatbox/internal/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingMockLLMService simulates streaming responses with multiple chunks
type streamingMockLLMService struct {
	chunks []string
	err    error
}

func (m *streamingMockLLMService) SendMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (*llm.LLMResponse, error) {
	return nil, nil
}

func (m *streamingMockLLMService) StreamMessage(ctx context.Context, modelID string, messages []llm.ChatMessage) (<-chan *llm.LLMChunk, error) {
	if m.err != nil {
		return nil, m.err
	}

	ch := make(chan *llm.LLMChunk, len(m.chunks))
	go func() {
		defer close(ch)
		for i, content := range m.chunks {
			ch <- &llm.LLMChunk{
				Content: content,
				Done:    i == len(m.chunks)-1,
			}
			// Small delay to simulate real streaming
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return ch, nil
}

// TestStreamingResponseForwarding verifies that LLM response chunks are forwarded to the client
func TestStreamingResponseForwarding(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	tests := []struct {
		name           string
		chunks         []string
		expectedChunks int
	}{
		{
			name:           "single chunk",
			chunks:         []string{"Hello, world!"},
			expectedChunks: 1,
		},
		{
			name:           "multiple chunks",
			chunks:         []string{"Hello", ", ", "world", "!"},
			expectedChunks: 4,
		},
		{
			name:           "empty chunk in middle",
			chunks:         []string{"Hello", "", "world"},
			expectedChunks: 2, // Empty chunks should not be sent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLLM := &streamingMockLLMService{chunks: tt.chunks}
			router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

			// Create and register connection
			conn := websocket.NewConnection("user-1", []string{"user"})
			conn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, conn)
			require.NoError(t, err)

			// Create user message
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sess.ID,
				Content:   "Test message",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			// Handle the message in a goroutine
			go func() {
				err := router.HandleUserMessage(conn, msg)
				assert.NoError(t, err)
			}()

			// Collect messages from the connection's send channel
			var receivedMessages []*message.Message
			timeout := time.After(2 * time.Second)
			expectedMessages := tt.expectedChunks + 1 // +1 for loading indicator

		collectLoop:
			for len(receivedMessages) < expectedMessages {
				select {
				case data := <-conn.ReceiveForTest():
					var msg message.Message
					err := json.Unmarshal(data, &msg)
					require.NoError(t, err)
					receivedMessages = append(receivedMessages, &msg)
				case <-timeout:
					break collectLoop
				}
			}

			// Verify we received the expected number of messages
			assert.GreaterOrEqual(t, len(receivedMessages), expectedMessages,
				"Should receive at least loading indicator + chunks")

			// First message should be loading indicator
			assert.Equal(t, message.TypeLoading, receivedMessages[0].Type)

			// Subsequent messages should be AI responses
			chunkCount := 0
			var fullContent string
			for i := 1; i < len(receivedMessages); i++ {
				msg := receivedMessages[i]
				if msg.Type == message.TypeAIResponse {
					chunkCount++
					fullContent += msg.Content
					
					// Verify metadata
					assert.Equal(t, "true", msg.Metadata["streaming"])
					assert.NotEmpty(t, msg.Metadata["done"])
					
					// Last chunk should have done=true
					if i == len(receivedMessages)-1 {
						assert.Equal(t, "true", msg.Metadata["done"])
					}
				}
			}

			assert.Equal(t, tt.expectedChunks, chunkCount, "Should receive expected number of chunks")

			// Verify full content matches
			expectedContent := ""
			for _, chunk := range tt.chunks {
				if chunk != "" {
					expectedContent += chunk
				}
			}
			assert.Equal(t, expectedContent, fullContent, "Full content should match")
		})
	}
}

// TestStreamingErrorHandling verifies error handling during streaming
func TestStreamingErrorHandling(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	mockLLM := &streamingMockLLMService{
		err: assert.AnError,
	}
	router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

	// Create and register connection
	conn := websocket.NewConnection("user-1", []string{"user"})
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Create user message
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Test message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Handle the message
	err = router.HandleUserMessage(conn, msg)
	assert.NoError(t, err) // Error is sent to client, not returned

	// Collect messages from the connection's send channel
	timeout := time.After(1 * time.Second)
	var receivedMessages []*message.Message

collectLoop:
	for len(receivedMessages) < 2 {
		select {
		case data := <-conn.ReceiveForTest():
			var msg message.Message
			err := json.Unmarshal(data, &msg)
			require.NoError(t, err)
			receivedMessages = append(receivedMessages, &msg)
		case <-timeout:
			break collectLoop
		}
	}

	// Should receive loading indicator and error message
	assert.GreaterOrEqual(t, len(receivedMessages), 2)
	assert.Equal(t, message.TypeLoading, receivedMessages[0].Type)
	assert.Equal(t, message.TypeError, receivedMessages[1].Type)
	assert.NotNil(t, receivedMessages[1].Error)
	assert.Equal(t, "LLM_UNAVAILABLE", receivedMessages[1].Error.Code)
	assert.Equal(t, "AI service is temporarily unavailable", receivedMessages[1].Error.Message)
	assert.True(t, receivedMessages[1].Error.Recoverable)
}

// TestLLMErrorScenarios tests various LLM failure scenarios
func TestLLMErrorScenarios(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	tests := []struct {
		name              string
		llmError          error
		expectedErrorCode string
		expectedMessage   string
	}{
		{
			name:              "network timeout",
			llmError:          assert.AnError,
			expectedErrorCode: "LLM_UNAVAILABLE",
			expectedMessage:   "AI service is temporarily unavailable",
		},
		{
			name:              "service unavailable",
			llmError:          assert.AnError,
			expectedErrorCode: "LLM_UNAVAILABLE",
			expectedMessage:   "AI service is temporarily unavailable",
		},
		{
			name:              "rate limit exceeded",
			llmError:          assert.AnError,
			expectedErrorCode: "LLM_UNAVAILABLE",
			expectedMessage:   "AI service is temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a session with unique user ID for each test
			userID := fmt.Sprintf("user-%s", tt.name)
			sess, err := sm.CreateSession(userID)
			require.NoError(t, err)

			mockLLM := &streamingMockLLMService{
				err: tt.llmError,
			}
			router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

			// Create and register connection
			conn := websocket.NewConnection(userID, []string{"user"})
			conn.SessionID = sess.ID
			err = router.RegisterConnection(sess.ID, conn)
			require.NoError(t, err)

			// Create user message
			msg := &message.Message{
				Type:      message.TypeUserMessage,
				SessionID: sess.ID,
				Content:   "Test message",
				Sender:    message.SenderUser,
				Timestamp: time.Now(),
			}

			// Handle the message
			err = router.HandleUserMessage(conn, msg)
			assert.NoError(t, err) // Error is sent to client, not returned

			// Collect messages from the connection's send channel
			timeout := time.After(1 * time.Second)
			var receivedMessages []*message.Message

		collectLoop:
			for len(receivedMessages) < 2 {
				select {
				case data := <-conn.ReceiveForTest():
					var msg message.Message
					err := json.Unmarshal(data, &msg)
					require.NoError(t, err)
					receivedMessages = append(receivedMessages, &msg)
				case <-timeout:
					break collectLoop
				}
			}

			// Verify error message
			require.GreaterOrEqual(t, len(receivedMessages), 2)
			errorMsg := receivedMessages[1]
			assert.Equal(t, message.TypeError, errorMsg.Type)
			assert.NotNil(t, errorMsg.Error)
			assert.Equal(t, tt.expectedErrorCode, errorMsg.Error.Code)
			assert.Equal(t, tt.expectedMessage, errorMsg.Error.Message)
			assert.True(t, errorMsg.Error.Recoverable, "LLM errors should be recoverable")
		})
	}
}

// TestStreamingChunkError tests error handling when streaming fails mid-stream
func TestStreamingChunkError(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Mock LLM that sends some chunks then closes channel (simulating connection drop)
	mockLLM := &streamingMockLLMService{
		chunks: []string{"Hello", " world"},
	}
	router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

	// Create and register connection
	conn := websocket.NewConnection("user-1", []string{"user"})
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Create user message
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Test message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Handle the message
	go func() {
		err := router.HandleUserMessage(conn, msg)
		assert.NoError(t, err)
	}()

	// Collect messages
	timeout := time.After(2 * time.Second)
	var receivedMessages []*message.Message

collectLoop:
	for {
		select {
		case data := <-conn.ReceiveForTest():
			var msg message.Message
			err := json.Unmarshal(data, &msg)
			require.NoError(t, err)
			receivedMessages = append(receivedMessages, &msg)
			
			// Check if we got the final chunk
			if msg.Type == message.TypeAIResponse && msg.Metadata["done"] == "true" {
				break collectLoop
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Should receive loading indicator and at least some chunks
	assert.GreaterOrEqual(t, len(receivedMessages), 2)
	assert.Equal(t, message.TypeLoading, receivedMessages[0].Type)
	
	// Verify we got some content
	var fullContent string
	for i := 1; i < len(receivedMessages); i++ {
		if receivedMessages[i].Type == message.TypeAIResponse {
			fullContent += receivedMessages[i].Content
		}
	}
	assert.NotEmpty(t, fullContent, "Should receive some content before stream ends")
}

// TestLLMErrorDoesNotLeakInternalDetails verifies that error messages don't expose internal details
func TestLLMErrorDoesNotLeakInternalDetails(t *testing.T) {
	logger := createTestLogger()
	sm := session.NewSessionManager(15*time.Minute, logger)

	// Create a session
	sess, err := sm.CreateSession("user-1")
	require.NoError(t, err)

	// Mock LLM with detailed internal error
	mockLLM := &streamingMockLLMService{
		err: fmt.Errorf("internal error: database connection failed at 192.168.1.100:5432 with credentials user=admin"),
	}
	router := NewMessageRouter(sm, mockLLM, nil, nil, nil, 120*time.Second, logger)

	// Create and register connection
	conn := websocket.NewConnection("user-1", []string{"user"})
	conn.SessionID = sess.ID
	err = router.RegisterConnection(sess.ID, conn)
	require.NoError(t, err)

	// Create user message
	msg := &message.Message{
		Type:      message.TypeUserMessage,
		SessionID: sess.ID,
		Content:   "Test message",
		Sender:    message.SenderUser,
		Timestamp: time.Now(),
	}

	// Handle the message
	err = router.HandleUserMessage(conn, msg)
	assert.NoError(t, err)

	// Collect error message
	timeout := time.After(1 * time.Second)
	var errorMsg *message.Message

collectLoop:
	for {
		select {
		case data := <-conn.ReceiveForTest():
			var msg message.Message
			err := json.Unmarshal(data, &msg)
			require.NoError(t, err)
			if msg.Type == message.TypeError {
				errorMsg = &msg
				break collectLoop
			}
		case <-timeout:
			break collectLoop
		}
	}

	// Verify error message doesn't contain internal details
	require.NotNil(t, errorMsg)
	require.NotNil(t, errorMsg.Error)
	
	// Error message should be generic
	assert.Equal(t, "AI service is temporarily unavailable", errorMsg.Error.Message)
	
	// Should not contain internal details
	assert.NotContains(t, errorMsg.Error.Message, "database")
	assert.NotContains(t, errorMsg.Error.Message, "192.168")
	assert.NotContains(t, errorMsg.Error.Message, "credentials")
	assert.NotContains(t, errorMsg.Error.Message, "admin")
}
