package fxapp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/config"
	grpcinfra "github.com/zgiai/zgi/api/internal/infra/grpc"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	redispkg "github.com/zgiai/zgi/api/pkg/redis"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const corsAllowOriginsEnvKey = "WEB_API_CORS_ALLOW_ORIGINS"

var corsAllowHeaders = []string{
	"Origin",
	"Content-Type",
	"Accept",
	"Authorization",
	"X-Requested-With",
	"X-Request-ID",
	"X-User-Account-Id",
	"Cache-Control",
	"Connection",
}

var defaultCORSAllowOrigins = []string{
	"http://localhost:2679",
	"http://localhost:3000",
	"http://localhost:3001",
	"http://localhost:3002",
}

type ServerAddresses struct {
	HTTP        string
	GRPC        int
	GRPCEnabled bool
}

type SentryResource struct {
	Enabled bool
}

type listenerResult struct {
	fx.Out

	HTTP net.Listener `name:"http_listener"`
	GRPC net.Listener `name:"grpc_listener"`
}

var infraModule = fx.Module("infra",
	fx.Provide(
		provideServerAddresses,
		provideSentryResource,
		provideOpenTelemetryResource,
		provideDatabase,
		provideRedis,
		providePlatformContainer,
		provideListeners,
		provideGinEngine,
		provideHTTPServer,
		provideGRPCServer,
	),
	fx.Invoke(registerInfraLifecycle),
)

func provideServerAddresses(cfg *config.Config) ServerAddresses {
	return ServerAddresses{
		HTTP:        fmt.Sprintf(":%d", cfg.Server.Port),
		GRPC:        cfg.Server.GRPCPort,
		GRPCEnabled: cfg.Server.GRPCEnabled,
	}
}

func provideSentryResource(cfg *config.Config, log *zap.Logger) *SentryResource {
	sentryDSN := strings.TrimSpace(cfg.Sentry.DSN)
	if sentryDSN == "" {
		log.Info("Sentry DSN not configured, skipping Sentry initialization")
		return &SentryResource{}
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		Environment:      cfg.Sentry.Environment,
		Release:          cfg.Sentry.Release,
		EnableTracing:    true,
		TracesSampleRate: 0.1,
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			if hint == nil || hint.Context == nil {
				return event
			}

			req, ok := hint.Context.Value(sentry.RequestContextKey).(*http.Request)
			if ok {
				event.Request = sentry.NewRequest(req)
			}
			return event
		},
	})
	if err != nil {
		log.Warn("Sentry initialization failed", zap.Error(err))
		return &SentryResource{}
	}

	log.Info("Sentry initialized successfully")
	return &SentryResource{Enabled: true}
}

func provideDatabase(cfg *config.Config, _ *zap.Logger) (*gorm.DB, error) {
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return nil, err
	}

	database.SetDB(db)
	return db, nil
}

func provideRedis(cfg *config.Config) (*redis.Client, error) {
	if err := redispkg.Init(cfg); err != nil {
		return nil, err
	}

	client := redispkg.GetClient()
	redispkg.SetClient(client)
	return client, nil
}

func providePlatformContainer(db *gorm.DB) (*platform.Container, error) {
	return platform.NewContainer(db)
}

func provideListeners(addresses ServerAddresses) (listenerResult, error) {
	httpListener, err := net.Listen("tcp", addresses.HTTP)
	if err != nil {
		return listenerResult{}, fmt.Errorf("failed to listen on %s: %w", addresses.HTTP, err)
	}

	if !addresses.GRPCEnabled {
		return listenerResult{HTTP: httpListener}, nil
	}

	grpcAddr := fmt.Sprintf(":%d", addresses.GRPC)
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		_ = httpListener.Close()
		return listenerResult{}, fmt.Errorf("failed to listen on %s: %w", grpcAddr, err)
	}

	return listenerResult{
		HTTP: httpListener,
		GRPC: grpcListener,
	}, nil
}

func provideGinEngine(cfg *config.Config, sentryResource *SentryResource, otelResource *OpenTelemetryResource) *gin.Engine {
	setGinMode(cfg.Server.Mode)

	engine := gin.New()
	engine.Use(middleware.RequestID())
	if otelResource != nil && otelResource.Enabled {
		engine.Use(otelgin.Middleware(cfg.OpenTelemetry.ServiceName))
		engine.Use(middleware.OpenTelemetryRequestAttributes())
	}
	engine.Use(middleware.Logger())
	engine.Use(middleware.AuditLogger())
	engine.Use(cors.New(cors.Config{
		AllowOrigins:     getCORSAllowOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     corsAllowHeaders,
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
	}))
	engine.Use(middleware.Recovery())

	if sentryResource.Enabled {
		engine.Use(sentrygin.New(sentrygin.Options{
			Repanic:         true,
			WaitForDelivery: false,
			Timeout:         2 * time.Second,
		}))
		engine.Use(middleware.SentryErrorReporter())
	}

	return engine
}

func provideHTTPServer(cfg *config.Config, addresses ServerAddresses, engine *gin.Engine) *http.Server {
	return &http.Server{
		Addr:           addresses.HTTP,
		Handler:        engine,
		ReadTimeout:    time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:   time.Duration(cfg.Server.WriteTimeout) * time.Second,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}
}

func provideGRPCServer(db *gorm.DB) *grpcinfra.Server {
	return grpcinfra.NewServer(db)
}

func registerInfraLifecycle(lc fx.Lifecycle, db *gorm.DB, redisClient *redis.Client) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			var result error

			if redisClient != nil {
				if err := redisClient.Close(); err != nil {
					result = errors.Join(result, err)
				}
			}

			sqlDB, err := db.DB()
			if err != nil {
				return errors.Join(result, err)
			}

			if err := sqlDB.Close(); err != nil {
				result = errors.Join(result, err)
			}

			return result
		},
	})
}

func setGinMode(mode string) {
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		gin.SetMode(mode)
	default:
		gin.SetMode(gin.DebugMode)
	}
}

func getCORSAllowOrigins() []string {
	cfg := config.Current()
	if len(cfg.Server.CORSAllowOrigins) > 0 {
		logger.Info("CORS origins configured",
			zap.String("source", corsAllowOriginsEnvKey),
			zap.Strings("origins", cfg.Server.CORSAllowOrigins),
		)
		return append([]string(nil), cfg.Server.CORSAllowOrigins...)
	}

	logger.Info("CORS default origins configured", zap.Strings("origins", defaultCORSAllowOrigins))
	return defaultCORSAllowOrigins
}
