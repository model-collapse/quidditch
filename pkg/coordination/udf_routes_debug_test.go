package coordination

import (
	"context"
	"testing"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestUDFRoutesRegistration tests that UDF routes are properly registered
func TestUDFRoutesRegistration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.CoordinationConfig{
		NodeID:     "coord-debug",
		BindAddr:   "127.0.0.1",
		RESTPort:   9299,
		MasterAddr: "127.0.0.1:8000",
	}

	node, err := NewCoordinationNode(cfg, logger)
	require.NoError(t, err)
	defer node.Stop(context.Background())

	t.Logf("UDF Registry initialized: %v", node.udfRegistry != nil)
	t.Logf("UDF Runtime initialized: %v", node.udfRuntime != nil)

	// Get all routes from Gin
	routes := node.ginRouter.Routes()

	t.Logf("Total routes registered: %d", len(routes))

	udfRouteCount := 0
	for _, route := range routes {
		t.Logf("Route: %s %s", route.Method, route.Path)
		if len(route.Path) >= 13 && route.Path[:13] == "/api/v1/udfs" {
			udfRouteCount++
			t.Logf("  ^^ UDF route found!")
		}
	}

	if node.udfRegistry != nil {
		require.Greater(t, udfRouteCount, 0, "UDF routes should be registered when UDF registry is available")
	}
}
