package main

import (
	"chat/internal/app"
	"chat/internal/config"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad()

	log := setupLogger(cfg.Env)

	log.Info("starting application",
		slog.String("env", cfg.Env),
		slog.Any("cfg", cfg), slog.Int("port",
			cfg.GRPC.Port),
	)

	application := app.New(log,
		cfg.GRPC.Port,
		cfg.PostgresDSN,
		cfg.SSOAddr,
		cfg.JWTSecret,
		cfg.RedisUrl,
		cfg.RedisTTL,
	)

	go application.GRPCSrv.MustRun()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	// main будет висеть в этой строчке, пока не придёт сигнал
	// в это время горутина GRPCSrv.MustRun() обрабатывает запросы
	signal := <-stop
	log.Info("stopping application", slog.String("signal", signal.String()))
	application.GRPCSrv.Stop()
	log.Info("application stopped")

}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger

	switch env {
	case envLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	}
	return log
}
