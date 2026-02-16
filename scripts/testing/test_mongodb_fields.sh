#!/bin/bash
# Test script to verify MongoDB operations with new camelCase field names

set -e

echo "Testing MongoDB operations with new field names..."
echo ""

# Check if MongoDB is running
if ! docker ps | grep -q chatbox-mongodb; then
    echo "Error: MongoDB container is not running"
    echo "Please start MongoDB with: docker-compose up -d mongodb"
    exit 1
fi

echo "✓ MongoDB is running"
echo ""

# Create a temporary config file for testing
TEMP_CONFIG=$(mktemp /tmp/test_config_XXXXXX.toml)
cat > "$TEMP_CONFIG" << 'EOF'
[dbs]
verbose = 1
slowThreshold = 2

[dbs.test_chat_db]
uri = "mongodb://localhost:27017/test_chat_db"
EOF

echo "✓ Created temporary config file: $TEMP_CONFIG"
echo ""

# Export config file path
export CONFIG_FILE="$TEMP_CONFIG"
export MONGO_TEST_URI="mongodb://localhost:27017"

# Run storage tests
echo "Running storage tests..."
echo ""

go test -v ./internal/storage \
    -run "^Test(CreateSession|UpdateSession|GetSession|AddMessage|EndSession|ListUserSessions|ListAllSessions)" \
    -timeout 60s

TEST_EXIT_CODE=$?

# Cleanup
rm -f "$TEMP_CONFIG"
unset CONFIG_FILE
unset MONGO_TEST_URI

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✓ All MongoDB tests passed!"
    echo ""
    echo "MongoDB operations are working correctly with new camelCase field names:"
    echo "  - uid (user ID)"
    echo "  - nm (name)"
    echo "  - msgs (messages)"
    echo "  - ts (timestamp/start time)"
    echo "  - endTs (end timestamp)"
    echo "  - dur (duration)"
    echo "  - adminAssisted"
    echo "  - assistingAdminId"
    echo "  - helpRequested"
    echo "  - totalTokens"
    echo "  - maxRespTime (max response time)"
    echo "  - avgRespTime (avg response time)"
else
    echo "✗ Some tests failed"
    exit 1
fi
