package store

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	"database/sql"

	migrations "github.com/JonMukaj/go-producer-consumer/migrations"
)

// Migrate runs all pending up migrations from the embedded SQL files.
func Migrate(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("migration db open: %w", err)
	}
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migration init: %w", err)
	}

	if err := m.Migrate(1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up: %w", err)
	}

	return nil
}
