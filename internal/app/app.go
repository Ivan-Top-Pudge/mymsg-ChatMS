package app

import (
	grpcapp "chat/internal/app/grpc"
	chatservice "chat/internal/services/chat"
	"log/slog"
	"time"
)

type App struct {
	GRPCSrv *grpcapp.App
}

func New(
	log *slog.Logger,
	grpcPort int,
	storagePath string,
	tokenTTL time.Duration,
) *App {
	// TODO: init storage

	chatService := chatservice.New(log) // need storage
	grpcApp := grpcapp.New(log, chatService, grpcPort)
	return &App{
		GRPCSrv: grpcApp,
	}
}
