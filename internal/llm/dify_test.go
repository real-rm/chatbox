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

func TestDifyProvider_SendMessage(t *testing.T) {
	tests := []struct {
		name           string
		messages       []ChatMessage
		mockResponse   difyResponse
		mockStatusCode int
		wantErr        bool
		errContains    string
	}{
		{
			name: "successful request",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: difyResponse{
				MessageID:      "msg_123",
				ConversationID: "conv_456",
				Mode:           "chat",
				Answer:         "Hello! How can I help you?",
				Metadata: difyMetadata{
					Usage: difyUsage{
						PromptTokens:     10,
						CompletionTokens: 15,
						TotalTokens:      25,
					},
				},
				CreatedAt: time.Now().Unix(),
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
			errContains:    "Dify API error",
		},
		{
			name: "empty answer",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockResponse: difyResponse{
				MessageID: "msg_123",
				Answer:    "",
			},
			mockStatusCode: http.StatusOK,
			wantErr:        true,
			errContains:    "no answer",
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
				var reqBody difyRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.NotEmpty(t, reqBody.Query)
				assert.Equal(t, "blocking", reqBody.ResponseMode)
				assert.NotEmpty(t, reqBody.User)
				
				// Send response
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusOK {
					json.NewEncoder(w).Encode(tt.mockResponse)
				} else {
					w.Write([]byte(`{"error": "rate limit exceeded"}`))
				}
			}))
			defer server.Close()

			provider := NewDifyProvider("test-key", server.URL, "dify-model")
			
			req := &LLMRequest{
				ModelID:  "dify-model",
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
				assert.Equal(t, tt.mockResponse.Answer, resp.Content)
				assert.Equal(t, tt.mockResponse.Metadata.Usage.TotalTokens, resp.TokensUsed)
				assert.Greater(t, resp.Duration, time.Duration(0))
			}
		})
	}
}

func TestDifyProvider_StreamMessage(t *testing.T) {
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
				`data: {"event":"message","message_id":"msg_123","conversation_id":"conv_456","answer":"Hello"}`,
				`data: {"event":"message","message_id":"msg_123","conversation_id":"conv_456","answer":" there"}`,
				`data: {"event":"message","message_id":"msg_123","conversation_id":"conv_456","answer":"!"}`,
				`data: {"event":"message_end","message_id":"msg_123","metadata":{"usage":{"total_tokens":25}}}`,
			},
			mockStatusCode: http.StatusOK,
			wantChunks:     4, // 3 message chunks + 1 end
			wantErr:        false,
		},
		{
			name: "streaming with error event",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			streamData: []string{
				`data: {"event":"message","answer":"Hello"}`,
				`data: {"event":"error","message":"Something went wrong"}`,
			},
			mockStatusCode: http.StatusOK,
			wantChunks:     2, // 1 message + 1 error (treated as done)
			wantErr:        false,
		},
		{
			name: "API error response",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			mockStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Dify API error",
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
				var reqBody difyRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, "streaming", reqBody.ResponseMode)
				
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

			provider := NewDifyProvider("test-key", server.URL, "dify-model")
			
			req := &LLMRequest{
				ModelID:  "dify-model",
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

func TestDifyProvider_GetTokenCount(t *testing.T) {
	provider := NewDifyProvider("test-key", "https://api.dify.ai/v1", "dify-model")
	
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

func TestDifyProvider_FormatMessages(t *testing.T) {
	provider := NewDifyProvider("test-key", "https://api.dify.ai/v1", "dify-model")
	
	tests := []struct {
		name     string
		messages []ChatMessage
		want     string
	}{
		{
			name: "single message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			want: "user: Hello",
		},
		{
			name: "multiple messages",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
				{Role: "user", Content: "How are you?"},
			},
			want: "user: Hello\nassistant: Hi there!\nuser: How are you?",
		},
		{
			name:     "empty messages",
			messages: []ChatMessage{},
			want:     "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.formatMessages(tt.messages)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestDifyProvider_MultipleMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody difyRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		
		// Verify query contains all messages formatted correctly
		assert.Contains(t, reqBody.Query, "user: Hello")
		assert.Contains(t, reqBody.Query, "assistant: Hi")
		
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(difyResponse{
			Answer: "Response",
			Metadata: difyMetadata{
				Usage: difyUsage{TotalTokens: 20},
			},
		})
	}))
	defer server.Close()

	provider := NewDifyProvider("test-key", server.URL, "dify-model")
	
	req := &LLMRequest{
		ModelID: "dify-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
			{Role: "user", Content: "How are you?"},
		},
	}
	
	ctx := context.Background()
	resp, err := provider.SendMessage(ctx, req)
	
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}
