package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PanickingProvider is a mock that panics during StreamMessage
type PanickingProvider struct{}

func (p *PanickingProvider) SendMessage(ctx context.Context, req *LLMRequest) (*LLMResponse, error) {
	return &LLMResponse{Content: "ok", TokensUsed: 1, Duration: time.Millisecond}, nil
}

func (p *PanickingProvider) StreamMessage(ctx context.Context, req *LLMRequest) (<-chan *LLMChunk, error) {
	ch := make(chan *LLMChunk)
	go func() {
		defer close(ch)
		defer recoverStreamPanic(ch, "test-panicking")
		// Simulate a panic inside the streaming goroutine
		panic("unexpected nil pointer in streaming")
	}()
	return ch, nil
}

func (p *PanickingProvider) GetTokenCount(text string) int {
	return len(text) / 4
}

func TestStreamPanicRecovery(t *testing.T) {
	provider := &PanickingProvider{}

	ch, err := provider.StreamMessage(context.Background(), &LLMRequest{
		ModelID:  "test",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   true,
	})
	require.NoError(t, err)
	require.NotNil(t, ch)

	// The channel should receive a Done chunk (from panic recovery) and then close
	var chunks []*LLMChunk
	timeout := time.After(2 * time.Second)
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				goto done
			}
			chunks = append(chunks, chunk)
		case <-timeout:
			t.Fatal("timed out waiting for chunks from panicking provider")
		}
	}
done:

	// Should have received at least one chunk with Done: true
	require.NotEmpty(t, chunks, "should receive at least one chunk from panic recovery")
	assert.True(t, chunks[len(chunks)-1].Done, "last chunk should have Done=true")
}

func TestRecoverStreamPanic_NoPanic(t *testing.T) {
	// When there is no panic, recoverStreamPanic should not send anything
	ch := make(chan *LLMChunk, 1)

	func() {
		defer recoverStreamPanic(ch, "test-no-panic")
		// No panic here
	}()

	// Channel should be empty
	select {
	case <-ch:
		t.Fatal("should not have received a chunk when there's no panic")
	default:
		// Expected: no chunk sent
	}
}
