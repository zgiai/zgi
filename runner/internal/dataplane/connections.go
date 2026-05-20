package dataplane

import (
	"context"
	"database/sql"

	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/runner/internal/config"
)

type connectionParams struct {
	fx.In
	Lifecycle fx.Lifecycle
	Config    *config.Config
	Log       *zap.Logger
}

type connectionResult struct {
	fx.Out
	Connections *Connections
	ORM         *gorm.DB
	SQL         *sql.DB `name:"controlPlaneDB"`
	Cache       redis.UniversalClient
}

// NewConnections initializes database and cache clients based on config.
func NewConnections(p connectionParams) (connectionResult, error) {
	conns := &Connections{}
	result := connectionResult{
		Connections: conns,
	}

	if p.Config.DatabaseEnabled() {
		ormDB, sqlDB, err := initDatabase(p.Config, p.Log)
		if err != nil {
			return result, err
		}
		conns.ORM = ormDB
		conns.SQL = sqlDB
		result.ORM = ormDB
		result.SQL = sqlDB
		p.Lifecycle.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return sqlDB.Close()
			},
		})
	} else {
		p.Log.Info("data-plane database not configured; skipping DB init")
	}

	if p.Config.RedisEnabled() {
		cache, err := initRedis(p.Config, p.Log)
		if err != nil {
			return result, err
		}
		conns.Cache = cache
		result.Cache = cache
		p.Lifecycle.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				return cache.Close()
			},
		})
	} else {
		p.Log.Info("redis not configured; skipping cache init")
	}

	return result, nil
}
