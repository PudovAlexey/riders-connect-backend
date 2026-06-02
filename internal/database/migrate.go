package database

import (
	"database/sql"
	"embed"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(db *sql.DB) {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrations source: %v", err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("migrations driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", d, "postgres", driver)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate up: %v", err)
	}
	log.Println("migrations: ok")
}
