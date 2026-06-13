package chat

import (
	"context"

	"chat/internal/domain/models"

	chatv1 "github.com/Ivan-Top-Pudge/mymsg-protos/gen/go/chat"
	"google.golang.org/grpc"
)

// Это интерфейс для функций бизнес логики, который описывает контракт для gRPC
type Chat interface {
	CreateChat(ctx context.Context, members []int64) (int64, error)
	DeleteChat(ctx context.Context, chatID int64) error

	SendMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error)
	DeleteMessage(ctx context.Context, msgID int64, chatID int64) error

	GetChatHistory(ctx context.Context, chatID int64, limit int64, offset int64) ([]models.Message, error)
}

type serverAPI struct {
	chatv1.UnimplementedChatServer
	chat Chat
}

func Register(gRPC *grpc.Server, chat Chat) {
	chatv1.RegisterChatServer(gRPC, &serverAPI{chat: chat})
}
