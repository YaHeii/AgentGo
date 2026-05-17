package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version string
	upSQL   string
	downSQL string
}

func MigrateUp(ctx context.Context, dbPath string, steps int) error {
	dbConn, err := openSQLite(dbPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = dbConn.Close()
	}()

	return migrateUp(ctx, dbConn, steps)
}

func MigrateDown(ctx context.Context, dbPath string, steps int) error {
	dbConn, err := openSQLite(dbPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = dbConn.Close()
	}()

	return migrateDown(ctx, dbConn, steps)
}

func migrateUp(ctx context.Context, dbConn *sql.DB, steps int) error {
	if steps < 0 {
		return fmt.Errorf("migrate up: invalid steps %d", steps)
	}
	if err := enableForeignKeys(ctx, dbConn); err != nil {
		return err
	}
	if err := ensureMigrationTable(ctx, dbConn); err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	applied, err := loadAppliedVersions(ctx, dbConn)
	if err != nil {
		return err
	}

	remaining := steps
	for _, m := range migrations {
		if applied[m.version] {
			continue
		}
		if m.upSQL == "" {
			return fmt.Errorf("migrate up: missing up migration for %s", m.version)
		}
		if err := applyUpMigration(ctx, dbConn, m); err != nil {
			return err
		}
		if steps > 0 {
			remaining--
			if remaining == 0 {
				break
			}
		}
	}

	return nil
}

func migrateDown(ctx context.Context, dbConn *sql.DB, steps int) error {
	if steps < 0 {
		return fmt.Errorf("migrate down: invalid steps %d", steps)
	}
	if err := enableForeignKeys(ctx, dbConn); err != nil {
		return err
	}
	if err := ensureMigrationTable(ctx, dbConn); err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	applied, err := loadAppliedVersions(ctx, dbConn)
	if err != nil {
		return err
	}

	remaining := steps
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if !applied[m.version] {
			continue
		}
		if m.downSQL == "" {
			return fmt.Errorf("migrate down: missing down migration for %s", m.version)
		}
		if err := applyDownMigration(ctx, dbConn, m); err != nil {
			return err
		}
		if steps > 0 {
			remaining--
			if remaining == 0 {
				break
			}
		}
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	byVersion := make(map[string]migration, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		version, direction, ok := parseMigrationName(name)
		if !ok {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}

		current := byVersion[version]
		current.version = version
		switch direction {
		case "up":
			current.upSQL = string(sqlBytes)
		case "down":
			current.downSQL = string(sqlBytes)
		}
		byVersion[version] = current
	}

	versions := make([]string, 0, len(byVersion))
	for version := range byVersion {
		versions = append(versions, version)
	}
	sort.Strings(versions)

	migrations := make([]migration, 0, len(versions))
	for _, version := range versions {
		migrations = append(migrations, byVersion[version])
	}
	return migrations, nil
}

func parseMigrationName(name string) (version string, direction string, ok bool) {
	switch {
	case strings.HasSuffix(name, ".up.sql"):
		return strings.TrimSuffix(name, ".up.sql"), "up", true
	case strings.HasSuffix(name, ".down.sql"):
		return strings.TrimSuffix(name, ".down.sql"), "down", true
	default:
		return "", "", false
	}
}

func ensureMigrationTable(ctx context.Context, dbConn *sql.DB) error {
	_, err := dbConn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at INTEGER NOT NULL
);`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func loadAppliedVersions(ctx context.Context, dbConn *sql.DB) (map[string]bool, error) {
	rows, err := dbConn.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func applyUpMigration(ctx context.Context, dbConn *sql.DB, m migration) error {
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin up migration %s: %w", m.version, err)
	}

	if _, err := tx.ExecContext(ctx, m.upSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply up migration %s: %w", m.version, err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
		m.version,
		time.Now().UTC().UnixMilli(),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record up migration %s: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit up migration %s: %w", m.version, err)
	}
	return nil
}

func applyDownMigration(ctx context.Context, dbConn *sql.DB, m migration) error {
	tx, err := dbConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin down migration %s: %w", m.version, err)
	}

	if _, err := tx.ExecContext(ctx, m.downSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply down migration %s: %w", m.version, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, m.version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete down migration record %s: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit down migration %s: %w", m.version, err)
	}
	return nil
}
