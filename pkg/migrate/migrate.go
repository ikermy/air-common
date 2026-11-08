// Package migrate contains versioned SQL migrations and a small runner for
// the Air MariaDB schema. It uses the database/sql driver supplied by the
// caller and stores applied versions in schema_migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const (
	lockName = "air_common_schema_migration"
	tableDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT NOT NULL,
		name VARCHAR(255) NOT NULL,
		checksum CHAR(64) NOT NULL,
		dirty TINYINT(1) NOT NULL DEFAULT 0,
		applied_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
)

var migrationNameRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

// Migration is one versioned schema change.
type Migration struct {
	Version  int64
	Name     string
	Up       string
	Down     string
	Checksum string
}

// Runner applies embedded migrations to a MariaDB database.
type Runner struct {
	db *sql.DB
}

func New(db *sql.DB) *Runner {
	return &Runner{db: db}
}

// Ensure creates the schema_migrations table if it does not exist.
func (r *Runner) Ensure(ctx context.Context) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("migrate: nil database")
	}
	_, err := r.db.ExecContext(ctx, tableDDL)
	return err
}

// Version returns the highest applied version and whether a dirty migration exists.
func (r *Runner) Version(ctx context.Context) (version int64, dirty bool, err error) {
	if err := r.Ensure(ctx); err != nil {
		return 0, false, err
	}
	var v sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations WHERE dirty = 0`).Scan(&v); err != nil {
		return 0, false, err
	}
	if v.Valid {
		version = v.Int64
	}
	var dirtyCount int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE dirty = 1`).Scan(&dirtyCount); err != nil {
		return 0, false, err
	}
	return version, dirtyCount > 0, nil
}

// Required returns an error when the database schema is older than version.
func (r *Runner) Required(ctx context.Context, version int64) error {
	current, dirty, err := r.Version(ctx)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("migrate: database has a dirty migration")
	}
	if current < version {
		return fmt.Errorf("migrate: schema version %d is older than required %d", current, version)
	}
	return nil
}

// Up applies all pending migrations and returns the number applied.
func (r *Runner) Up(ctx context.Context) (int, error) {
	migrations, err := Load()
	if err != nil {
		return 0, err
	}
	if err := r.Ensure(ctx); err != nil {
		return 0, err
	}

	conn, err := r.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	if err := acquireLock(ctx, conn); err != nil {
		return 0, err
	}
	defer releaseLock(conn)
	defer func() { _, _ = conn.ExecContext(context.Background(), `SET FOREIGN_KEY_CHECKS=1`) }()
	if _, err := conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=0`); err != nil {
		return 0, fmt.Errorf("migrate: disable foreign key checks: %w", err)
	}

	var dirty int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE dirty = 1`).Scan(&dirty); err != nil {
		return 0, err
	}
	if dirty > 0 {
		return 0, fmt.Errorf("migrate: database has a dirty migration; restore or repair schema_migrations")
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return 0, err
	}

	appliedCount := 0
	for _, m := range migrations {
		if checksum, ok := applied[m.Version]; ok {
			if checksum != m.Checksum {
				return appliedCount, fmt.Errorf("migrate: checksum mismatch for version %d (%s)", m.Version, m.Name)
			}
			continue
		}
		if err := applyOne(ctx, conn, m); err != nil {
			return appliedCount, err
		}
		appliedCount++
	}
	return appliedCount, nil
}

// Down reverts up to steps applied migrations, newest first.
func (r *Runner) Down(ctx context.Context, steps int) (int, error) {
	if steps <= 0 {
		return 0, nil
	}
	migrations, err := Load()
	if err != nil {
		return 0, err
	}
	byVersion := make(map[int64]Migration, len(migrations))
	for _, m := range migrations {
		byVersion[m.Version] = m
	}

	conn, err := r.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	if err := acquireLock(ctx, conn); err != nil {
		return 0, err
	}
	defer releaseLock(conn)
	defer func() { _, _ = conn.ExecContext(context.Background(), `SET FOREIGN_KEY_CHECKS=1`) }()
	if _, err := conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=0`); err != nil {
		return 0, fmt.Errorf("migrate: disable foreign key checks: %w", err)
	}

	applied, err := appliedVersionsDesc(ctx, conn, steps)
	if err != nil {
		return 0, err
	}
	reverted := 0
	for _, version := range applied {
		m, ok := byVersion[version]
		if !ok {
			return reverted, fmt.Errorf("migrate: applied version %d has no embedded migration", version)
		}
		statements, err := splitStatements(m.Down)
		if err != nil {
			return reverted, fmt.Errorf("migrate: parse down %d: %w", version, err)
		}
		for _, stmt := range statements {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return reverted, fmt.Errorf("migrate: down %d (%s): %w", version, m.Name, err)
			}
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, version); err != nil {
			return reverted, err
		}
		reverted++
	}
	return reverted, nil
}

func applyOne(ctx context.Context, conn *sql.Conn, m Migration) error {
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum, dirty) VALUES (?, ?, ?, 1)`,
		m.Version, m.Name, m.Checksum); err != nil {
		return fmt.Errorf("migrate: record start %d: %w", m.Version, err)
	}
	statements, err := splitStatements(m.Up)
	if err != nil {
		return fmt.Errorf("migrate: parse up %d: %w", m.Version, err)
	}
	for _, stmt := range statements {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: up %d (%s): %w", m.Version, m.Name, err)
		}
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE schema_migrations SET dirty = 0, applied_at = CURRENT_TIMESTAMP(3) WHERE version = ?`,
		m.Version); err != nil {
		return fmt.Errorf("migrate: mark applied %d: %w", m.Version, err)
	}
	return nil
}

func acquireLock(ctx context.Context, conn *sql.Conn) error {
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, lockName).Scan(&got); err != nil {
		return fmt.Errorf("migrate: acquire lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("migrate: could not acquire migration lock")
	}
	return nil
}

func releaseLock(conn *sql.Conn) {
	_, _ = conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK(?)`, lockName)
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[int64]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations WHERE dirty = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]string)
	for rows.Next() {
		var v int64
		var checksum string
		if err := rows.Scan(&v, &checksum); err != nil {
			return nil, err
		}
		result[v] = checksum
	}
	return result, rows.Err()
}

func appliedVersionsDesc(ctx context.Context, conn *sql.Conn, limit int) ([]int64, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT version FROM schema_migrations WHERE dirty = 0 ORDER BY version DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// Load reads and validates all embedded migrations.
func Load() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	type pair struct {
		name string
		up   string
		down string
	}
	pairs := make(map[int64]*pair)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		m := migrationNameRE.FindStringSubmatch(entry.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrate: invalid version in %q: %w", entry.Name(), err)
		}
		body, err := fs.ReadFile(migrationFS, filepath.ToSlash(filepath.Join("migrations", entry.Name())))
		if err != nil {
			return nil, err
		}
		p := pairs[version]
		if p == nil {
			p = &pair{name: m[2]}
			pairs[version] = p
		}
		switch m[3] {
		case "up":
			p.up = string(body)
		case "down":
			p.down = string(body)
		}
	}

	versions := make([]int64, 0, len(pairs))
	for version := range pairs {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })

	result := make([]Migration, 0, len(versions))
	for _, version := range versions {
		p := pairs[version]
		if strings.TrimSpace(p.up) == "" {
			return nil, fmt.Errorf("migrate: migration %d has no up file", version)
		}
		sum := sha256.Sum256([]byte(p.up))
		result = append(result, Migration{
			Version:  version,
			Name:     p.name,
			Up:       p.up,
			Down:     p.down,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	return result, nil
}
