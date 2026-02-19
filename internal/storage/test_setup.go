package storage

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// Shared MongoDB client for all tests
	sharedMongoClient *gomongo.Mongo
	sharedLogger      *golog.Logger
	mongoInitOnce     sync.Once
	mongoInitError    error
)

// getSharedMongoClient returns a shared MongoDB client for all tests
// This avoids the "MongoDB has already been initialized" error
func getSharedMongoClient(t *testing.T) (*gomongo.Mongo, *golog.Logger) {
	mongoInitOnce.Do(func() {
		// Check if we should skip MongoDB tests
		if os.Getenv("SKIP_MONGO_TESTS") != "" {
			mongoInitError = fmt.Errorf("SKIP_MONGO_TESTS is set")
			return
		}

		// Get MongoDB URI from environment or use default test configuration
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			// Default test configuration from test.md
			mongoURI = "mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin"
		}

		// Create temporary config file
		configContent := fmt.Sprintf(`
[dbs]
verbose = 1
slowThreshold = 2

[dbs.chatbox]
uri = "%s"
`, mongoURI)

		tmpFile, err := os.CreateTemp("", "test_config_shared_*.toml")
		if err != nil {
			mongoInitError = fmt.Errorf("failed to create temp config: %w", err)
			return
		}
		defer tmpFile.Close()

		_, err = tmpFile.WriteString(configContent)
		if err != nil {
			mongoInitError = fmt.Errorf("failed to write config: %w", err)
			return
		}

		// Set config file path
		os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())

		// Reset and load config
		goconfig.ResetConfig()
		err = goconfig.LoadConfig()
		if err != nil {
			mongoInitError = fmt.Errorf("failed to load config: %w", err)
			return
		}

		configAccessor, err := goconfig.Default()
		if err != nil {
			mongoInitError = fmt.Errorf("failed to get config accessor: %w", err)
			return
		}

		// Initialize logger
		sharedLogger, err = golog.InitLog(golog.LogConfig{
			Level:          "info",
			StandardOutput: true,
			Dir:            "/tmp",
		})
		if err != nil {
			mongoInitError = fmt.Errorf("failed to initialize logger: %w", err)
			return
		}

		// Initialize MongoDB client
		sharedMongoClient, err = gomongo.InitMongoDB(sharedLogger, configAccessor)
		if err != nil {
			mongoInitError = fmt.Errorf("failed to initialize MongoDB: %w", err)
			return
		}

		// Test connection
		testColl := sharedMongoClient.Coll("chatbox", "test_connection")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = testColl.InsertOne(ctx, bson.M{"test": "connection"})
		if err != nil {
			mongoInitError = fmt.Errorf("failed to verify connection: %w", err)
			return
		}
	})

	if mongoInitError != nil {
		t.Skipf("Skipping MongoDB tests: %v", mongoInitError)
		return nil, nil
	}

	return sharedMongoClient, sharedLogger
}

// setupTestStorageShared creates a test storage service using the shared MongoDB client
// This avoids MongoDB initialization conflicts between tests
func setupTestStorageShared(t *testing.T) (*StorageService, func()) {
	mongoClient, logger := getSharedMongoClient(t)
	if mongoClient == nil {
		return nil, func() {}
	}

	// Create storage service with unique collection name per test
	collectionName := fmt.Sprintf("test_sessions_%d_%s", time.Now().UnixNano(), t.Name())
	service := NewStorageService(mongoClient, "chatbox", collectionName, logger, nil)

	// Return cleanup function
	cleanup := func() {
		// Drop test collection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		db, _ := mongoClient.Database("chatbox")
		if db != nil {
			db.Coll(collectionName).Drop(ctx)
		}
	}

	return service, cleanup
}
