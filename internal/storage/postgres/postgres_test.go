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
	_, err := testPool.Exec(ctx, "TRUNCATE TABLE chats CASCADE;")
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

	tests := []struct {
		name     string
		senderID int64
		text     string
		expMsgID int64
		expError bool
		//targetError error
	}{
		{name: "success", senderID: 1,
			text: "test", expMsgID: 1, expError: false},
	}

	chatID, err := storage.CreateChat(ctx, []int64{1, 2})
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgID, err := storage.SaveMessage(ctx, chatID, tt.senderID, tt.text)

			if tt.expError {
				require.Error(t, err)
				//require.ErrorIs(t, err, tt.targetError)
			} else {
				require.NoError(t, err)
				require.NotNil(t, msgID)
				assert.Equal(t, tt.expMsgID, msgID)
			}
		})
	}
}
