// Command migrate applies or rolls back the database migrations.
//
//	go run ./cmd/migrate up        # apply all pending migrations
//	go run ./cmd/migrate down      # roll back the latest migration
//	go run ./cmd/migrate reset     # roll back everything (goose down to 0)
//	go run ./cmd/migrate version   # print the current schema version
//
// The server already runs "up" on start (DB_MIGRATE_ON_START=true), so this
// command is for development and operations. It reads DATABASE_URL.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/victorradael/condoguard/api/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: migrate up|down|reset|version")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := db.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch os.Args[1] {
	case "up":
		err = db.MigrateUp(ctx, pool)
	case "down":
		err = db.MigrateDown(ctx, pool)
	case "reset":
		err = db.MigrateDownTo(ctx, pool, 0)
	case "version":
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		return err
	}

	v, err := db.MigrationVersion(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Println("schema version:", v)
	return nil
}
