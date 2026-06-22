package chat

import (
	"chat/internal/domain/models"
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var (
	ErrUserNotFound     = errors.New("user does not exist")
	ErrPermissionDenied = errors.New("permission denied")
	ErrCacheMiss        = errors.New("cache miss")
)

// Структура сервиса Chat с бизнес логикой
type Chat struct {
	log             *slog.Logger
	chatSaver       ChatSaver
	chatProvider    ChatProvider
	messageSaver    MessageSaver
	messageProvider MessageProvider
	ssoProvider     SSOProvider
	chatCache       ChatCache
}

type SSOProvider interface {
	IsUserExists(ctx context.Context, userID int64) (bool, error)
}

type ChatCache interface {
	CheckChatMember(ctx context.Context, chatID int64, userID int64) (bool, error)
	SetChatMember(ctx context.Context, chatID int64, userID int64, isMember bool) error
}

// Интерфейсы для логики storage`а
//
//go:generate mockgen -source=chat.go -destination=mocks/mock_chat.go -package=mocks
type ChatSaver interface {
	CreateChat(ctx context.Context, members []int64) (int64, error)
	DeleteChat(ctx context.Context, chatID int64) error
}

type ChatProvider interface {
	ChatExists(ctx context.Context, chatID int64) (bool, error)
	IsChatMember(ctx context.Context, chatID int64, requestorID int64) (bool, error)
}

type MessageSaver interface {
	SaveMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error)
	DeleteMessage(ctx context.Context, msgID int64, chatID int64) error
}

type MessageProvider interface {
	GetHistory(ctx context.Context, chatID int64, limit int64, offset int64) ([]models.Message, error)
	GetMessage(ctx context.Context, chatID int64, msgID int64) (models.Message, error)
}

// New создаёт новую сущность Сервиса Chat
func New(
	log *slog.Logger,
	chatSaver ChatSaver,
	chatProvider ChatProvider,
	messageSaver MessageSaver,
	messageProvider MessageProvider,
	ssoProvider SSOProvider,
	chatCache ChatCache,
) *Chat {
	return &Chat{
		log:             log,
		chatSaver:       chatSaver,
		chatProvider:    chatProvider,
		messageSaver:    messageSaver,
		messageProvider: messageProvider,
		ssoProvider:     ssoProvider,
		chatCache:       chatCache,
	}
}

// Реализация функций Бизнес-логики микросервиса

// CreateChat реализует бизнес логику создания чата
func (c *Chat) CreateChat(ctx context.Context, members []int64) (int64, error) {
	const op = "chat.CreateChat"

	// Check if members are valid
	c.log.With(slog.String("op", op)).Info("creating new chat")
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
			return 0, fmt.Errorf("user %d: %w", memberID, ErrUserNotFound)
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

	err := c.chatSaver.DeleteChat(ctx, chatID)
	if err != nil {
		c.log.Error("failed to delete chat", slog.String("error", err.Error()))
		return err
	}
	return nil
}

// SendMessage creates a new message in chat with chatID
// senderID is validated on gRPC level
func (c *Chat) SendMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error) {
	const op = "chat.SendMessage"

	isMember, err := c.chatCache.CheckChatMember(ctx, chatID, senderID)

	if err != nil {
		if !errors.Is(err, ErrCacheMiss) {
			c.log.Warn("failed to check in redis, checking in db", slog.String("error", err.Error()))
		}
		// storage request
		isMember, err = c.chatProvider.IsChatMember(ctx, chatID, senderID)
		if err != nil {
			c.log.Error("failed to check chat member in DB", slog.String("error", err.Error()))
			return 0, fmt.Errorf("%s: %w", op, err)
		}

		// кэшируем запрос, если удачно сходили в бд
		err = c.chatCache.SetChatMember(ctx, chatID, senderID, isMember)
		if err != nil {
			c.log.Warn("failed to save chat member to cache", slog.String("error", err.Error()))
		}
	}

	if !isMember {
		return 0, ErrPermissionDenied
	}

	msgID, err := c.messageSaver.SaveMessage(ctx, chatID, senderID, text)
	if err != nil {
		c.log.Error("failed to save message in db", slog.String("error", err.Error()))
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return msgID, nil
}

// DeleteMessage deletes the message with msgID in chat with chatID
func (c *Chat) DeleteMessage(ctx context.Context, msgID int64, chatID int64, requestorID int64) error {
	const op = "chat.DeleteMessage"

	log := c.log.With(
		slog.String("op", op),
		slog.Int64("msg_id", msgID),
		slog.Int64("chat_id", chatID),
		slog.Int64("requestor_id", requestorID),
	)

	msg, err := c.messageProvider.GetMessage(ctx, chatID, msgID)
	if err != nil {
		log.Error("failed to get message", slog.String("error", err.Error()))
		return fmt.Errorf("%s: %w", op, err)
	}

	if msg.SenderID != requestorID {
		log.Warn("permission denied: requestor is not the sender",
			slog.Int64("actual_sender_id", msg.SenderID),
		)
		return fmt.Errorf("%s: %w", op, ErrPermissionDenied)
	}

	err = c.messageSaver.DeleteMessage(ctx, msgID, chatID)
	if err != nil {
		log.Error("failed to delete message", slog.String("error", err.Error()))
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// GetChatHistory return array of messages from chat with chatID using limit and offset
func (c *Chat) GetChatHistory(ctx context.Context, chatID int64, requestorID int64, limit int64, offset int64) ([]models.Message, error) {
	const op = "chat.GetChatHistory"

	isMember, err := c.chatProvider.IsChatMember(ctx, chatID, requestorID)
	if err != nil {
		c.log.Error("failed to get chat", slog.String("error", err.Error()))
		return nil, err
	}

	if !isMember {
		c.log.Warn("permission denied: requestor is not in the chat")
		return nil, fmt.Errorf("%s: %w", op, ErrPermissionDenied)
	}

	messages, err := c.messageProvider.GetHistory(ctx, chatID, limit, offset)
	if err != nil {
		c.log.Error("failed to get history", slog.String("error", err.Error()))
		return nil, err
	}
	return messages, nil
}
