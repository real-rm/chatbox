# Graceful Shutdown Implementation

This document describes the graceful shutdown implementation for the chatbox WebSocket service.

## Overview

The chatbox service implements graceful shutdown to ensure:
1. All active WebSocket connections are closed properly
2. Clients receive close messages before disconnection
3. Logs are flushed before the process exits
4. No data loss occurs during shutdown

## Implementation

### Signal Handling

The service listens for SIGTERM and SIGINT signals (handled by gomain or the main application):

```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

<-sigChan
// Call chatbox.Shutdown()
```

### Shutdown Process

When `chatbox.Shutdown(ctx)` is called:

1. **WebSocket Connection Closure**
   - All active WebSocket connections are identified
   - Close messages are sent to all connected clients
   - Connections are gracefully closed with proper cleanup

2. **Resource Cleanup**
   - Connection maps are cleared
   - Rate limiters are released
   - Channels are closed

3. **Log Flushing**
   - Final shutdown log messages are written
   - Logger is flushed (but not closed, as gomain handles that)

### Kubernetes Integration

The Kubernetes deployment is configured with:

```yaml
terminationGracePeriodSeconds: 30
```

This gives the service 30 seconds to complete graceful shutdown before Kubernetes forcefully terminates the pod.

### Health Check Endpoints

Two health check endpoints are provided:

#### `/chat/healthz` - Liveness Probe
- Checks if the application process is alive
- Returns 200 if the service can respond
- Kubernetes will restart the pod if this fails

#### `/chat/readyz` - Readiness Probe
- Checks if the application is ready to serve traffic
- Verifies MongoDB connection
- Returns 503 if dependencies are not ready
- Kubernetes will remove the pod from service endpoints if this fails

## Usage

### In gomain Integration

If using gomain, the shutdown is typically handled automatically. However, you can explicitly call:

```go
import "github.com/real-rm/chatbox"

// During shutdown
ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
defer cancel()

if err := chatbox.Shutdown(ctx); err != nil {
    log.Printf("Error during shutdown: %v", err)
}
```

### Standalone Usage

For standalone deployments:

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/real-rm/chatbox"
)

func main() {
    // ... initialize and start server ...
    
    // Setup graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    <-sigChan
    log.Println("Shutdown signal received")
    
    // Create shutdown context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
    defer cancel()
    
    // Shutdown chatbox service
    if err := chatbox.Shutdown(ctx); err != nil {
        log.Printf("Error during chatbox shutdown: %v", err)
    }
    
    // Shutdown HTTP server
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Error during server shutdown: %v", err)
    }
    
    log.Println("Server stopped")
}
```

## Testing Graceful Shutdown

### Manual Testing

1. Start the service
2. Connect a WebSocket client
3. Send SIGTERM to the process: `kill -TERM <pid>`
4. Verify the client receives a close message
5. Check logs for shutdown messages

### Kubernetes Testing

1. Deploy to Kubernetes
2. Connect WebSocket clients
3. Delete the pod: `kubectl delete pod <pod-name>`
4. Verify clients receive close messages
5. Check pod logs: `kubectl logs <pod-name>`

### Expected Behavior

When shutdown is triggered:
- Clients receive WebSocket close message with code 1001 (Going Away)
- Clients should attempt to reconnect (handled by client logic)
- New connections are rejected during shutdown
- Existing connections are closed within the grace period
- Pod terminates cleanly without force-kill

## Monitoring

### Logs

During shutdown, you'll see logs like:

```
INFO  Shutting down WebSocket handler, closing all connections
INFO  Closing WebSocket connection user_id=user-123
INFO  All WebSocket connections closed
INFO  Chatbox service shutdown complete
```

### Metrics

Monitor these metrics during shutdown:
- Active WebSocket connections (should go to 0)
- Connection close errors (should be minimal)
- Shutdown duration (should be < 30 seconds)

## Troubleshooting

### Connections Not Closing

If connections don't close properly:
- Check if `terminationGracePeriodSeconds` is sufficient
- Verify WebSocket close messages are being sent
- Check for network issues preventing close messages

### Force Termination

If pods are force-killed:
- Increase `terminationGracePeriodSeconds` in deployment.yaml
- Optimize shutdown logic to complete faster
- Check for blocking operations during shutdown

### Client Reconnection Issues

If clients don't reconnect after shutdown:
- Verify client implements reconnection logic
- Check if close code 1001 is handled properly
- Ensure load balancer/ingress routes to healthy pods

## Best Practices

1. **Always use graceful shutdown** - Never force-kill the process
2. **Set appropriate timeouts** - Balance between quick shutdown and connection cleanup
3. **Monitor shutdown metrics** - Track shutdown duration and errors
4. **Test regularly** - Include shutdown testing in CI/CD pipeline
5. **Handle client reconnection** - Implement exponential backoff in clients
6. **Log shutdown events** - Ensure visibility into shutdown process

## References

- [Kubernetes Pod Lifecycle](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/)
- [WebSocket Close Codes](https://developer.mozilla.org/en-US/docs/Web/API/CloseEvent)
- [Graceful Shutdown Patterns](https://cloud.google.com/blog/products/containers-kubernetes/kubernetes-best-practices-terminating-with-grace)
