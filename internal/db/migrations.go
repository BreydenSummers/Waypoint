package db

import (
    "context"
    "database/sql"
    "embed"
    "fmt"
    "io/fs"
    "sort"
    "strings"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

func ApplyMigrations(ctx context.Context, db *sql.DB) error {
    entries, err := fs.ReadDir(migrationFS, "migrations")
    if err != nil {
        return err
    }

    names := make([]string, 0, len(entries))
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
            continue
        }
        names = append(names, entry.Name())
    }
    sort.Strings(names)

    for _, name := range names {
        body, err := migrationFS.ReadFile("migrations/" + name)
        if err != nil {
            return err
        }

        tx, err := db.BeginTx(ctx, nil)
        if err != nil {
            return err
        }
        if _, err := tx.ExecContext(ctx, string(body)); err != nil {
            _ = tx.Rollback()
            return fmt.Errorf("apply %s: %w", name, err)
        }
        if err := tx.Commit(); err != nil {
            _ = tx.Rollback()
            return fmt.Errorf("commit %s: %w", name, err)
        }
    }
    return nil
}
