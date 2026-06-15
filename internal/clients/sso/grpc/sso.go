package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	ssov1 "github.com/Ivan-Top-Pudge/protos/gen/go/sso"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Client struct {
	api     ssov1.AuthClient
	log     *slog.Logger
	timeout time.Duration
}

// New creates new connection to sso
func New(
	log *slog.Logger,
	addr string,
	timeout time.Duration,
) (*Client, error) {
	const op = "clients.sso.grpc.New"

	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Client{
		api:     ssov1.NewAuthClient(cc),
		log:     log.With(slog.String("component", "sso-client")),
		timeout: timeout,
	}, nil
}

// IsUserExists — implementation in usecase for IsUserExists
func (c *Client) IsUserExists(ctx context.Context, userID int64) (bool, error) {
	const op = "clients.sso.grpc.IsUserExists"

	// Создаем контекст с таймаутом специально для этого сетевого запроса
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Делаем реальный gRPC вызов по сети в SSO микросервис
	resp, err := c.api.IsUserExists(ctx, &ssov1.IsUserExistsRequest{
		UserId: userID,
	})
	if err != nil {
		// Обрабатываем gRPC статусы ошибок
		if st, ok := status.FromError(err); ok {
			if st.Code() == codes.InvalidArgument {
				c.log.Warn("invalid argument sent to sso", slog.Int64("user_id", userID))
				return false, fmt.Errorf("%s: %w", op, err)
			}
		}

		c.log.Error("failed to execute gRPC request to sso", slog.Any("error", err))
		return false, fmt.Errorf("%s: %w", op, err)
	}

	// Возвращаем чистый булев результат наверх в бизнес-логику
	return resp.GetExists(), nil
}
