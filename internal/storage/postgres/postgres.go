package postgres

import (
	"chat/internal/domain/models"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Storage implements DB interface through pgx connection pool
type Storage struct {
	pool *pgxpool.Pool
}

// New creates new instance of Postgre Storage
func New(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}

// CreateChat creates new chat in db and adds members to this chat
func (s *Storage) CreateChat(
	ctx context.Context,
	members []int64,
) (int64, error) {
	// Init transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// defer guarantees that transation will be done fully
	// or canceled
	// If we had already called Commit(), Rollback will be ignored
	defer tx.Rollback(ctx)

	var chatID int64
	// Create chat in db
	err = tx.QueryRow(ctx,
		"INSERT INTO chats DEFAULT VALUES RETURNING id",
	).Scan(&chatID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert chat: %w", err)
	}

	for _, memberID := range members {
		_, err = tx.Exec(
			ctx,
			"INSERT INTO chat_members (chat_id, user_id) VALUES ($1, $2)",
			chatID, memberID)
		if err != nil {
			return 0, fmt.Errorf("failed to insert chat member %d: %w", memberID, err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return chatID, nil
}

func (s *Storage) DeleteChat(ctx context.Context, chatID int64) error {
	panic("Imlement me")
}

func (s *Storage) ChatExists(ctx context.Context, chatID int64) (bool, error) {
	panic("Imlement me")
}

func (s *Storage) SaveMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error) {
	panic("Imlement me")
}

func (s *Storage) DeleteMessage(ctx context.Context, msgID int64, chatID int64) error {
	panic("Imlement me")
}

func (s *Storage) GetHistory(ctx context.Context, chatID int64, limit int64, offset int64) ([]models.Message, error) {
	panic("Implement me")
}
