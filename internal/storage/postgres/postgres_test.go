package postgres

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

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

	if err != nil {
		log.Fatalf("failed to start container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %v", err)
	}

	// 2. Накатываем миграции (один раз создаем структуру таблиц)
	migrator, err := migrate.New("file://../../../migrations", connStr)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}

	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// 3. Инициализируем глобальный пул соединений
	testPool, err = pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("failed to create pgxpool: %v", err)
	}

	// 4. ЗАПУСКАЕМ ВСЕ ТЕСТЫ ПАКЕТА
	code := m.Run()

	// 5. ТЕАРДАУН (Очистка ресурсов после прохождения всех тестов)
	testPool.Close()
	if err := pgContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %v", err)
	}

	// Выходим с кодом, который вернули тесты
	os.Exit(code)
}

func clearDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, "TRUNCATE TABLE chats RESTART IDENTITY CASCADE;")
	require.NoError(t, err, "failed to truncate tables for test isolation")
}

func TestStorage_CreateChat(t *testing.T) {
	clearDB(t)
	ctx := context.Background()

	storage := New(testPool)

	members := []int64{1, 2}
	chatID, err := storage.CreateChat(ctx, members)
	require.NoError(t, err)
	require.NotZero(t, chatID)

	var chatCount int
	err = testPool.QueryRow(ctx,
		"SELECT count(*) FROM chats WHERE id = $1",
		chatID).Scan(&chatCount)
	require.NoError(t, err)
	require.Equal(t, 1, chatCount,
		"Chat row should exist in chats table")

	// Проверка 2: Корректно ли отработала транзакция и сохранились ли связи в chat_members
	var membersCount int
	err = testPool.QueryRow(ctx,
		"SELECT count(*) FROM chat_members WHERE chat_id = $1",
		chatID).Scan(&membersCount)
	require.NoError(t, err)
	require.Equal(t, len(members),
		membersCount,
		"All members should be saved in chat_members table")
}

func TestStorage_SaveMessage(t *testing.T) {
	clearDB(t)

	ctx := context.Background()
	storage := New(testPool)

	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)

	tests := []struct {
		name     string
		chatID   int64
		senderID int64
		text     string
		expMsgID int64
		expError bool
	}{
		{
			name:     "success",
			chatID:   chatID,
			senderID: 1,
			text:     "test",
			expError: false,
		},
		/*{
			name:     "fail_invalid_chat_id",
			chatID:   9999, // invalid chat
			senderID: 1,
			text:     "Ghost message",
			expError: true,
		},*/ // TODO: Fix invalid chat id validation
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgID, err := storage.SaveMessage(ctx, chatID, tt.senderID, tt.text)

			if tt.expError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotZero(t, msgID)
			}
		})
	}
}

// TODO: all tests
func TestStorage_DeleteMessage(t *testing.T) {
	// setup clean db for tests
	clearDB(t)
	ctx := context.Background()
	storage := New(testPool)

	// test data
	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)
	msgID, err := storage.SaveMessage(ctx, chatID, 1, "message to delete")
	require.NoError(t, err)

	tests := []struct {
		name     string
		msgID    int64
		chatID   int64
		expError bool
	}{
		{
			name:     "success_delete",
			msgID:    msgID,
			chatID:   chatID,
			expError: false,
		},
		{
			name:     "fail_not_found",
			msgID:    9999, // invalid id
			chatID:   chatID,
			expError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.DeleteMessage(ctx, tt.msgID, tt.chatID)
			if tt.expError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStorage_GetHistory(t *testing.T) {
	clearDB(t)
	ctx := context.Background()
	storage := New(testPool)

	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)

	// Генерируем 5 сообщений
	for i := 1; i <= 5; i++ {
		_, err := storage.SaveMessage(ctx, chatID, 1, "message")
		require.NoError(t, err)
	}

	tests := []struct {
		name           string
		limit          int64
		offset         int64
		expectedLength int
	}{
		{name: "get_first_page", limit: 3, offset: 0, expectedLength: 3},
		{name: "get_second_page", limit: 3, offset: 3, expectedLength: 2},
		{name: "get_empty_page", limit: 3, offset: 10, expectedLength: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := storage.GetHistory(ctx, chatID, tt.limit, tt.offset)
			require.NoError(t, err)
			assert.Len(t, messages, tt.expectedLength)
		})
	}
}

func TestStorage_GetMessage(t *testing.T) {
	clearDB(t)
	ctx := context.Background()
	storage := New(testPool)

	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)

	msgText := "specific message"
	msgID, err := storage.SaveMessage(ctx, chatID, 1, msgText)
	require.NoError(t, err)

	// Проверяем успешное получение
	msg, err := storage.GetMessage(ctx, chatID, msgID)
	require.NoError(t, err)
	assert.Equal(t, msgText, msg.Text)
	assert.Equal(t, int64(1), msg.SenderID)

	// Проверяем ошибку при несуществующем сообщении
	_, err = storage.GetMessage(ctx, chatID, 9999)
	require.Error(t, err)
}

func TestStorage_IsChatMember(t *testing.T) {
	clearDB(t)
	ctx := context.Background()
	storage := New(testPool)

	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)

	tests := []struct {
		name     string
		chatID   int64
		userID   int64
		expected bool
	}{
		{name: "user_is_member", chatID: chatID, userID: 1, expected: true},
		{name: "user_is_also_member", chatID: chatID, userID: 2, expected: true},
		{name: "user_not_member", chatID: chatID, userID: 3, expected: false},
		{name: "chat_does_not_exist", chatID: 9999, userID: 1, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMember, err := storage.IsChatMember(ctx, tt.chatID, tt.userID)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, isMember)
		})
	}
}
