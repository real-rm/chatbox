// Package metrics provides Prometheus metrics collection for the chatbox application.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WebSocketConnections tracks the current number of active WebSocket connections
	WebSocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "chatbox_websocket_connections_total",
		Help: "Current number of active WebSocket connections",
	})

	// MessagesReceived tracks the total number of messages received from clients
	MessagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_messages_received_total",
		Help: "Total number of messages received from clients",
	})

	// MessagesSent tracks the total number of messages sent to clients
	MessagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_messages_sent_total",
		Help: "Total number of messages sent to clients",
	})

	// LLMRequests tracks the total number of LLM requests by provider
	LLMRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chatbox_llm_requests_total",
		Help: "Total number of LLM requests by provider",
	}, []string{"provider"})

	// LLMLatency tracks the latency of LLM requests by provider
	LLMLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "chatbox_llm_latency_seconds",
		Help:    "Latency of LLM requests in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider"})

	// LLMErrors tracks the total number of LLM errors by provider
	LLMErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chatbox_llm_errors_total",
		Help: "Total number of LLM errors by provider",
	}, []string{"provider"})

	// ActiveSessions tracks the current number of active chat sessions
	ActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "chatbox_active_sessions_total",
		Help: "Current number of active chat sessions",
	})

	// SessionsCreated tracks the total number of sessions created
	SessionsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_sessions_created_total",
		Help: "Total number of chat sessions created",
	})

	// SessionsEnded tracks the total number of sessions ended
	SessionsEnded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_sessions_ended_total",
		Help: "Total number of chat sessions ended",
	})

	// AdminTakeovers tracks the total number of admin takeovers
	AdminTakeovers = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_admin_takeovers_total",
		Help: "Total number of admin session takeovers",
	})

	// MessageErrors tracks the total number of message processing errors
	MessageErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chatbox_message_errors_total",
		Help: "Total number of message processing errors",
	})

	// TokensUsed tracks the total number of LLM tokens used by provider
	TokensUsed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "chatbox_tokens_used_total",
		Help: "Total number of LLM tokens used by provider",
	}, []string{"provider"})
)
