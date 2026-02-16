package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestMetricsRegistration verifies that all metrics are properly registered
func TestMetricsRegistration(t *testing.T) {
	tests := []struct {
		name   string
		metric prometheus.Collector
	}{
		{"WebSocketConnections", WebSocketConnections},
		{"MessagesReceived", MessagesReceived},
		{"MessagesSent", MessagesSent},
		{"LLMRequests", LLMRequests},
		{"LLMLatency", LLMLatency},
		{"LLMErrors", LLMErrors},
		{"ActiveSessions", ActiveSessions},
		{"SessionsCreated", SessionsCreated},
		{"SessionsEnded", SessionsEnded},
		{"AdminTakeovers", AdminTakeovers},
		{"MessageErrors", MessageErrors},
		{"TokensUsed", TokensUsed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.metric == nil {
				t.Errorf("Metric %s is nil", tt.name)
			}
		})
	}
}

// TestWebSocketConnectionsMetric verifies the WebSocket connections gauge
func TestWebSocketConnectionsMetric(t *testing.T) {
	// Get initial value
	var m dto.Metric
	if err := WebSocketConnections.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	initialValue := m.GetGauge().GetValue()

	// Increment
	WebSocketConnections.Inc()
	if err := WebSocketConnections.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	afterInc := m.GetGauge().GetValue()

	if afterInc != initialValue+1 {
		t.Errorf("Expected value %f after Inc(), got %f", initialValue+1, afterInc)
	}

	// Decrement
	WebSocketConnections.Dec()
	if err := WebSocketConnections.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	afterDec := m.GetGauge().GetValue()

	if afterDec != initialValue {
		t.Errorf("Expected value %f after Dec(), got %f", initialValue, afterDec)
	}
}

// TestMessagesReceivedMetric verifies the messages received counter
func TestMessagesReceivedMetric(t *testing.T) {
	// Get initial value
	var m dto.Metric
	if err := MessagesReceived.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	initialValue := m.GetCounter().GetValue()

	// Increment
	MessagesReceived.Inc()
	if err := MessagesReceived.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	afterInc := m.GetCounter().GetValue()

	if afterInc != initialValue+1 {
		t.Errorf("Expected value %f after Inc(), got %f", initialValue+1, afterInc)
	}
}

// TestLLMMetricsWithLabels verifies LLM metrics with provider labels
func TestLLMMetricsWithLabels(t *testing.T) {
	providers := []string{"openai", "anthropic", "dify"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			// Test LLM requests counter
			LLMRequests.WithLabelValues(provider).Inc()

			// Test LLM latency histogram
			LLMLatency.WithLabelValues(provider).Observe(0.5)

			// Test LLM errors counter
			LLMErrors.WithLabelValues(provider).Inc()

			// Test tokens used counter
			TokensUsed.WithLabelValues(provider).Add(100)
		})
	}
}

// TestSessionMetrics verifies session-related metrics
func TestSessionMetrics(t *testing.T) {
	// Get initial values
	var m dto.Metric

	// Test SessionsCreated
	if err := SessionsCreated.Write(&m); err != nil {
		t.Fatalf("Failed to write SessionsCreated metric: %v", err)
	}
	initialCreated := m.GetCounter().GetValue()

	SessionsCreated.Inc()
	if err := SessionsCreated.Write(&m); err != nil {
		t.Fatalf("Failed to write SessionsCreated metric: %v", err)
	}
	afterCreated := m.GetCounter().GetValue()

	if afterCreated != initialCreated+1 {
		t.Errorf("Expected SessionsCreated %f, got %f", initialCreated+1, afterCreated)
	}

	// Test ActiveSessions
	if err := ActiveSessions.Write(&m); err != nil {
		t.Fatalf("Failed to write ActiveSessions metric: %v", err)
	}
	initialActive := m.GetGauge().GetValue()

	ActiveSessions.Inc()
	if err := ActiveSessions.Write(&m); err != nil {
		t.Fatalf("Failed to write ActiveSessions metric: %v", err)
	}
	afterActive := m.GetGauge().GetValue()

	if afterActive != initialActive+1 {
		t.Errorf("Expected ActiveSessions %f, got %f", initialActive+1, afterActive)
	}

	// Test SessionsEnded
	if err := SessionsEnded.Write(&m); err != nil {
		t.Fatalf("Failed to write SessionsEnded metric: %v", err)
	}
	initialEnded := m.GetCounter().GetValue()

	SessionsEnded.Inc()
	ActiveSessions.Dec()

	if err := SessionsEnded.Write(&m); err != nil {
		t.Fatalf("Failed to write SessionsEnded metric: %v", err)
	}
	afterEnded := m.GetCounter().GetValue()

	if afterEnded != initialEnded+1 {
		t.Errorf("Expected SessionsEnded %f, got %f", initialEnded+1, afterEnded)
	}
}
