package chat

import (
	"chat/internal/domain/models"
	"context"
	"fmt"
	"log/slog"
)

// Структура сервиса Chat с бизнес логикой
type Chat struct {
	log             *slog.Logger
	chatSaver       ChatSaver
	chatProvider    ChatProvider
	messageSaver    MessageSaver
	messageProvider MessageProvider
	ssoProvider     SSOProvider
}

type SSOProvider interface {
	IsUserExists(ctx context.Context, userID int64) (bool, error)
}

// Интерфейсы для логики storage`а

type ChatSaver interface {
	CreateChat(ctx context.Context, members []int64) (int64, error)
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
	GetHistory(ctx context.Context, chatID int64, limit int64, offset int64) ([]models.Message, error)
}

// New создаёт новую сущность Сервиса Chat
func New(
	log *slog.Logger,
	chatSaver ChatSaver,
	chatProvider ChatProvider,
	messageSaver MessageSaver,
	messageProvider MessageProvider,
	ssoProvider SSOProvider,
) *Chat {
	return &Chat{
		log:             log,
		chatSaver:       chatSaver,
		chatProvider:    chatProvider,
		messageSaver:    messageSaver,
		messageProvider: messageProvider,
		ssoProvider:     ssoProvider,
	}
}

// Реализация функций Бизнес-логики микросервиса

// CreateChat реализует бизнес логику создания чата
func (c *Chat) CreateChat(ctx context.Context, members []int64) (int64, error) {
	const op = "chat.CreateChat"

	c.log.With(slog.String("op", op)).Info("creating new chat")
	// TODO: проверка SSO
	for _, memberID := range members {
		exists, err := c.ssoProvider.IsUserExists(ctx, memberID)
		if err != nil {
			c.log.With(slog.String("op", op)).Error(
				"failed to check user existence in sso",
				slog.Int64("user_id", memberID),
				slog.Any("error", err),
			)
			return 0, fmt.Errorf("%s: failed to validate member %d: %w", op, memberID, err)
		}

		if !exists {
			c.log.Warn("attempt to create chat with non-existent user", slog.Int64("user_id", memberID))
			return 0, fmt.Errorf("%s: user %d does not exist", op, memberID)
		}
	}

	chatID, err := c.chatSaver.CreateChat(ctx, members)
	if err != nil {
		c.log.Error("failed to save chat to db", slog.String("error", err.Error()))
		return 0, err
	}
	return chatID, nil
}

// DeleteChat deletes a chat with given chatID (business logic)
func (c *Chat) DeleteChat(ctx context.Context, chatID int64) error {
	const op = "chat.DeleteChat"
	c.log.With(slog.String("op", op)).Info("deleting a chat")
	// TODO: проверка SSO

	err := c.chatSaver.DeleteChat(ctx, chatID)
	if err != nil {
		c.log.Error("failed to delete chat", slog.String("error", err.Error()))
		return err
	}
	return nil
}

// SendMessage creates a new message in chat with chatID
func (c *Chat) SendMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error) {
	const op = "chat.SendMessage"
	// TODO: SSO

	msgID, err := c.messageSaver.SaveMessage(ctx, chatID, senderID, text)
	if err != nil {
		c.log.Error("failed to send message", slog.String("error", err.Error()))
		return 0, err
	}
	return msgID, nil
}

// DeleteMessage deletes the message with msgID in chat with chatID
func (c *Chat) DeleteMessage(ctx context.Context, msgID int64, chatID int64) error {
	const op = "chat.DeleteMessage"
	// TODO: SSO

	err := c.messageSaver.DeleteMessage(ctx, msgID, chatID)
	if err != nil {
		c.log.Error("failed to delete message", slog.String("error", err.Error()))
		return err
	}
	return nil
}

// GetChatHistory return array of messages from chat with chatID using limit and offset
func (c *Chat) GetChatHistory(ctx context.Context, chatID int64, limit int64, offset int64) ([]models.Message, error) {
	const op = "chat.GetChatHistory"
	// TODO: SSO

	messages, err := c.messageProvider.GetHistory(ctx, chatID, limit, offset)
	if err != nil {
		c.log.Error("failed to get history", slog.String("error", err.Error()))
		return nil, err
	}
	return messages, nil
}
