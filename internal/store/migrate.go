package store

import (
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/migrations"
)

// schema_migrations records every migration with its version, name, status
// and the timestamp it was applied.  The name column maps directly to the
// SQL filename (without version prefix and .up.sql suffix), making it easy
// to trace which DDL was run when.
const createSchemaMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT       NOT NULL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL DEFAULT '',
    dirty      BOOLEAN      NOT NULL DEFAULT false,
    applied_at TIMESTAMPTZ  NOT NULL DEFAULT now()
)`

// upgradeSchemaColumns adds name / applied_at to existing schema_migrations
// tables that were created with the old (version, dirty) format.
const upgradeSchemaColumns = `
ALTER TABLE schema_migrations
    ADD COLUMN IF NOT EXISTS name       VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ  NOT NULL DEFAULT now()`

var reMigrationFile = regexp.MustCompile(`^(\d+)_.+\.up\.sql$`)

type migration struct {
	version int64
	name    string // e.g. "create_users" (filename without number prefix and .up.sql)
	sql     string
}

// Migrate ensures schema_migrations exists (with name + applied_at columns),
// applies every pending *.up.sql migration in version order, and records each
// one with its file name and timestamp.
//
// All SQL files use CREATE TABLE IF NOT EXISTS so the runner is safe to call
// on any database state.
func Migrate(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// 1. Guarantee the tracking table with the full schema.
	if _, err := sqlDB.Exec(createSchemaMigrationsTable); err != nil {
		return fmt.Errorf("store.Migrate: create schema_migrations: %w", err)
	}

	// 2. Upgrade legacy tables that only had (version, dirty).
	if _, err := sqlDB.Exec(upgradeSchemaColumns); err != nil {
		return fmt.Errorf("store.Migrate: upgrade schema_migrations columns: %w", err)
	}

	// 3. Load all *.up.sql files from the embedded FS.
	migs, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("store.Migrate: load migration files: %w", err)
	}

	// 4. Find already-applied versions.
	applied, err := appliedVersions(sqlDB)
	if err != nil {
		return fmt.Errorf("store.Migrate: query applied versions: %w", err)
	}

	// 5. Back-fill missing names on rows that were recorded by the old runner.
	if err := backfillNames(sqlDB, migs, applied); err != nil {
		return fmt.Errorf("store.Migrate: backfill migration names: %w", err)
	}

	// 6. Apply pending migrations in version order.
	pending := 0
	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		if err := runMigration(sqlDB, m); err != nil {
			return err
		}
		pending++
	}

	if pending == 0 {
		maxVer := int64(0)
		for v := range applied {
			if v > maxVer {
				maxVer = v
			}
		}
		log.Debug().Int64("version", maxVer).Msg("database schema up to date")
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// loadMigrations reads all *.up.sql files from the embedded FS, parses their
// version numbers and labels, and returns them sorted ascending by version.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, err
	}

	var migs []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := reMigrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		ver, _ := strconv.ParseInt(m[1], 10, 64)
		content, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		// label: strip leading "000002_" and trailing ".up.sql"
		label := strings.TrimSuffix(
			strings.TrimPrefix(e.Name(), fmt.Sprintf("%s_", m[1])),
			".up.sql",
		)
		migs = append(migs, migration{version: ver, name: label, sql: string(content)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// appliedVersions returns the set of version numbers that succeeded (dirty=false).
func appliedVersions(db *sql.DB) (map[int64]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations WHERE dirty = false`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// backfillNames updates rows whose name column is empty (recorded by the old
// runner that only tracked version + dirty).
func backfillNames(db *sql.DB, migs []migration, applied map[int64]bool) error {
	for _, m := range migs {
		if !applied[m.version] {
			continue
		}
		if _, err := db.Exec(
			`UPDATE schema_migrations SET name = $1 WHERE version = $2 AND name = ''`,
			m.name, m.version,
		); err != nil {
			return fmt.Errorf("backfill name for version %d: %w", m.version, err)
		}
	}
	return nil
}

// runMigration executes one migration and records it in schema_migrations.
//
//  1. INSERT version + name with dirty=true  (crash-visible)
//  2. Execute the SQL
//  3. UPDATE dirty=false + applied_at=now()  (success)
func runMigration(db *sql.DB, m migration) error {
	if _, err := db.Exec(
		`INSERT INTO schema_migrations (version, name, dirty)
		 VALUES ($1, $2, true)
		 ON CONFLICT (version) DO UPDATE SET name = $2, dirty = true`,
		m.version, m.name,
	); err != nil {
		return fmt.Errorf("store.Migrate: mark v%d dirty: %w", m.version, err)
	}

	log.Info().
		Int64("version", m.version).
		Str("name", m.name).
		Msg("applying migration")

	if _, err := db.Exec(m.sql); err != nil {
		return fmt.Errorf("store.Migrate: execute v%d (%s): %w", m.version, m.name, err)
	}

	if _, err := db.Exec(
		`UPDATE schema_migrations
		 SET dirty = false, applied_at = now()
		 WHERE version = $1`,
		m.version,
	); err != nil {
		return fmt.Errorf("store.Migrate: mark v%d clean: %w", m.version, err)
	}

	log.Info().
		Int64("version", m.version).
		Str("name", m.name).
		Msg("migration applied")
	return nil
}
