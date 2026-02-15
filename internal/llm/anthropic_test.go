package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicProvider_SendMessage(t *testing.T) {
	tests := []struct {
		name           string
		messages       []ChatMessage
		mockResponse   anthropicResponse
		mockStatusCode int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful request",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: anthropicResponse{
				ID:   "msg_123",
				Type: "message",
				Role: "assistant",
				Content: []anthropicContent{
					{Type: "text", Text: "Hello! How can I help you?"},
				},
				Model: "claude-3-opus-20240229",
				Usage: anthropicUsage{
					InputTokens:  10,
					OutputTokens: 15,
				},
			},
			mockStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name: "API error response",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockStatusCode: http.StatusTooManyRequests,
			wantErr:        true,
			errContains:    "Anthropic API error",
		},
		{
			name: "empty content",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: anthropicResponse{
				ID:      "msg_123",
				Content: []anthropicContent{},
			},
			mockStatusCode: http.StatusOK,
			wantErr:        true,
			errContains:    "no content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.NotEmpty(t, r.Header.Get("x-api-key"))
				assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
				
				// Verify request body
				var reqBody anthropicRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, len(tt.messages), len(reqBody.Messages))
				assert.False(t, reqBody.Stream)
				assert.Greater(t, reqBody.MaxTokens, 0)
				
				// Send response
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusOK {
					json.NewEncoder(w).Encode(tt.mockResponse)
				} else {
					w.Write([]byte(`{"error": {"message": "rate limit exceeded"}}`))
				}
			}))
			defer server.Close()

			provider := NewAnthropicProvider("test-key", server.URL, "claude-3-opus-20240229")
			
			req := &LLMRequest{
				ModelID:  "claude-3-opus-20240229",
				Messages: tt.messages,
				Stream:   false,
			}
			
			ctx := context.Background()
			resp, err := provider.SendMessage(ctx, req)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				assert.Equal(t, tt.mockResponse.Content[0].Text, resp.Content)
				expectedTokens := tt.mockResponse.Usage.InputTokens + tt.mockResponse.Usage.OutputTokens
				assert.Equal(t, expectedTokens, resp.TokensUsed)
				assert.Greater(t, resp.Duration, time.Duration(0))
			}
		})
	}
}

func TestAnthropicProvider_StreamMessage(t *testing.T) {
	tests := []struct {
		name           string
		messages       []ChatMessage
		streamData     []string
		mockStatusCode int
		wantChunks     int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful streaming",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			streamData: []string{
				`event: message_start`,
				`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant"}}`,
				``,
				`event: content_block_start`,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
				``,
				`event: content_block_delta`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}`,
				``,
				`event: message_stop`,
				`data: {"type":"message_stop"}`,
			},
			mockStatusCode: http.StatusOK,
			wantChunks:     4, // 3 content deltas + 1 stop
			wantErr:        false,
		},
		{
			name: "API error response",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Anthropic API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.NotEmpty(t, r.Header.Get("x-api-key"))
				assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
				
				// Verify request body
				var reqBody anthropicRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.True(t, reqBody.Stream)
				
				// Send response
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusOK {
					w.Header().Set("Content-Type", "text/event-stream")
					for _, data := range tt.streamData {
						w.Write([]byte(data + "\n"))
					}
				} else {
					w.Write([]byte(`{"error": {"message": "unauthorized"}}`))
				}
			}))
			defer server.Close()

			provider := NewAnthropicProvider("test-key", server.URL, "claude-3-opus-20240229")
			
			req := &LLMRequest{
				ModelID:  "claude-3-opus-20240229",
				Messages: tt.messages,
				Stream:   true,
			}
			
			ctx := context.Background()
			chunkChan, err := provider.StreamMessage(ctx, req)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, chunkChan)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, chunkChan)
				
				// Collect chunks
				chunks := []LLMChunk{}
				for chunk := range chunkChan {
					chunks = append(chunks, *chunk)
				}
				
				assert.Equal(t, tt.wantChunks, len(chunks))
				// Last chunk should be marked as done
				if len(chunks) > 0 {
					assert.True(t, chunks[len(chunks)-1].Done)
				}
			}
		})
	}
}

func TestAnthropicProvider_GetTokenCount(t *testing.T) {
	provider := NewAnthropicProvider("test-key", "https://api.anthropic.com/v1", "claude-3-opus-20240229")
	
	tests := []struct {
		name     string
		text     string
		wantMin  int
		wantMax  int
	}{
		{
			name:    "short text",
			text:    "Hello",
			wantMin: 1,
			wantMax: 2,
		},
		{
			name:    "medium text",
			text:    "Hello, how are you doing today?",
			wantMin: 7,
			wantMax: 9,
		},
		{
			name:    "empty text",
			text:    "",
			wantMin: 0,
			wantMax: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := provider.GetTokenCount(tt.text)
			assert.GreaterOrEqual(t, count, tt.wantMin)
			assert.LessOrEqual(t, count, tt.wantMax)
		})
	}
}

func TestAnthropicProvider_SystemMessageHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		
		// Verify system message was converted to user message
		for _, msg := range reqBody.Messages {
			assert.NotEqual(t, "system", msg.Role)
		}
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContent{{Type: "text", Text: "Response"}},
			Usage:   anthropicUsage{InputTokens: 10, OutputTokens: 5},
		})
	}))
	defer server.Close()

	provider := NewAnthropicProvider("test-key", server.URL, "claude-3-opus-20240229")
	
	req := &LLMRequest{
		ModelID: "claude-3-opus-20240229",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant"},
			{Role: "user", Content: "Hello"},
		},
	}
	
	ctx := context.Background()
	resp, err := provider.SendMessage(ctx, req)
	
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}
