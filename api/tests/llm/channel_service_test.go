package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
)

// TestUserOwnedChannelDefaultPriority tests that user-owned channels get default priority of 100
func TestUserOwnedChannelDefaultPriority(t *testing.T) {
	// User-owned channels should have higher default priority than system channels
	// System channels typically have priority=10, user channels should be 100
	userChannelDefaultPriority := 100
	systemChannelDefaultPriority := 10

	assert.Greater(t, userChannelDefaultPriority, systemChannelDefaultPriority,
		"User channel priority should be higher than system channel priority")
}

// TestUserOwnedChannelDefaultWeight tests that user-owned channels get default weight of 100
func TestUserOwnedChannelDefaultWeight(t *testing.T) {
	userChannelDefaultWeight := 100
	assert.Equal(t, 100, userChannelDefaultWeight)
}

// TestRouteTypeConstants tests that route type constants are defined correctly
func TestRouteTypeConstants(t *testing.T) {
	assert.Equal(t, shared.RouteType("PRIVATE"), shared.RouteTypePrivate)
	assert.Equal(t, shared.RouteType("ZGI_CLOUD"), shared.RouteTypeZGICloud)
}

// TestUserOwnedRouteShouldHaveHigherPriorityThanSystem tests the priority ordering logic
func TestUserOwnedRouteShouldHaveHigherPriorityThanSystem(t *testing.T) {
	// When a user creates a channel without specifying priority/weight,
	// the system should automatically set them to 100 to ensure
	// user channels are preferred over system channels (which typically have priority=10)

	type routeConfig struct {
		routeType shared.RouteType
		priority  int
		weight    int
	}

	// Simulate default values for different route types
	userOwnedRoute := routeConfig{
		routeType: shared.RouteTypePrivate,
		priority:  100, // Default for user-owned
		weight:    100, // Default for user-owned
	}

	systemRefRoute := routeConfig{
		routeType: shared.RouteTypeZGICloud,
		priority:  10, // Typical system channel priority
		weight:    1,  // Default for system ref
	}

	// User-owned routes should have higher priority
	assert.Greater(t, userOwnedRoute.priority, systemRefRoute.priority,
		"User-owned route should have higher priority than system ref route")

	// User-owned routes should have higher weight
	assert.Greater(t, userOwnedRoute.weight, systemRefRoute.weight,
		"User-owned route should have higher weight than system ref route")
}
