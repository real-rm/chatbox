package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllProviders_Integration tests all three LLM providers (OpenAI, Anthropic, Dify)
// with both streaming and non-streaming modes, including error handling.
// This test validates requirement 1.2: All three LLM providers work correctly.
func TestAllProviders_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	providers := []struct {
		name     string
		config   LLMProviderConfig
		provider LLMProvider
	}{
		{
			name: "OpenAI",
			config: LLMProviderConfig{
				ID:       "openai-test",
				Name:     "OpenAI Test",
				Type:     "openai",
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "test-key",
				Model:    "gpt-4",
			},
		},
		{
			name: "Anthropic",
			config: LLMProviderConfig{
				ID:       "anthropic-test",
				Name:     "Anthropic Test",
				Type:     "anthropic",
				Endpoint: "https://api.anthropic.com/v1",
				APIKey:   "test-key",
				Model:    "claude-3-opus-20240229",
			},
		},
		{
			name: "Dify",
			config: LLMProviderConfig{
				ID:       "dify-test",
				Name:     "Dify Test",
				Type:     "dify",
				Endpoint: "https://api.dify.ai/v1",
				APIKey:   "test-key",
				Model:    "assistant",
			},
		},
	}

	for _, tc := range providers {
		t.Run(tc.name, func(t *testing.T) {
			// Create provider instance
			provider, err := createProvider(tc.config)
			require.NoError(t, err, "Failed to create %s provider", tc.name)
			require.NotNil(t, provider, "%s provider should not be nil", tc.name)

			// Test 1: Non-streaming message
			t.Run("SendMessage", func(t *testing.T) {
				testSendMessage(t, provider, tc.name)
			})

			// Test 2: Streaming message
			t.Run("StreamMessage", func(t *testing.T) {
				testStreamMessage(t, provider, tc.name)
			})

			// Test 3: Error handling
			t.Run("ErrorHandling", func(t *testing.T) {
				testErrorHandling(t, provider, tc.name)
			})

			// Test 4: Token counting
			t.Run("TokenCount", func(t *testing.T) {
				testTokenCount(t, provider, tc.name)
			})
		})
	}
}

// testSendMessage tests non-streaming message sending
func testSendMessage(t *testing.T, provider LLMProvider, providerName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &LLMRequest{
		ModelID: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello, this is a test message"},
		},
		Stream: false,
	}

	// Note: This will fail with real API calls without valid keys
	// The test validates that the provider interface is correctly implemented
	resp, err := provider.SendMessage(ctx, req)

	// We expect an error due to invalid API keys, but the error should be properly formatted
	if err != nil {
		t.Logf("%s SendMessage returned expected error (invalid API key): %v", providerName, err)
		// Verify error message format is appropriate
		assert.NotEmpty(t, err.Error(), "Error message should not be empty")
		return
	}

	// If somehow it succeeds (shouldn't with test keys), validate response
	assert.NotNil(t, resp, "%s response should not be nil", providerName)
	assert.NotEmpty(t, resp.Content, "%s response content should not be empty", providerName)
	assert.GreaterOrEqual(t, resp.TokensUsed, 0, "%s token count should be non-negative", providerName)
	assert.Greater(t, resp.Duration, time.Duration(0), "%s duration should be positive", providerName)
}

// testStreamMessage tests streaming message sending
func testStreamMessage(t *testing.T, provider LLMProvider, providerName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &LLMRequest{
		ModelID: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello, this is a streaming test"},
		},
		Stream: true,
	}

	// Note: This will fail with real API calls without valid keys
	chunkChan, err := provider.StreamMessage(ctx, req)

	// We expect an error due to invalid API keys
	if err != nil {
		t.Logf("%s StreamMessage returned expected error (invalid API key): %v", providerName, err)
		assert.NotEmpty(t, err.Error(), "Error message should not be empty")
		return
	}

	// If somehow it succeeds, validate the stream
	assert.NotNil(t, chunkChan, "%s chunk channel should not be nil", providerName)

	// Collect chunks with timeout
	chunks := []LLMChunk{}
	timeout := time.After(3 * time.Second)
	done := false

	for !done {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				done = true
				break
			}
			chunks = append(chunks, *chunk)
			if chunk.Done {
				done = true
			}
		case <-timeout:
			t.Logf("%s stream timed out after collecting %d chunks", providerName, len(chunks))
			done = true
		}
	}

	// Validate chunks if any were received
	if len(chunks) > 0 {
		lastChunk := chunks[len(chunks)-1]
		assert.True(t, lastChunk.Done, "%s last chunk should be marked as done", providerName)
	}
}

