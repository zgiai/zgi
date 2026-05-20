package platform

import (
	"fmt"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform/billing"
	"github.com/zgiai/zgi/api/internal/infra/platform/channel"
	"github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/infra/platform/identity"
	"gorm.io/gorm"
)

// Container holds all platform capability implementations.
// This is the central dependency injection point for platform services.
type Container struct {
	Billing  billing.BillingProvider
	Identity identity.IdentityProvider
	Channel  channel.ChannelProvider
	Console  console.ConsoleProvider
}

// NewContainer initializes the platform container based on application config.
// Platform edition controls the behavior:
// - "CLOUD": Use gRPC clients to communicate with zgi-console
// - "" or "SELF_HOSTED": Use standalone implementations
func NewContainer(db *gorm.DB) (*Container, error) {
	cfg := appconfig.Current()
	edition := cfg.Platform.Edition

	c := &Container{}

	if edition == "CLOUD" {
		// Console: Use remote HTTP client
		consoleURL := cfg.Console.APIURL
		if consoleURL == "" {
			consoleURL = "http://localhost:2625"
		}
		consoleAPIKey := cfg.Console.InternalAPIKey
		c.Console = console.NewRemote(consoleURL, consoleAPIKey)

		// Channel: HTTP + TTL cache (replaces gRPC stream)
		c.Channel = channel.NewCloud(consoleURL, consoleAPIKey)

		// Billing: Use remote gRPC (required in CLOUD mode)
		grpcAddr := cfg.Console.GRPCAddr
		if grpcAddr == "" {
			grpcAddr = "localhost:50051"
		}
		billingRemote, err := billing.NewRemote(grpcAddr)
		if err != nil {
			return nil, fmt.Errorf("cloud mode requires remote billing at %s: %w", grpcAddr, err)
		}
		c.Billing = billingRemote

		// Identity: Use standalone for now (JWT validation is local)
		c.Identity = identity.NewStandalone()
	} else {
		// Standalone/Open Source mode
		c.Billing = billing.NewStandaloneBilling()
		c.Identity = identity.NewStandalone()
		c.Channel = channel.NewStandalone(db)
		c.Console = console.NewStandalone()
	}

	return c, nil
}
