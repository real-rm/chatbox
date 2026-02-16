# Chatbox Metrics

This package provides Prometheus metrics collection for the chatbox application.

## Metrics Endpoint

The `/metrics` endpoint exposes Prometheus-formatted metrics that can be scraped by Prometheus or other monitoring systems.

**Endpoint:** `GET /metrics`  
**Authentication:** None (public endpoint)  
**Format:** Prometheus text format

## Available Metrics

### Connection Metrics

- **`chatbox_websocket_connections_total`** (Gauge)
  - Current number of active WebSocket connections
  - Use this to monitor real-time connection load
  - **Instrumented in:** `internal/websocket/handler.go` (registerConnection/unregisterConnection)

- **`chatbox_active_sessions_total`** (Gauge)
  - Current number of active chat sessions
  - Use this to monitor active user sessions
  - **Instrumented in:** `internal/storage/storage.go` (CreateSession/EndSession)

### Message Metrics

- **`chatbox_messages_received_total`** (Counter)
  - Total number of messages received from clients
  - Use this to track message throughput
  - **Instrumented in:** `internal/websocket/handler.go` (readPump)

- **`chatbox_messages_sent_total`** (Counter)
  - Total number of messages sent to clients
  - Use this to track outbound message volume
  - **Instrumented in:** `internal/websocket/handler.go` (writePump)

- **`chatbox_message_errors_total`** (Counter)
  - Total number of message processing errors
  - Use this to monitor error rates
  - **Instrumented in:** `internal/websocket/handler.go` (readPump)

### LLM Metrics

- **`chatbox_llm_requests_total{provider}`** (Counter)
  - Total number of LLM requests by provider
  - Labels: `provider` (openai, anthropic, dify)
  - Use this to track LLM usage by provider
  - **Instrumented in:** `internal/llm/llm.go` (SendMessage/StreamMessage)

- **`chatbox_llm_latency_seconds{provider}`** (Histogram)
  - Latency of LLM requests in seconds
  - Labels: `provider` (openai, anthropic, dify)
  - Use this to monitor LLM response times
  - For streaming requests, measures time to first token
  - **Instrumented in:** `internal/llm/llm.go` (SendMessage/StreamMessage)

- **`chatbox_llm_errors_total{provider}`** (Counter)
  - Total number of LLM errors by provider
  - Labels: `provider` (openai, anthropic, dify)
  - Use this to monitor LLM error rates
  - **Instrumented in:** `internal/llm/llm.go` (SendMessage/StreamMessage)

- **`chatbox_tokens_used_total{provider}`** (Counter)
  - Total number of LLM tokens used by provider
  - Labels: `provider` (openai, anthropic, dify)
  - Use this to track token consumption and costs
  - **Instrumented in:** `internal/llm/llm.go` (SendMessage)

### Session Metrics

- **`chatbox_sessions_created_total`** (Counter)
  - Total number of chat sessions created
  - Use this to track session creation rate
  - **Instrumented in:** `internal/storage/storage.go` (CreateSession)

- **`chatbox_sessions_ended_total`** (Counter)
  - Total number of chat sessions ended
  - Use this to track session completion rate
  - **Instrumented in:** `internal/storage/storage.go` (EndSession)

- **`chatbox_admin_takeovers_total`** (Counter)
  - Total number of admin session takeovers
  - Use this to monitor admin intervention frequency
  - **Instrumented in:** `internal/router/router.go` (HandleAdminTakeover)

## Instrumentation Details

All metrics are automatically collected throughout the application lifecycle:

1. **WebSocket Connections**: Tracked when connections are established and closed
2. **Messages**: Tracked when messages are received from clients and sent to clients
3. **LLM Requests**: Tracked for both streaming and non-streaming requests, including retries
4. **Sessions**: Tracked when sessions are created and ended
5. **Admin Actions**: Tracked when admins take over sessions

## Usage Example

### Prometheus Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'chatbox'
    static_configs:
      - targets: ['chatbox-service:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### Kubernetes HPA with Custom Metrics

You can use these metrics for Horizontal Pod Autoscaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: chatbox-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: chatbox
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metric:
        name: chatbox_websocket_connections_total
      target:
        type: AverageValue
        averageValue: "100"
```

## Instrumenting Code

To instrument your code with metrics, import the metrics package:

```go
import "github.com/real-rm/chatbox/internal/metrics"

// Increment a counter
metrics.MessagesReceived.Inc()

// Set a gauge value
metrics.WebSocketConnections.Set(float64(connectionCount))

// Record a histogram observation
metrics.LLMLatency.WithLabelValues("openai").Observe(duration.Seconds())

// Increment a counter with labels
metrics.LLMRequests.WithLabelValues("anthropic").Inc()
```

## Default Metrics

In addition to the custom metrics above, the Prometheus client library automatically exports:

- Go runtime metrics (memory, goroutines, GC stats)
- Process metrics (CPU, memory, file descriptors)
- HTTP metrics (if using promhttp handler)

These are useful for monitoring the health and performance of the application.

## Testing

The metrics package includes comprehensive tests to verify:
- All metrics are properly registered
- Counters increment correctly
- Gauges increase and decrease correctly
- Histograms record observations
- Labels work correctly for provider-specific metrics

Run tests with:
```bash
go test ./internal/metrics/... -v
```