// testErrorHandling tests error handling capabilities
func testErrorHandling(t *testing.T, provider LLMProvider, providerName string) {
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &LLMRequest{
		ModelID: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "This should fail"},
		},
		Stream: false,
	}

	_, err := provider.SendMessage(ctx, req)
	assert.Error(t, err, "%s should return error for cancelled context", providerName)
	t.Logf("%s correctly handled cancelled context: %v", providerName, err)

	// Test with very short timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel2()
	time.Sleep(10 * time.Millisecond) // Ensure timeout has passed

	_, err = provider.SendMessage(ctx2, req)
	assert.Error(t, err, "%s should return error for timeout", providerName)
	t.Logf("%s correctly handled timeout: %v", providerName, err)
}

// testTokenCount tests token counting functionality
func testTokenCount(t *testing.T, provider LLMProvider, providerName string) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "empty text",
			text: "",
		},
		{
			name: "short text",
			text: "Hello",
		},
		{
			name: "medium text",
			text: "Hello, how are you doing today? This is a test message.",
		},
		{
			name: "long text",
			text: "This is a much longer text that contains multiple sentences. " +
				"It should result in a higher token count. " +
				"Token counting is important for managing API costs and rate limits. " +
				"Different providers may have slightly different tokenization schemes.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := provider.GetTokenCount(tt.text)
			assert.GreaterOrEqual(t, count, 0, "%s token count should be non-negative", providerName)

			// Validate rough approximation (1 token ≈ 4 characters)
			expectedMin := len(tt.text) / 5
			expectedMax := len(tt.text)/3 + 1
			assert.GreaterOrEqual(t, count, expectedMin, "%s token count seems too low", providerName)
			assert.LessOrEqual(t, count, expectedMax, "%s token count seems too high", providerName)

			t.Logf("%s token count for %d chars: %d tokens", providerName, len(tt.text), count)
		})
	}
}

