package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
)

func main() {
	// Load configuration using goconfig
	if err := goconfig.LoadConfig(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Get config accessor
	cfg, err := goconfig.Default()
	if err != nil {
		log.Fatalf("Failed to get config accessor: %v", err)
	}

	// Initialize golog logger
	logDir, _ := cfg.ConfigStringWithDefault("log.dir", "logs")
	logLevel, _ := cfg.ConfigStringWithDefault("log.level", "info")
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
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Get server port
	port, err := cfg.ConfigIntWithDefault("server.port", 8080)
	if err != nil {
		logger.Warn("Failed to get server port, using default", "error", err, "default_port", 8080)
	}

	logger.Info("Server starting", "port", port)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutting down gracefully")
}
