package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"

	"github.com/real-rm/chatbox/internal/constants"
)

// loadConfiguration loads the configuration and returns the config accessor
func loadConfiguration() (*goconfig.ConfigAccessor, error) {
	if err := goconfig.LoadConfig(); err != nil {
		return nil, err
	}

	cfg, err := goconfig.Default()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// initializeLogger initializes the logger with the given configuration
func initializeLogger(cfg *goconfig.ConfigAccessor) (*golog.Logger, error) {
	logDir, _ := cfg.ConfigStringWithDefault("log.dir", constants.DefaultLogDir)
	logLevel, _ := cfg.ConfigStringWithDefault("log.level", constants.DefaultLogLevel)
	standardOutput, _ := cfg.ConfigBoolWithDefault("log.standardOutput", true)

	logger, err := golog.InitLog(golog.LogConfig{
		Dir:            logDir,
		Level:          logLevel,
		StandardOutput: standardOutput,
		InfoFile:       "info.log",
		WarnFile:       "warn.log",
		ErrorFile:      "error.log",
	})
	if err != nil {
		return nil, err
	}

	return logger, nil
}

// getServerPort retrieves the server port from configuration
func getServerPort(cfg *goconfig.ConfigAccessor, logger *golog.Logger) int {
	port, _ := cfg.ConfigIntWithDefault("server.port", constants.DefaultPort)
	return port
}

// setupSignalHandler sets up signal handling for graceful shutdown
func setupSignalHandler() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

// runWithSignalChannel is a testable version of run that accepts a signal channel
func runWithSignalChannel(sigChan chan os.Signal) error {
	// Load configuration
	cfg, err := loadConfiguration()
	if err != nil {
		return err
	}

	// Initialize logger
	logger, err := initializeLogger(cfg)
	if err != nil {
		return err
	}
	defer logger.Close()

	// Get server port
	port := getServerPort(cfg, logger)
	logger.Info("Server starting", "port", port)

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutting down gracefully")

	return nil
}

// NewHTTPServer creates an HTTP server with production-safe timeout defaults.
// Use this when running chatbox as a standalone server (not via gomain).
func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  constants.HTTPReadTimeout,
		WriteTimeout: constants.HTTPWriteTimeout,
		IdleTimeout:  constants.HTTPIdleTimeout,
	}
}

func main() {
	if err := runMain(); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}

// runMain is the testable main function
func runMain() error {
	sigChan := setupSignalHandler()
	return runWithSignalChannel(sigChan)
}
