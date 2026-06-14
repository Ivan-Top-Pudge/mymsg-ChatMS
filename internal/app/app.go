package app

import (
	grpcapp "chat/internal/app/grpc"
	chatservice "chat/internal/services/chat"
	"chat/internal/storage/postgres"
	"context"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	GRPCSrv *grpcapp.App
}

func New(
	log *slog.Logger,
	grpcPort int,
	postgresDSN string,
) *App {
	ctx := context.Background()
	pool := MustSetupPostgres(ctx, postgresDSN)

	storage := postgres.New(pool)

	chatService := chatservice.New(log, storage, storage, storage, storage) // need storage
	grpcApp := grpcapp.New(log, chatService, grpcPort)
	return &App{
		GRPCSrv: grpcApp,
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
