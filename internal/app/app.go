package app

import (
	grpcapp "chat/internal/app/grpc"
	ssogrpc "chat/internal/clients/sso/grpc"
	chatservice "chat/internal/services/chat"
	"chat/internal/storage/postgres"
	"context"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
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
) *App {
	ctx := context.Background()
	pool := MustSetupPostgres(ctx, postgresDSN)

	storage := postgres.New(pool)

	ssoClient, err := ssogrpc.New(log, ssoAddr, 5*time.Second)
	if err != nil {
		log.Error("failed to init sso client", slog.Any("error", err))
		panic(err)
	}

	chatService := chatservice.New(log,
		storage,
		storage,
		storage,
		storage,
		ssoClient,
	) // need storage
	grpcApp := grpcapp.New(log, chatService, grpcPort)

	app := &App{
		GRPCSrv: grpcApp,
	}

	app.closers = append(app.closers, func() {
		log.Info("closing postgres connection pool")
		pool.Close()
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
