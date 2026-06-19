package redis

import (
	"chat/internal/services/chat"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func New(client *redis.Client, ttl time.Duration) *Cache {
	return &Cache{
		client: client,
		ttl:    ttl,
	}
}

func (c *Cache) CheckChatMember(
	ctx context.Context,
	chatID int64,
	userID int64,
) (bool, error) {
	const op = "storage.Redis.CheckChatMember"

	val, err := c.client.Get(ctx, chatMemberKey(chatID, userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, chat.ErrCacheMiss
		}
		return false, fmt.Errorf("%s: %w", op, err)
	}

	isMember, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("%s: failed to parse bool: %w", op, err)
	}

	return isMember, nil
}

func (c *Cache) SetChatMember(
	ctx context.Context,
	chatID int64,
	userID int64,
	isMember bool,
) error {
	const op = "storage.Redis.SetChatMember"

	err := c.client.Set(
		ctx,
		chatMemberKey(chatID, userID),
		isMember,
		c.ttl,
	).Err()

	if err != nil {
		return fmt.Errorf("%s: %v", op, err)
	}

	return nil
}

// chatMemberKey returns redis key for chat member
func chatMemberKey(chatID int64, userID int64) string {
	return fmt.Sprintf("chat:%d:member:%d", chatID, userID)
}
