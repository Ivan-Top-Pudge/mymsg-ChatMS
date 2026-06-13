package chat

import (
	"context"
	"log/slog"
)

type Chat struct {
	log             *slog.Logger
	chatSaver       ChatSaver
	chatProvider    ChatProvider
	messageSaver    MessageSaver
	messageProvider MessageProvider
}

type ChatSaver interface {
	SaveChat(ctx context.Context, members []int64) (int64, error)
	DeleteChat(ctx context.Context, chatID int64) error
}

type ChatProvider interface {
	ChatExists(ctx context.Context, chatID int64) (bool, error)
}

type MessageSaver interface {
	SaveMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error)
	DeleteMessage(ctx context.Context, msgID int64, chatID int64) error
}

type MessageProvider interface {
	//TODO: GetHistory()
}

func New(
	log *slog.Logger,
	chatSaver ChatSaver,
	chatProvider ChatProvider,
	messageSaver MessageSaver,
	messageProvider MessageProvider,
) *Chat {
	return &Chat{
		log:             log,
		chatSaver:       chatSaver,
		chatProvider:    chatProvider,
		messageSaver:    messageSaver,
		messageProvider: messageProvider,
	}
}
