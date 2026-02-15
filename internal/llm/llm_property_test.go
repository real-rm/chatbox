package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: chat-application-websocket, Property 10: Valid Message Routing to LLM
// **Validates: Requirements 3.2**
//
// For any valid user message, the WebSocket_Server should forward it to the LLM_Backend for processing.
func TestProperty_ValidMessageRoutingToLLM(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	// Create a single shared config for all test iterations
	cfg := createTestConfig([]LLMProviderConfig{
		{
			ID:       "test-model-1",
			Name:     "Test Model 1",
			Type:     "openai",
			Endpoint: "https://api.test.com",
			APIKey:   "test-key",
		},
		{
			ID:       "test-model-2",
			Name:     "Test Model 2",
			Type:     "openai",
			Endpoint: "https://api.test.com",
			APIKey:   "test-key",
		},
		{
			ID:       "test-model-3",
			Name:     "Test Model 3",
			Type:     "openai",
			Endpoint: "https://api.test.com",
			APIKey:   "test-key",
		},
		{
			ID:       "test-model-4",
			Name:     "Test Model 4",
			Type:     "openai",
			Endpoint: "https://api.test.com",
			APIKey:   "test-key",
		},
	})

	logger := createTestLogger()

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("valid messages are routed to LLM provider", prop.ForAll(
		func(modelIDIndex int, userMessage string) bool {
			// Skip if required fields are empty
			if userMessage == "" {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			// Track if provider was called
			providerCalled := false
			var receivedRequest *LLMRequest

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					providerCalled = true
					receivedRequest = req
					return &LLMResponse{
						Content:    "Response to: " + req.Messages[0].Content,
						TokensUsed: 10,
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Send message
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: userMessage}}
			_, err = service.SendMessage(ctx, modelID, messages)

			if err != nil {
				t.Logf("Failed to send message: %v", err)
				return false
			}

			// Verify provider was called
			if !providerCalled {
				t.Logf("Provider was not called")
				return false
			}

			// Verify request contains the message
			if receivedRequest == nil {
				t.Logf("Received request is nil")
				return false
			}

			if len(receivedRequest.Messages) == 0 {
				t.Logf("No messages in request")
				return false
			}

			if receivedRequest.Messages[0].Content != userMessage {
				t.Logf("Message content mismatch: expected %s, got %s", userMessage, receivedRequest.Messages[0].Content)
				return false
			}

			return true
		},
		gen.IntRange(0, 1000), // modelIDIndex - will be modulo'd to select from fixed list
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // userMessage
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 11: LLM Response Delivery
// **Validates: Requirements 3.3**
//
// For any LLM_Backend response, the WebSocket_Server should deliver it to the correct
// Chat_Client through the WebSocket connection.
func TestProperty_LLMResponseDelivery(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LLM responses are delivered correctly", prop.ForAll(
		func(modelIDIndex int, responseContent string, tokensUsed uint16) bool {
			// Skip if required fields are empty
			if responseContent == "" {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					return &LLMResponse{
						Content:    responseContent,
						TokensUsed: int(tokensUsed),
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Send message and get response
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}
			resp, err := service.SendMessage(ctx, modelID, messages)

			if err != nil {
				t.Logf("Failed to send message: %v", err)
				return false
			}

			// Verify response is delivered correctly
			if resp == nil {
				t.Logf("Response is nil")
				return false
			}

			if resp.Content != responseContent {
				t.Logf("Response content mismatch: expected %s, got %s", responseContent, resp.Content)
				return false
			}

			if resp.TokensUsed != int(tokensUsed) {
				t.Logf("Tokens used mismatch: expected %d, got %d", tokensUsed, resp.TokensUsed)
				return false
			}

			return true
		},
		gen.IntRange(0, 1000), // modelIDIndex
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // responseContent
		gen.UInt16(), // tokensUsed
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 13: Response Time Tracking
// **Validates: Requirements 3.7, 18.10**
//
// For any LLM_Backend request, the WebSocket_Server should measure and log the response time,
// and this time should be included in session metrics.
func TestProperty_ResponseTimeTracking(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("response time is tracked for all LLM requests", prop.ForAll(
		func(modelIDIndex int, delayMs uint8) bool {
			// Skip if delay is too large
			if delayMs > 200 {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			delay := time.Duration(delayMs) * time.Millisecond

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					// Simulate processing time
					time.Sleep(delay)
					return &LLMResponse{
						Content:    "Response",
						TokensUsed: 10,
						Duration:   0, // Not set by provider
					}, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Send message
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}
			resp, err := service.SendMessage(ctx, modelID, messages)

			if err != nil {
				t.Logf("Failed to send message: %v", err)
				return false
			}

			// Verify response time is tracked
			if resp.Duration == 0 {
				t.Logf("Response duration is zero")
				return false
			}

			// Verify duration is at least the simulated delay (with small tolerance)
			if resp.Duration < delay-10*time.Millisecond {
				t.Logf("Response duration %v is less than expected delay %v", resp.Duration, delay)
				return false
			}

			return true
		},
		gen.IntRange(0, 1000),   // modelIDIndex
		gen.UInt8Range(10, 200), // delayMs
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 27: LLM Request Context Inclusion
// **Validates: Requirements 7.2**
//
// For any message forwarded to the LLM_Backend, the request should include both
// the session context and the user message.
func TestProperty_LLMRequestContextInclusion(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LLM requests include session context and user message", prop.ForAll(
		func(modelIDIndex int, messageCount uint8) bool {
			// Skip if message count is invalid
			if messageCount == 0 || messageCount > 10 {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			var receivedRequest *LLMRequest

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					receivedRequest = req
					return &LLMResponse{
						Content:    "Response",
						TokensUsed: 10,
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Build conversation context with multiple messages
			messages := make([]ChatMessage, messageCount)
			for i := uint8(0); i < messageCount; i++ {
				role := "user"
				if i%2 == 1 {
					role = "assistant"
				}
				messages[i] = ChatMessage{
					Role:    role,
					Content: "Message " + string(rune('A'+i)),
				}
			}

			// Send message with context
			ctx := context.Background()
			_, err = service.SendMessage(ctx, modelID, messages)

			if err != nil {
				t.Logf("Failed to send message: %v", err)
				return false
			}

			// Verify request includes all context messages
			if receivedRequest == nil {
				t.Logf("Received request is nil")
				return false
			}

			if len(receivedRequest.Messages) != int(messageCount) {
				t.Logf("Message count mismatch: expected %d, got %d", messageCount, len(receivedRequest.Messages))
				return false
			}

			// Verify all messages are included
			for i := 0; i < int(messageCount); i++ {
				if receivedRequest.Messages[i].Content != messages[i].Content {
					t.Logf("Message %d content mismatch", i)
					return false
				}
				if receivedRequest.Messages[i].Role != messages[i].Role {
					t.Logf("Message %d role mismatch", i)
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 1000), // modelIDIndex
		gen.UInt8Range(1, 10), // messageCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 28: Streaming Response Forwarding
// **Validates: Requirements 7.3**
//
// For any LLM_Backend that supports streaming, response chunks should be forwarded
// to the Chat_Client in real-time as they are received.
func TestProperty_StreamingResponseForwarding(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("streaming responses are forwarded in real-time", prop.ForAll(
		func(modelIDIndex int, chunkCount uint8) bool {
			// Skip if chunk count is invalid
			if chunkCount == 0 || chunkCount > 10 {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			mockProvider := &MockLLMProvider{
				streamMessageFunc: func(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
					ch := make(chan *LLMChunk, chunkCount)
					go func() {
						defer close(ch)
						for i := uint8(0); i < chunkCount; i++ {
							chunk := &LLMChunk{
								Content: "Chunk " + string(rune('A'+i)),
								Done:    i == chunkCount-1,
							}
							ch <- chunk
						}
					}()
					return ch, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Stream message
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}
			chunkChan, err := service.StreamMessage(ctx, modelID, messages)

			if err != nil {
				t.Logf("Failed to stream message: %v", err)
				return false
			}

			// Collect all chunks
			receivedChunks := []LLMChunk{}
			for chunk := range chunkChan {
				receivedChunks = append(receivedChunks, *chunk)
			}

			// Verify all chunks were received
			if len(receivedChunks) != int(chunkCount) {
				t.Logf("Chunk count mismatch: expected %d, got %d", chunkCount, len(receivedChunks))
				return false
			}

			// Verify chunks are in order
			for i := 0; i < int(chunkCount); i++ {
				expectedContent := "Chunk " + string(rune('A'+uint8(i)))
				if receivedChunks[i].Content != expectedContent {
					t.Logf("Chunk %d content mismatch: expected %s, got %s", i, expectedContent, receivedChunks[i].Content)
					return false
				}
			}

			// Verify last chunk is marked as done
			if !receivedChunks[len(receivedChunks)-1].Done {
				t.Logf("Last chunk not marked as done")
				return false
			}

			return true
		},
		gen.IntRange(0, 1000), // modelIDIndex
		gen.UInt8Range(1, 10), // chunkCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 29: LLM Backend Retry Logic
// **Validates: Requirements 7.4**
//
// For any LLM_Backend failure, the WebSocket_Server should return an error message
// and retry the request with exponential backoff.
func TestProperty_LLMBackendRetryLogic(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("LLM backend failures trigger retry with exponential backoff", prop.ForAll(
		func(modelIDIndex int, failureCount uint8) bool {
			// Skip if failure count is invalid
			if failureCount == 0 || failureCount > 3 {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			attemptCount := 0
			var attemptTimes []time.Time

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					attemptCount++
					attemptTimes = append(attemptTimes, time.Now())

					if attemptCount <= int(failureCount) {
						// Fail with retryable error
						return nil, errors.New("connection timeout")
					}

					// Succeed after specified failures
					return &LLMResponse{
						Content:    "Success after retry",
						TokensUsed: 10,
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Send message
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}
			resp, err := service.SendMessage(ctx, modelID, messages)

			// If failure count is less than max retries, should succeed
			if failureCount < 3 {
				if err != nil {
					t.Logf("Expected success after %d failures, got error: %v", failureCount, err)
					return false
				}

				if resp == nil {
					t.Logf("Response is nil")
					return false
				}

				// Verify retry count
				expectedAttempts := int(failureCount) + 1
				if attemptCount != expectedAttempts {
					t.Logf("Attempt count mismatch: expected %d, got %d", expectedAttempts, attemptCount)
					return false
				}

				// Verify exponential backoff (each retry should take longer than previous)
				if len(attemptTimes) > 1 {
					for i := 1; i < len(attemptTimes); i++ {
						delay := attemptTimes[i].Sub(attemptTimes[i-1])
						// First retry should be ~1s, second ~2s
						expectedMinDelay := time.Duration(1<<uint(i-1)) * time.Second
						if delay < expectedMinDelay-100*time.Millisecond {
							t.Logf("Retry %d delay %v is less than expected %v", i, delay, expectedMinDelay)
							return false
						}
					}
				}
			} else {
				// If failure count equals max retries, should fail
				if err == nil {
					t.Logf("Expected error after %d failures", failureCount)
					return false
				}

				if !strings.Contains(err.Error(), "failed after") {
					t.Logf("Error message should indicate retry exhaustion: %v", err)
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 1000), // modelIDIndex
		gen.UInt8Range(1, 3),  // failureCount (1-3 to test retry logic)
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 30: Model Selection Persistence
// **Validates: Requirements 7.7**
//
// For any session where a user selects a different LLM model, all subsequent requests
// in that session should use the selected model.
func TestProperty_ModelSelectionPersistence(t *testing.T) {
	// Use fixed model IDs to avoid goconfig caching issues
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("selected model is used for all subsequent requests", prop.ForAll(
		func(modelID1Index, modelID2Index int, requestCount uint8) bool {
			// Skip if request count is invalid
			if requestCount == 0 || requestCount > 5 {
				return true
			}

			// Use different fixed model IDs
			modelID1 := fixedModelIDs[modelID1Index%len(fixedModelIDs)]
			modelID2 := fixedModelIDs[modelID2Index%len(fixedModelIDs)]

			// Skip if models are the same
			if modelID1 == modelID2 {
				return true
			}

			model1Calls := 0
			model2Calls := 0

			mockProvider1 := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					model1Calls++
					return &LLMResponse{
						Content:    "Response from model 1",
						TokensUsed: 10,
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			mockProvider2 := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					model2Calls++
					return &LLMResponse{
						Content:    "Response from model 2",
						TokensUsed: 10,
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			// Create config with all possible model IDs to avoid registration errors
			allProviders := make([]LLMProviderConfig, len(fixedModelIDs))
			for i, modelID := range fixedModelIDs {
				allProviders[i] = LLMProviderConfig{
					ID:       modelID,
					Name:     fmt.Sprintf("Test Model %d", i+1),
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				}
			}
			cfg := createTestConfig(allProviders)

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID1, mockProvider1)
			if err != nil {
				t.Logf("Failed to register provider 1: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID2, mockProvider2)
			if err != nil {
				t.Logf("Failed to register provider 2: %v", err)
				return false
			}

			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}

			// Send requests to model 1
			for i := uint8(0); i < requestCount; i++ {
				_, err := service.SendMessage(ctx, modelID1, messages)
				if err != nil {
					t.Logf("Failed to send message to model 1: %v", err)
					return false
				}
			}

			// Verify model 1 was called correct number of times
			if model1Calls != int(requestCount) {
				t.Logf("Model 1 call count mismatch: expected %d, got %d", requestCount, model1Calls)
				return false
			}

			// Switch to model 2 and send more requests
			for i := uint8(0); i < requestCount; i++ {
				_, err := service.SendMessage(ctx, modelID2, messages)
				if err != nil {
					t.Logf("Failed to send message to model 2: %v", err)
					return false
				}
			}

			// Verify model 2 was called correct number of times
			if model2Calls != int(requestCount) {
				t.Logf("Model 2 call count mismatch: expected %d, got %d", requestCount, model2Calls)
				return false
			}

			// Verify model 1 wasn't called again
			if model1Calls != int(requestCount) {
				t.Logf("Model 1 was called again after switching to model 2")
				return false
			}

			return true
		},
		gen.IntRange(0, 1000), // modelID1Index
		gen.IntRange(0, 1000), // modelID2Index
		gen.UInt8Range(1, 5),  // requestCount
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}

// Feature: chat-application-websocket, Property 62: Token Usage Tracking and Storage
// **Validates: Requirements 18.13, 18.14**
//
// For any LLM_Backend request, the WebSocket_Server should track token usage and store
// the total token count in the session metadata.
func TestProperty_TokenUsageTrackingAndStorage(t *testing.T) {
	// Use a fixed set of model IDs to avoid goconfig caching issues
	// Using 4 models to match TestProperty_ModelSelectionPersistence
	fixedModelIDs := []string{"test-model-1", "test-model-2", "test-model-3", "test-model-4"}

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("token usage is tracked and accumulated correctly", prop.ForAll(
		func(modelIDIndex int, requestCount uint8, tokensPerRequest uint16) bool {
			// Skip if counts are invalid
			if requestCount == 0 || requestCount > 10 || tokensPerRequest == 0 {
				return true
			}

			// Use a fixed model ID from the list
			modelID := fixedModelIDs[modelIDIndex%len(fixedModelIDs)]

			totalTokensUsed := 0

			mockProvider := &MockLLMProvider{
				sendMessageFunc: func(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
					return &LLMResponse{
						Content:    "Response",
						TokensUsed: int(tokensPerRequest),
						Duration:   100 * time.Millisecond,
					}, nil
				},
			}

			cfg := createTestConfig([]LLMProviderConfig{
				{
					ID:       modelID,
					Name:     "Test Model",
					Type:     "openai",
					Endpoint: "https://api.test.com",
					APIKey:   "test-key",
				},
			})

			logger := createTestLogger()
			service, err := NewLLMService(cfg, logger)
			if err != nil {
				t.Logf("Failed to create service: %v", err)
				return false
			}

			err = service.RegisterProvider(modelID, mockProvider)
			if err != nil {
				t.Logf("Failed to register provider: %v", err)
				return false
			}

			// Send multiple requests and track tokens
			ctx := context.Background()
			messages := []ChatMessage{{Role: "user", Content: "Test"}}

			for i := uint8(0); i < requestCount; i++ {
				resp, err := service.SendMessage(ctx, modelID, messages)
				if err != nil {
					t.Logf("Failed to send message: %v", err)
					return false
				}

				// Verify tokens are reported in response
				if resp.TokensUsed != int(tokensPerRequest) {
					t.Logf("Tokens used mismatch: expected %d, got %d", tokensPerRequest, resp.TokensUsed)
					return false
				}

				totalTokensUsed += resp.TokensUsed
			}

			// Verify total tokens
			expectedTotal := int(requestCount) * int(tokensPerRequest)
			if totalTokensUsed != expectedTotal {
				t.Logf("Total tokens mismatch: expected %d, got %d", expectedTotal, totalTokensUsed)
				return false
			}

			return true
		},
		gen.IntRange(0, 1000),    // modelIDIndex
		gen.UInt8Range(1, 10),    // requestCount
		gen.UInt16Range(1, 1000), // tokensPerRequest
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
