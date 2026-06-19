package app

import (
	grpcapp "chat/internal/app/grpc"
	ssogrpc "chat/internal/clients/sso/grpc"
	chatservice "chat/internal/services/chat"
	"chat/internal/storage/postgres"
	appredis "chat/internal/storage/redis"
	"context"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type App struct {
	GRPCSrv *grpcapp.App
	closers []func()
}

func New(
	log *slog.Logger,
	grpcPort int,
	postgresDSN string,
	ssoAddr string,
	jwtSecret string,
	redisUrl string,
	redisTTL time.Duration,
) *App {
	ctx := context.Background()
	pool := MustSetupPostgres(ctx, postgresDSN)

	storage := postgres.New(pool)

	ssoClient, err := ssogrpc.New(log, ssoAddr, 5*time.Second)
	if err != nil {
		log.Error("failed to init sso client", slog.Any("error", err))
		panic(err)
	}

	redisCli := MustSetupRedis(ctx, redisUrl)
	cache := appredis.New(redisCli, redisTTL)

	chatService := chatservice.New(log,
		storage,
		storage,
		storage,
		storage,
		ssoClient,
		cache,
	)
	grpcApp := grpcapp.New(log, chatService, grpcPort, jwtSecret)

	app := &App{
		GRPCSrv: grpcApp,
	}

	app.closers = append(app.closers, func() {
		log.Info("closing postgres connection pool")
		pool.Close()
	})
	app.closers = append(app.closers, func() {
		log.Info("closing redis client")
		redisCli.Close()
	})
	return app
}

// Stop stops all procesess and closes connections
func (a *App) Stop() {
	// Stop grpc requests
	a.GRPCSrv.Stop()

	// close all resources in reverse order (LIFO)
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

func MustSetupPostgres(ctx context.Context, dsn string) *pgxpool.Pool {
	pool, err := setupPostgres(ctx, dsn)
	if err != nil {
		panic("Failed to connect to postgres DB")
	}
	return pool
}

func setupPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return pool, nil
}

func setupRedis(ctx context.Context, redisUrl string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisUrl)
	if err != nil {
		return nil, err
	}
	redisCli := redis.NewClient(opt)

	if err := redisCli.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return redisCli, err
}

func MustSetupRedis(ctx context.Context, redisUrl string) *redis.Client {
	client, err := setupRedis(ctx, redisUrl)
	if err != nil {
		panic("Failed to setup redis")
	}

	return client
}
