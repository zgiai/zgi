package cache

import (
	"go.uber.org/fx"

	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/dataplane"
)

// Module provides a cache client if Redis is configured.
var Module = fx.Provide(func(conns *dataplane.Connections, cfg *config.Config) *Client {
	if conns == nil || conns.Cache == nil {
		return nil
	}
	return New(conns.Cache, cfg.RedisKeyPrefix)
})
