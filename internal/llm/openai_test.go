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

func TestOpenAIProvider_SendMessage(t *testing.T) {
	tests := []struct {
		name           string
		messages       []ChatMessage
		mockResponse   openAIResponse
		mockStatusCode int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful request",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: openAIResponse{
				ID:      "chatcmpl-123",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "gpt-4",
				Choices: []openAIChoice{
					{
						Index: 0,
						Message: openAIMessage{
							Role:    "assistant",
							Content: "Hello! How can I help you?",
						},
						FinishReason: "stop",
					},
				},
				Usage: openAIUsage{
					PromptTokens:     10,
					CompletionTokens: 15,
					TotalTokens:      25,
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
			errContains:    "OpenAI API error",
		},
		{
			name: "empty choices",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: openAIResponse{
				ID:      "chatcmpl-123",
				Choices: []openAIChoice{},
			},
			mockStatusCode: http.StatusOK,
			wantErr:        true,
			errContains:    "no choices",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

				// Verify request body
				var reqBody openAIRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, len(tt.messages), len(reqBody.Messages))
				assert.False(t, reqBody.Stream)

				// Send response
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusOK {
					json.NewEncoder(w).Encode(tt.mockResponse)
				} else {
					w.Write([]byte(`{"error": "rate limit exceeded"}`))
				}
			}))
			defer server.Close()

			provider := NewOpenAIProvider("test-key", server.URL, "gpt-4")

			req := &LLMRequest{
				ModelID:  "gpt-4",
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
				assert.Equal(t, tt.mockResponse.Choices[0].Message.Content, resp.Content)
				assert.Equal(t, tt.mockResponse.Usage.TotalTokens, resp.TokensUsed)
				assert.Greater(t, resp.Duration, time.Duration(0))
			}
		})
	}
}

func TestOpenAIProvider_StreamMessage(t *testing.T) {
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
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			},
			mockStatusCode: http.StatusOK,
			wantChunks:     4, // 3 content chunks + 1 done chunk
			wantErr:        false,
		},
		{
			name: "API error response",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "OpenAI API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request headers
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer")
				assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

				// Verify request body
				var reqBody openAIRequest
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
					w.Write([]byte(`{"error": "unauthorized"}`))
				}
			}))
			defer server.Close()

			provider := NewOpenAIProvider("test-key", server.URL, "gpt-4")

			req := &LLMRequest{
				ModelID:  "gpt-4",
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

func TestOpenAIProvider_GetTokenCount(t *testing.T) {
	provider := NewOpenAIProvider("test-key", "https://api.openai.com/v1", "gpt-4")

	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
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

func TestOpenAIProvider_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(openAIResponse{
			Choices: []openAIChoice{{Message: openAIMessage{Content: "Response"}}},
		})
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-key", server.URL, "gpt-4")

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &LLMRequest{
		ModelID:  "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}

	_, err := provider.SendMessage(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}
