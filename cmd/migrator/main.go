package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	// ВАЖНО: Заменили драйвер с sqlite3 на postgres
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var dbURL, migrationsPath, action string

	// Заменили storage-path на db-url (строка подключения DSN)
	flag.StringVar(&dbURL, "db-url", "", "postgres connection string (DSN)")
	flag.StringVar(&migrationsPath, "migrations-path", "", "path to migrations directory")
	flag.StringVar(&action, "action", "up", "action to perform: up or down")
	flag.Parse()

	if dbURL == "" || migrationsPath == "" {
		panic("db-url and migrations-path are required")
	}

	// Инициализируем мигратор. dbURL уже содержит правильный префикс postgres://
	m, err := migrate.New(
		"file://"+migrationsPath,
		dbURL,
	)
	if err != nil {
		panic(fmt.Errorf("failed to init migrator: %w", err))
	}

	// Выбираем действие в зависимости от флага
	switch action {
	case "up":
		if err := m.Up(); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				fmt.Println("no new migrations to apply")
				return
			}
			panic(fmt.Errorf("failed to apply migrations: %w", err))
		}
		fmt.Println("migrations applied successfully")

	case "down":
		// Откатываем все миграции вниз
		if err := m.Down(); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				fmt.Println("no migrations to rollback")
				return
			}
			panic(fmt.Errorf("failed to rollback migrations: %w", err))
		}
		fmt.Println("migrations rolled back successfully")

	default:
		panic(fmt.Sprintf("unknown action: %s", action))
	}
}
