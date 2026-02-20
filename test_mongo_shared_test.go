package chatbox

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/real-rm/goconfig"
	"github.com/real-rm/golog"
	"github.com/real-rm/gomongo"
)

var (
	rootMongoOnce   sync.Once
	rootMongoClient *gomongo.Mongo
	rootMongoLogger *golog.Logger
	rootMongoError  error
)

// getSharedRootMongoClient returns a shared MongoDB client for all root package tests.
// gomongo.InitMongoDB is a global singleton — this ensures it is called exactly once,
// preventing the 10s Ping timeout from being repeated for every test function.
func getSharedRootMongoClient(t *testing.T) *gomongo.Mongo {
	t.Helper()

	if testing.Short() || os.Getenv("SKIP_MONGO_TESTS") != "" {
		t.Skip("Skipping MongoDB-dependent test")
		return nil
	}

	rootMongoOnce.Do(func() {
		mongoURI := os.Getenv("MONGO_URI")
		if mongoURI == "" {
			mongoURI = "mongodb://chatbox:ChatBox123@127.0.0.1:27017/chatbox?authSource=admin"
		}

		configContent := fmt.Sprintf(`
[dbs]
verbose = 1
slowThreshold = 2

[dbs.chat]
uri = "%s"
`, mongoURI)

		tmpFile, err := os.CreateTemp("", "test_config_root_*.toml")
		if err != nil {
			rootMongoError = fmt.Errorf("failed to create temp config: %w", err)
			return
		}
		defer tmpFile.Close()

		if _, err = tmpFile.WriteString(configContent); err != nil {
			rootMongoError = fmt.Errorf("failed to write config: %w", err)
			return
		}

		os.Setenv("RMBASE_FILE_CFG", tmpFile.Name())
		goconfig.ResetConfig()
		if err = goconfig.LoadConfig(); err != nil {
			rootMongoError = fmt.Errorf("failed to load config: %w", err)
			return
		}

		configAccessor, err := goconfig.Default()
		if err != nil {
			rootMongoError = fmt.Errorf("failed to get config: %w", err)
			return
		}

		rootMongoLogger, err = golog.InitLog(golog.LogConfig{
			Level:          "error",
			StandardOutput: false,
			Dir:            "/tmp",
		})
		if err != nil {
			rootMongoError = fmt.Errorf("failed to init logger: %w", err)
			return
		}

		rootMongoClient, err = gomongo.InitMongoDB(rootMongoLogger, configAccessor)
		if err != nil {
			rootMongoError = fmt.Errorf("MongoDB not available: %w", err)
		} else {
			// Restore clean config state so subsequent tests can load their own configs.
			// goconfig.LoadConfig() is idempotent without ResetConfig(), so without this
			// all subsequent LoadConfig() calls would be no-ops, seeing only [dbs.chat].
			goconfig.ResetConfig()
		}
	})

	if rootMongoError != nil {
		t.Skipf("Skipping MongoDB test: %v", rootMongoError)
		return nil
	}

	return rootMongoClient
}
