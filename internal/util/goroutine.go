package util

import (
	"fmt"

	"github.com/real-rm/chatbox/internal/metrics"
	"github.com/real-rm/golog"
)

// SafeGo launches a goroutine with panic recovery.
// If the goroutine panics, the panic is recovered, logged, and the error metric is incremented.
// This prevents a single goroutine panic from crashing the entire process.
func SafeGo(logger *golog.Logger, component string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered in goroutine",
					"component", component,
					"panic", fmt.Sprintf("%v", r))
				metrics.MessageErrors.Inc()
			}
		}()
		fn()
	}()
}
