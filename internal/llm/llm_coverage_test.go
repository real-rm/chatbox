package llm

// llm_coverage_test.go contains targeted unit tests for functions that
// were at 0% coverage. All tests are pure in-memory — no goconfig, no
// external network calls, no MongoDB.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestService builds a bare-minimum *LLMService without goconfig or
// an actual logger. It is only usable in tests that call the pure-logic
// methods (RegisterProvider, GetAvailableModels, ValidateModel,
// GetTokenCount). The config and logger fields are intentionally nil
// because none of those methods use them.
func newTestService(t *testing.T) *LLMService {
	t.Helper()
	logger := createTestLogger() // defined in test_helpers.go
	return &LLMService{
		providers: make(map[string]LLMProvider),
		models:    make(map[string]ModelInfo),
		logger:    logger,
	}
}

// newTestServiceWithModel creates a test service with a pre-registered
// model in both models and providers maps.
func newTestServiceWithModel(t *testing.T, modelID, modelType string, p LLMProvider) *LLMService {
	t.Helper()
	svc := newTestService(t)
	svc.models[modelID] = ModelInfo{
		ID:       modelID,
		Name:     modelID,
		Type:     modelType,
		Endpoint: "https://test.example.com",
	}
	svc.providers[modelID] = p
	return svc
}

// ---------------------------------------------------------------------------
// createProvider
// ---------------------------------------------------------------------------

func TestCreateProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     LLMProviderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "openai provider",
			cfg: LLMProviderConfig{
				ID:       "p1",
				Type:     "openai",
				APIKey:   "sk-test",
				Endpoint: "https://api.openai.com/v1",
				Model:    "gpt-4",
			},
			wantErr: false,
		},
		{
			name: "anthropic provider",
			cfg: LLMProviderConfig{
				ID:       "p2",
				Type:     "anthropic",
				APIKey:   "sk-ant-test",
				Endpoint: "https://api.anthropic.com/v1",
				Model:    "claude-3-opus-20240229",
			},
			wantErr: false,
		},
		{
			name: "dify provider",
			cfg: LLMProviderConfig{
				ID:       "p3",
				Type:     "dify",
				APIKey:   "dify-key",
				Endpoint: "https://api.dify.ai/v1",
				Model:    "dify-model",
			},
			wantErr: false,
		},
		{
			name: "unsupported provider type",
			cfg: LLMProviderConfig{
				ID:   "p4",
				Type: "unsupported",
			},
			wantErr: true,
			errMsg:  "unsupported provider type",
		},
		{
			name: "empty type is unsupported",
			cfg: LLMProviderConfig{
				ID:   "p5",
				Type: "",
			},
			wantErr: true,
			errMsg:  "unsupported provider type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := createProvider(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, provider)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterProvider
// ---------------------------------------------------------------------------

func TestRegisterProvider_EmptyModelID(t *testing.T) {
	svc := newTestService(t)
	err := svc.RegisterProvider("", &MockLLMProvider{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidModelID)
}

func TestRegisterProvider_NilProvider(t *testing.T) {
	svc := newTestService(t)
	// Put a model in the models map so the nil check is what triggers the error.
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4"}
	err := svc.RegisterProvider("gpt-4", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider cannot be nil")
}

func TestRegisterProvider_ModelNotInConfiguration(t *testing.T) {
	svc := newTestService(t)
	// No models registered — model lookup must fail.
	err := svc.RegisterProvider("non-existent", &MockLLMProvider{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in configuration")
}

func TestRegisterProvider_Success(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4", Name: "GPT-4", Type: "openai"}

	mock := &MockLLMProvider{}
	err := svc.RegisterProvider("gpt-4", mock)
	require.NoError(t, err)

	// Verify the provider was actually stored.
	svc.mu.RLock()
	stored := svc.providers["gpt-4"]
	svc.mu.RUnlock()
	assert.Equal(t, mock, stored)
}

func TestRegisterProvider_ReplaceExisting(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4"}

	first := &MockLLMProvider{}
	second := &MockLLMProvider{}

	require.NoError(t, svc.RegisterProvider("gpt-4", first))
	require.NoError(t, svc.RegisterProvider("gpt-4", second))

	svc.mu.RLock()
	stored := svc.providers["gpt-4"]
	svc.mu.RUnlock()
	assert.Equal(t, second, stored)
}

// ---------------------------------------------------------------------------
// GetAvailableModels
// ---------------------------------------------------------------------------

func TestGetAvailableModels_Empty(t *testing.T) {
	svc := newTestService(t)
	models := svc.GetAvailableModels()
	assert.NotNil(t, models)
	assert.Empty(t, models)
}

func TestGetAvailableModels_SingleModel(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{
		ID:       "gpt-4",
		Name:     "GPT-4",
		Type:     "openai",
		Endpoint: "https://api.openai.com",
	}

	models := svc.GetAvailableModels()
	require.Len(t, models, 1)
	assert.Equal(t, "gpt-4", models[0].ID)
	assert.Equal(t, "GPT-4", models[0].Name)
	assert.Equal(t, "openai", models[0].Type)
}

func TestGetAvailableModels_MultipleModels(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4", Type: "openai"}
	svc.models["claude-3"] = ModelInfo{ID: "claude-3", Type: "anthropic"}
	svc.models["dify-1"] = ModelInfo{ID: "dify-1", Type: "dify"}

	models := svc.GetAvailableModels()
	assert.Len(t, models, 3)

	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}
	assert.True(t, ids["gpt-4"])
	assert.True(t, ids["claude-3"])
	assert.True(t, ids["dify-1"])
}

func TestGetAvailableModels_ConcurrentReads(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4", Type: "openai"}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			models := svc.GetAvailableModels()
			assert.Len(t, models, 1)
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// ValidateModel
// ---------------------------------------------------------------------------

func TestValidateModel_EmptyModelID(t *testing.T) {
	svc := newTestService(t)
	err := svc.ValidateModel("")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidModelID)
}

func TestValidateModel_NotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.ValidateModel("non-existent-model")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderNotFound)
}

func TestValidateModel_ValidModel(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4", Type: "openai"}

	err := svc.ValidateModel("gpt-4")
	require.NoError(t, err)
}

func TestValidateModel_TableDriven(t *testing.T) {
	svc := newTestService(t)
	svc.models["gpt-4"] = ModelInfo{ID: "gpt-4", Type: "openai"}
	svc.models["claude-3"] = ModelInfo{ID: "claude-3", Type: "anthropic"}

	tests := []struct {
		name    string
		modelID string
		wantErr bool
		errIs   error
	}{
		{name: "empty model ID", modelID: "", wantErr: true, errIs: ErrInvalidModelID},
		{name: "non-existent model", modelID: "missing", wantErr: true, errIs: ErrProviderNotFound},
		{name: "valid gpt-4", modelID: "gpt-4", wantErr: false},
		{name: "valid claude-3", modelID: "claude-3", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidateModel(tt.modelID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetTokenCount
// ---------------------------------------------------------------------------

func TestGetTokenCount_EmptyModelID(t *testing.T) {
	svc := newTestService(t)
	count, err := svc.GetTokenCount("", "some text")
	assert.Equal(t, 0, count)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidModelID)
}

func TestGetTokenCount_ProviderNotFound(t *testing.T) {
	svc := newTestService(t)
	count, err := svc.GetTokenCount("non-existent", "some text")
	assert.Equal(t, 0, count)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderNotFound)
}

func TestGetTokenCount_Success(t *testing.T) {
	mock := &MockLLMProvider{
		getTokenCountFunc: func(text string) int {
			return len(text) / 4
		},
	}
	svc := newTestServiceWithModel(t, "gpt-4", "openai", mock)

	count, err := svc.GetTokenCount("gpt-4", "Hello, world!")
	require.NoError(t, err)
	assert.Equal(t, 13/4, count) // 3
}

func TestGetTokenCount_EmptyText(t *testing.T) {
	mock := &MockLLMProvider{
		getTokenCountFunc: func(text string) int {
			return len(text) / 4
		},
	}
	svc := newTestServiceWithModel(t, "gpt-4", "openai", mock)

	count, err := svc.GetTokenCount("gpt-4", "")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetTokenCount_LongText(t *testing.T) {
	const text = "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs."
	mock := &MockLLMProvider{} // uses default: len(text)/4
	svc := newTestServiceWithModel(t, "gpt-4", "openai", mock)

	count, err := svc.GetTokenCount("gpt-4", text)
	require.NoError(t, err)
	assert.Equal(t, len(text)/4, count)
}

func TestGetTokenCount_TableDriven(t *testing.T) {
	mock := &MockLLMProvider{
		getTokenCountFunc: func(text string) int {
			return len(text) / 4
		},
	}
	svc := newTestServiceWithModel(t, "gpt-4", "openai", mock)

	tests := []struct {
		name      string
		modelID   string
		text      string
		wantErr   bool
		errIs     error
		wantCount int
	}{
		{name: "empty model ID", modelID: "", text: "hi", wantErr: true, errIs: ErrInvalidModelID},
		{name: "unknown model", modelID: "gpt-99", text: "hi", wantErr: true, errIs: ErrProviderNotFound},
		{name: "empty text", modelID: "gpt-4", text: "", wantErr: false, wantCount: 0},
		{name: "4 chars", modelID: "gpt-4", text: "abcd", wantErr: false, wantCount: 1},
		{name: "8 chars", modelID: "gpt-4", text: "abcdefgh", wantErr: false, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := svc.GetTokenCount(tt.modelID, tt.text)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				assert.Equal(t, 0, count)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCount, count)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MockLLMProvider — StreamMessage and GetTokenCount default paths
// (increases coverage of test_helpers.go lines 120-136)
// ---------------------------------------------------------------------------

func TestMockLLMProvider_StreamMessage_DefaultPath(t *testing.T) {
	mock := &MockLLMProvider{} // no streamMessageFunc set
	ctx := context.Background()
	req := &LLMRequest{ModelID: "test", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}

	ch, err := mock.StreamMessage(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, ch)

	var chunks []*LLMChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	assert.Len(t, chunks, 2)
	assert.Equal(t, "chunk1", chunks[0].Content)
	assert.False(t, chunks[0].Done)
	assert.Equal(t, "chunk2", chunks[1].Content)
	assert.True(t, chunks[1].Done)
}

func TestMockLLMProvider_StreamMessage_CustomFunc(t *testing.T) {
	called := false
	mock := &MockLLMProvider{
		streamMessageFunc: func(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
			called = true
			ch := make(chan *LLMChunk, 1)
			ch <- &LLMChunk{Content: "custom", Done: true}
			close(ch)
			return ch, nil
		},
	}
	ctx := context.Background()
	req := &LLMRequest{ModelID: "m", Messages: nil}

	ch, err := mock.StreamMessage(ctx, req)
	require.NoError(t, err)
	assert.True(t, called)

	var chunks []*LLMChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	assert.Len(t, chunks, 1)
}

func TestMockLLMProvider_StreamMessage_ReturnsError(t *testing.T) {
	mock := &MockLLMProvider{
		streamMessageFunc: func(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
			return nil, errors.New("stream error")
		},
	}
	ctx := context.Background()
	req := &LLMRequest{ModelID: "m"}

	ch, err := mock.StreamMessage(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, ch)
}

func TestMockLLMProvider_GetTokenCount_DefaultPath(t *testing.T) {
	mock := &MockLLMProvider{} // no getTokenCountFunc set — uses default
	result := mock.GetTokenCount("hello world")
	// Default: len(text)/4 = 11/4 = 2
	assert.Equal(t, 11/4, result)
}

func TestMockLLMProvider_GetTokenCount_CustomFunc(t *testing.T) {
	mock := &MockLLMProvider{
		getTokenCountFunc: func(text string) int {
			return 42
		},
	}
	assert.Equal(t, 42, mock.GetTokenCount("anything"))
}

func TestMockLLMProvider_SendMessage_DefaultPath(t *testing.T) {
	mock := &MockLLMProvider{} // no sendMessageFunc — uses default response
	ctx := context.Background()
	req := &LLMRequest{ModelID: "m", Messages: nil}

	resp, err := mock.SendMessage(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "mock response", resp.Content)
	assert.Equal(t, 10, resp.TokensUsed)
}

// ---------------------------------------------------------------------------
// getStringFromMap
// ---------------------------------------------------------------------------

func TestGetStringFromMap(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]interface{}
		key      string
		expected string
	}{
		{
			name:     "key present with string value",
			m:        map[string]interface{}{"name": "Alice"},
			key:      "name",
			expected: "Alice",
		},
		{
			name:     "key absent",
			m:        map[string]interface{}{"other": "value"},
			key:      "name",
			expected: "",
		},
		{
			name:     "key present with non-string value (int)",
			m:        map[string]interface{}{"count": 42},
			key:      "count",
			expected: "", // non-string returns empty
		},
		{
			name:     "key present with nil value",
			m:        map[string]interface{}{"nilKey": nil},
			key:      "nilKey",
			expected: "", // nil is not a string
		},
		{
			name:     "empty map",
			m:        map[string]interface{}{},
			key:      "anything",
			expected: "",
		},
		{
			name:     "key present with bool value",
			m:        map[string]interface{}{"flag": true},
			key:      "flag",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringFromMap(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// registerProviderUnsafe (internal helper)
// ---------------------------------------------------------------------------

func TestRegisterProviderUnsafe_EmptyModelID(t *testing.T) {
	svc := newTestService(t)
	err := svc.registerProviderUnsafe("", &MockLLMProvider{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidModelID)
}

func TestRegisterProviderUnsafe_NilProvider(t *testing.T) {
	svc := newTestService(t)
	err := svc.registerProviderUnsafe("model-1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider cannot be nil")
}

func TestRegisterProviderUnsafe_AddsModelInfo(t *testing.T) {
	svc := newTestService(t)
	// Model does NOT exist in svc.models — registerProviderUnsafe should add it.
	mock := &MockLLMProvider{}
	err := svc.registerProviderUnsafe("new-model", mock)
	require.NoError(t, err)

	// Model info should now exist.
	svc.mu.RLock()
	info, exists := svc.models["new-model"]
	svc.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, "new-model", info.ID)
	assert.Equal(t, "test", info.Type)
}

func TestRegisterProviderUnsafe_ModelAlreadyExists(t *testing.T) {
	svc := newTestService(t)
	svc.models["existing"] = ModelInfo{ID: "existing", Name: "Existing", Type: "openai"}

	mock := &MockLLMProvider{}
	err := svc.registerProviderUnsafe("existing", mock)
	require.NoError(t, err)

	// Provider should be stored and model info preserved.
	svc.mu.RLock()
	stored := svc.providers["existing"]
	info := svc.models["existing"]
	svc.mu.RUnlock()
	assert.Equal(t, mock, stored)
	// Original type should not be overwritten (the guard only adds if absent).
	assert.Equal(t, "openai", info.Type)
}
