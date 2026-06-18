package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(
	t *testing.T,
	ctx context.Context,
) (*pgxpool.Pool, func()) {
	// Поднимаем контейнер с постгре 17
	pgContainer, err := postgres.Run(ctx,
		"postgres:17",
		postgres.WithDatabase("chat_test"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	require.NoError(t, err)

	// Получение строки подключения к дб
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Накат миграций
	m, err := migrate.New("file://../../../migrations", connStr)
	require.NoError(t, err)

	if err = m.Up(); err != nil && err != migrate.ErrNoChange {
		require.NoError(t, err)
	}

	// Создаем пул соединений для тест хранилища
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	// Создания клинап функции для вызова в тестах
	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	return pool, cleanup
}

func TestStorage_CreateChat(t *testing.T) {
	ctx := context.Background()

	// сетапим бд
	pool, cleanup := setupTestDB(t, ctx)
	defer cleanup()

	storage := New(pool)

	members := []int64{1, 2}
	chatID, err := storage.CreateChat(ctx, members)
	require.NoError(t, err)
	require.NotZero(t, chatID)

	var chatCount int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM chats WHERE id = $1",
		chatID).Scan(&chatCount)
	require.NoError(t, err)
	require.Equal(t, 1, chatCount,
		"Chat row should exist in chats table")

	// Проверка 2: Корректно ли отработала транзакция и сохранились ли связи в chat_members
	var membersCount int
	err = pool.QueryRow(ctx,
		"SELECT count(*) FROM chat_members WHERE chat_id = $1",
		chatID).Scan(&membersCount)
	require.NoError(t, err)
	require.Equal(t, len(members),
		membersCount,
		"All members should be saved in chat_members table")
}