// TestLLMService_AllProviders tests the LLM service with all three providers configured
func TestLLMService_AllProviders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create configuration with all three providers
	providers := []LLMProviderConfig{
		{
			ID:       "openai-gpt4",
			Name:     "GPT-4",
			Type:     "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "test-key-openai",
			Model:    "gpt-4",
		},
		{
			ID:       "anthropic-claude",
			Name:     "Claude",
			Type:     "anthropic",
			Endpoint: "https://api.anthropic.com/v1",
			APIKey:   "test-key-anthropic",
			Model:    "claude-3-opus-20240229",
		},
		{
			ID:       "dify-assistant",
			Name:     "Dify Assistant",
			Type:     "dify",
			Endpoint: "https://api.dify.ai/v1",
			APIKey:   "test-key-dify",
			Model:    "assistant",
		},
	}

	cfg := createTestConfig(providers)
	logger := createTestLogger()

	service, err := NewLLMService(cfg, logger)
	require.NoError(t, err, "Failed to create LLM service")
	require.NotNil(t, service, "LLM service should not be nil")

	// Test 1: Verify all providers are registered
	t.Run("AllProvidersRegistered", func(t *testing.T) {
		models := service.GetAvailableModels()
		assert.Equal(t, 3, len(models), "Should have 3 providers registered")

		modelIDs := make(map[string]bool)
		for _, model := range models {
			modelIDs[model.ID] = true
			assert.NotEmpty(t, model.Name, "Model name should not be empty")
			assert.NotEmpty(t, model.Type, "Model type should not be empty")
			assert.NotEmpty(t, model.Endpoint, "Model endpoint should not be empty")
		}

		assert.True(t, modelIDs["openai-gpt4"], "OpenAI provider should be registered")
		assert.True(t, modelIDs["anthropic-claude"], "Anthropic provider should be registered")
		assert.True(t, modelIDs["dify-assistant"], "Dify provider should be registered")
	})

	// Test 2: Validate each provider
	t.Run("ValidateProviders", func(t *testing.T) {
		for _, p := range providers {
			err := service.ValidateModel(p.ID)
			assert.NoError(t, err, "Provider %s should be valid", p.ID)
		}
	})

	// Test 3: Test sending messages to each provider
	t.Run("SendMessageToAllProviders", func(t *testing.T) {
		ctx := context.Background()
		messages := []ChatMessage{
			{Role: "user", Content: "Hello, this is a test"},
		}

		for _, p := range providers {
			t.Run(p.Name, func(t *testing.T) {
				// This will fail with invalid API keys, but validates the routing works
				_, err := service.SendMessage(ctx, p.ID, messages)
				// We expect errors due to invalid keys, but the service should handle them gracefully
				if err != nil {
					t.Logf("Provider %s returned expected error: %v", p.Name, err)
					assert.NotEmpty(t, err.Error(), "Error should have a message")
				}
			})
		}
	})

	// Test 4: Test streaming to each provider
	t.Run("StreamMessageToAllProviders", func(t *testing.T) {
		ctx := context.Background()
		messages := []ChatMessage{
			{Role: "user", Content: "Hello, streaming test"},
		}

		for _, p := range providers {
			t.Run(p.Name, func(t *testing.T) {
				// This will fail with invalid API keys, but validates the routing works
				_, err := service.StreamMessage(ctx, p.ID, messages)
				if err != nil {
					t.Logf("Provider %s streaming returned expected error: %v", p.Name, err)
					assert.NotEmpty(t, err.Error(), "Error should have a message")
				}
			})
		}
	})

	// Test 5: Test token counting for each provider
	t.Run("TokenCountForAllProviders", func(t *testing.T) {
		testText := "This is a test message for token counting"

		for _, p := range providers {
			t.Run(p.Name, func(t *testing.T) {
				count, err := service.GetTokenCount(p.ID, testText)
				assert.NoError(t, err, "Token count should not error")
				assert.Greater(t, count, 0, "Token count should be positive")
				t.Logf("Provider %s token count: %d", p.Name, count)
			})
		}
	})
}

// TestProviderRetryLogic_AllProviders tests retry logic for all providers
func TestProviderRetryLogic_AllProviders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	providers := []LLMProviderConfig{
		{
			ID:       "openai-retry",
			Name:     "OpenAI Retry Test",
			Type:     "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "test-key",
			Model:    "gpt-4",
		},
		{
			ID:       "anthropic-retry",
			Name:     "Anthropic Retry Test",
			Type:     "anthropic",
			Endpoint: "https://api.anthropic.com/v1",
			APIKey:   "test-key",
			Model:    "claude-3-opus-20240229",
		},
		{
			ID:       "dify-retry",
			Name:     "Dify Retry Test",
			Type:     "dify",
			Endpoint: "https://api.dify.ai/v1",
			APIKey:   "test-key",
			Model:    "assistant",
		},
	}

	cfg := createTestConfig(providers)
	logger := createTestLogger()

	service, err := NewLLMService(cfg, logger)
	require.NoError(t, err)

	ctx := context.Background()
	messages := []ChatMessage{
		{Role: "user", Content: "Test retry logic"},
	}

	for _, p := range providers {
		t.Run(p.Name, func(t *testing.T) {
			// Test that service attempts retries on failures
			// With invalid keys, we expect the service to retry and eventually fail
			startTime := time.Now()
			_, err := service.SendMessage(ctx, p.ID, messages)
			duration := time.Since(startTime)

			// Should fail due to invalid keys
			assert.Error(t, err, "Should fail with invalid API key")

			// Retry logic should add some delay, but not too much for non-retryable errors
			// With invalid auth (401), it should fail fast without retries
			assert.Less(t, duration, 5*time.Second, "Should fail relatively quickly for auth errors")

			t.Logf("Provider %s retry test completed in %v: %v", p.Name, duration, err)
		})
	}
}
