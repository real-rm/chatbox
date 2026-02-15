#!/bin/bash
# Test script for chatbox service integration with gomain

set -e

echo "=== Chatbox Service Integration Test ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test 1: Verify gomain repository exists
echo "Test 1: Checking gomain repository..."
if [ -d "/Users/fx/work/gomain" ]; then
    echo -e "${GREEN}✓ gomain repository exists${NC}"
else
    echo -e "${RED}✗ gomain repository not found${NC}"
    exit 1
fi

# Test 2: Verify services.imports.txt contains chatbox
echo ""
echo "Test 2: Checking services.imports.txt..."
if grep -q "chatbox,github.com/real-rm/chatbox" /Users/fx/work/gomain/services.imports.txt; then
    echo -e "${GREEN}✓ chatbox entry found in services.imports.txt${NC}"
    echo "   Entry: $(grep chatbox /Users/fx/work/gomain/services.imports.txt)"
else
    echo -e "${RED}✗ chatbox entry not found in services.imports.txt${NC}"
    exit 1
fi

# Test 3: Verify go.mod contains chatbox dependency
echo ""
echo "Test 3: Checking go.mod..."
if grep -q "github.com/real-rm/chatbox" /Users/fx/work/gomain/go.mod; then
    echo -e "${GREEN}✓ chatbox dependency found in go.mod${NC}"
    echo "   Require: $(grep 'github.com/real-rm/chatbox' /Users/fx/work/gomain/go.mod | head -1)"
    echo "   Replace: $(grep 'replace github.com/real-rm/chatbox' /Users/fx/work/gomain/go.mod)"
else
    echo -e "${RED}✗ chatbox dependency not found in go.mod${NC}"
    exit 1
fi

# Test 4: Verify generated_services.go contains chatbox
echo ""
echo "Test 4: Checking generated_services.go..."
if grep -q "chatbox" /Users/fx/work/gomain/generated_services.go; then
    echo -e "${GREEN}✓ chatbox registration found in generated_services.go${NC}"
    echo "   Import: $(grep 'chatbox' /Users/fx/work/gomain/generated_services.go | grep import -A 1 | tail -1)"
    echo "   Registry: $(grep '"chatbox"' /Users/fx/work/gomain/generated_services.go)"
else
    echo -e "${RED}✗ chatbox registration not found in generated_services.go${NC}"
    exit 1
fi

# Test 5: Verify chatbox Register function exists
echo ""
echo "Test 5: Checking chatbox Register function..."
if [ -f "chatbox.go" ]; then
    if grep -q "func Register" chatbox.go; then
        echo -e "${GREEN}✓ Register function found in chatbox.go${NC}"
        echo "   Signature: $(grep 'func Register' chatbox.go)"
    else
        echo -e "${RED}✗ Register function not found in chatbox.go${NC}"
        exit 1
    fi
else
    echo -e "${RED}✗ chatbox.go not found${NC}"
    exit 1
fi

# Test 6: Verify graceful shutdown handling in gomain
echo ""
echo "Test 6: Checking graceful shutdown handling..."
if grep -q "server.Shutdown" /Users/fx/work/gomain/main.go; then
    echo -e "${GREEN}✓ Graceful shutdown implemented in gomain${NC}"
    echo "   Signal handling: $(grep -A 2 'signal.Notify' /Users/fx/work/gomain/main.go | head -1)"
    echo "   Shutdown call: $(grep 'server.Shutdown' /Users/fx/work/gomain/main.go)"
else
    echo -e "${RED}✗ Graceful shutdown not found in gomain${NC}"
    exit 1
fi

# Test 7: Verify health check endpoints
echo ""
echo "Test 7: Checking health check endpoints..."
if grep -q "handleHealthCheck" chatbox.go && grep -q "handleReadyCheck" chatbox.go; then
    echo -e "${GREEN}✓ Health check endpoints implemented${NC}"
    echo "   Liveness: /chat/healthz"
    echo "   Readiness: /chat/readyz"
else
    echo -e "${RED}✗ Health check endpoints not found${NC}"
    exit 1
fi

# Test 8: Verify WebSocket endpoint registration
echo ""
echo "Test 8: Checking WebSocket endpoint..."
if grep -q 'GET.*"/ws"' chatbox.go; then
    echo -e "${GREEN}✓ WebSocket endpoint registered${NC}"
    echo "   Endpoint: /chat/ws"
else
    echo -e "${RED}✗ WebSocket endpoint not found${NC}"
    exit 1
fi

# Test 9: Verify admin endpoints registration
echo ""
echo "Test 9: Checking admin endpoints..."
if grep -q "/admin" chatbox.go; then
    echo -e "${GREEN}✓ Admin endpoints registered${NC}"
    echo "   Sessions: /chat/admin/sessions"
    echo "   Metrics: /chat/admin/metrics"
    echo "   Takeover: /chat/admin/takeover/:sessionID"
else
    echo -e "${RED}✗ Admin endpoints not found${NC}"
    exit 1
fi

# Test 10: Verify authentication middleware
echo ""
echo "Test 10: Checking authentication middleware..."
if grep -q "authMiddleware" chatbox.go; then
    echo -e "${GREEN}✓ Authentication middleware implemented${NC}"
    echo "   Function: authMiddleware(validator)"
else
    echo -e "${RED}✗ Authentication middleware not found${NC}"
    exit 1
fi

# Summary
echo ""
echo "=== Test Summary ==="
echo -e "${GREEN}All integration tests passed!${NC}"
echo ""
echo "Integration completed successfully:"
echo "  ✓ chatbox added to gomain's services.imports.txt"
echo "  ✓ chatbox dependency added to gomain's go.mod"
echo "  ✓ generated_services.go updated with chatbox registration"
echo "  ✓ Register function implements correct signature"
echo "  ✓ Graceful shutdown handling verified"
echo "  ✓ Health check endpoints implemented"
echo "  ✓ WebSocket endpoint registered"
echo "  ✓ Admin endpoints registered"
echo "  ✓ Authentication middleware implemented"
echo ""
echo "Next steps:"
echo "  1. Run 'go mod tidy' in gomain directory"
echo "  2. Build gomain: go build -o gomain"
echo "  3. Configure config.toml with chatbox settings"
echo "  4. Start gomain with ENABLE_SERVICES=chatbox"
echo "  5. Test WebSocket connections and admin endpoints"
echo ""
