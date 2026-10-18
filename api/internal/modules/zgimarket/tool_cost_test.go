package zgimarket

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool whose one call can cost more than the instance lets a tool spend unasked is costly. The
// cap is the market's, read live with the offering, so a price change is followed like a version.
func TestAToolIsCostlyAboveWhatTheInstanceLetsItSpendUnasked(t *testing.T) {
	fixture := newToolsFixture(t, true)
	fixture.market.items = true
	ctx := context.Background()
	path := "tools/subjects/agent/" + fixture.agent.String()
	status, answer := fixture.send(t, http.MethodPut, path, `{"items":[{"listing_id":20,"offering_slug":"items-standard"}]}`)
	require.Equal(t, http.StatusOK, status, answer)

	clock := time.Now()
	fixture.tools.now = func() time.Time { return clock }
	publish := func(caps ...map[string]any) {
		listing := itemsListing()
		if caps != nil {
			listing["price_caps"] = caps
		}
		fixture.gateway.mu.Lock()
		fixture.gateway.listing = listing
		fixture.gateway.mu.Unlock()
		// Past the life of what was read before.
		clock = clock.Add(2 * toolCatalogTTL)
	}
	costs := func() (int64, bool) {
		t.Helper()
		listed, err := fixture.tools.SubjectTools(ctx, fixture.org, SubjectAgent, fixture.agent)
		require.NoError(t, err)
		require.NotEmpty(t, listed)
		for _, tool := range listed {
			assert.Equal(t, listed[0].MaxChargeMicros, tool.MaxChargeMicros, "every operation of an offering shares its cap")
			assert.Equal(t, listed[0].Costly, tool.Costly)
		}
		return listed[0].MaxChargeMicros, listed[0].Costly
	}

	// The market publishes no cap: nothing is known, nothing asks for cost.
	fixture.tools.confirmAboveMicros = 100_000
	micros, costly := costs()
	assert.Zero(t, micros)
	assert.False(t, costly)

	// 150 points a call is above 100.
	publish(map[string]any{"offering_slug": "items-chat", "max_charge_micros": 9_000_000},
		map[string]any{"offering_slug": "items-standard", "max_charge_micros": 150_000})
	micros, costly = costs()
	assert.EqualValues(t, 150_000, micros, "the cap of this offering, not of another one of the listing")
	assert.True(t, costly)

	// Exactly the threshold is not above it, and a lower price is followed.
	publish(map[string]any{"offering_slug": "items-standard", "max_charge_micros": 100_000})
	micros, costly = costs()
	assert.EqualValues(t, 100_000, micros)
	assert.False(t, costly)

	// Zero never asks for cost, whatever a call can cost.
	fixture.tools.confirmAboveMicros = 0
	publish(map[string]any{"offering_slug": "items-standard", "max_charge_micros": 9_000_000})
	micros, costly = costs()
	assert.EqualValues(t, 9_000_000, micros)
	assert.False(t, costly)

	// A negative figure is no cap.
	fixture.tools.confirmAboveMicros = 100_000
	publish(map[string]any{"offering_slug": "items-standard", "max_charge_micros": -5})
	micros, costly = costs()
	assert.Zero(t, micros)
	assert.False(t, costly)
}

func TestTheThresholdComesFromTheGatewaySettingsInPoints(t *testing.T) {
	tools, err := NewTools(nil, nil, &GatewayConfig{ConfirmAbovePoints: 100})
	require.NoError(t, err)
	assert.EqualValues(t, 100_000, tools.confirmAboveMicros)
	assert.True(t, tools.costly(100_001))
	assert.False(t, tools.costly(100_000))

	off, err := NewTools(nil, nil, &GatewayConfig{ConfirmAbovePoints: -3})
	require.NoError(t, err)
	assert.Zero(t, off.confirmAboveMicros)
	assert.False(t, off.costly(1<<40))
}
