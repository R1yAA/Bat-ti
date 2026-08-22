// Command migrate applies and rolls back the SQL files in database/migrations.
//
// The golang-migrate CLI only registers database drivers that were selected by
// build tag, which makes it awkward to run through `go tool`. Driving the same
// library from a few lines here imports the driver explicitly instead, so
// `go run ./cmd/migrate up` behaves identically on a laptop and in CI with no
// build-tag ceremony.
package main

import (
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

const migrationsSourceURL = "file://database/migrations"

func main() {
	stepCount := flag.Int("steps", 1, "number of migrations to roll back when the command is \"down\"")
	flag.Parse()

	command := flag.Arg(0)
	if command == "" {
		command = "up"
	}

	// A missing .env is fine: the environment may already carry DATABASE_URL,
	// which is how CI and production supply it.
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is not set; copy .env.example to .env or export it")
		os.Exit(1)
	}

	migrator, err := migrate.New(migrationsSourceURL, toPgxURL(databaseURL))
	if err != nil {
		slog.Error("could not open migrations", "error", err)
		os.Exit(1)
	}
	defer migrator.Close()

	switch command {
	case "up":
		err = migrator.Up()
	case "down":
		err = migrator.Steps(-*stepCount)
	case "version":
		version, isDirty, versionErr := migrator.Version()
		if versionErr != nil {
			slog.Error("could not read schema version", "error", versionErr)
			os.Exit(1)
		}
		slog.Info("schema version", "version", version, "dirty", isDirty)
		return
	default:
		slog.Error("unknown command; expected up, down or version", "command", command)
		os.Exit(1)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("schema already up to date")
		return
	}
	if err != nil {
		slog.Error("migration failed", "command", command, "error", err)
		os.Exit(1)
	}
	slog.Info("migration complete", "command", command)
}

// toPgxURL rewrites a postgres:// URL to the pgx/v5 scheme golang-migrate
// registers its driver under, so one DATABASE_URL works for both the
// application and the migrator.
func toPgxURL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if len(databaseURL) > len(prefix) && databaseURL[:len(prefix)] == prefix {
			return "pgx5://" + databaseURL[len(prefix):]
		}
	}
	return databaseURL
}
