// Command migrate applies the embedded Air MariaDB migrations.
//
// Usage:
//
//	DB_HOST=127.0.0.1:3306 DB_NAME=air DB_USER=air DB_PASSWORD=... \
//	  go run ./cmd/migrate -status
//
//	DB_HOST=... DB_NAME=air DB_USER=air DB_PASSWORD=... \
//	  go run ./cmd/migrate -up -confirm
//
// -up requires -confirm (or MIGRATE_CONFIRM=yes). -status is read-only and is
// the default. DB_DSN overrides the individual DB_* variables.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/ikermy/air-common/v2/pkg/migrate"
)

func main() {
	var (
		status  = flag.Bool("status", false, "print current schema version (default)")
		up      = flag.Bool("up", false, "apply all pending migrations")
		confirm = flag.Bool("confirm", false, "required with -up: confirm the target database")
		timeout = flag.Duration("timeout", 30*time.Minute, "overall operation timeout")
	)
	flag.Parse()

	if *status && *up {
		fatal("-status and -up are mutually exclusive")
	}
	if !*up {
		*status = true
	}
	confirmed := *confirm || strings.EqualFold(os.Getenv("MIGRATE_CONFIRM"), "yes")

	dsn, target, err := buildDSN()
	if err != nil {
		fatal("%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fatal("open %s: %v", target, err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		fatal("connect %s: %v", target, err)
	}

	runner := migrate.New(db)
	version, dirty, err := runner.Version(ctx)
	if err != nil {
		fatal("read schema version: %v", err)
	}
	fmt.Printf("target: %s\nschema version: %d (dirty=%t)\n", target, version, dirty)

	if *status {
		return
	}
	if dirty {
		fatal("database has a dirty migration; resolve it before applying new ones")
	}
	if !confirmed {
		fatal("-up requires -confirm (or MIGRATE_CONFIRM=yes) for target %s", target)
	}

	applied, err := runner.Up(ctx)
	if err != nil {
		fatal("apply migrations: %v", err)
	}
	version, dirty, err = runner.Version(ctx)
	if err != nil {
		fatal("read schema version after up: %v", err)
	}
	fmt.Printf("applied: %d\nschema version: %d (dirty=%t)\n", applied, version, dirty)
}

func buildDSN() (dsn, target string, err error) {
	if raw := os.Getenv("DB_DSN"); raw != "" {
		return raw, "DB_DSN", nil
	}
	host := envOr("DB_HOST", "127.0.0.1:3306")
	name := envOr("DB_NAME", "air")
	user := envOr("DB_USER", "root")
	pass := os.Getenv("DB_PASSWORD")

	addr := host
	if _, _, splitErr := net.SplitHostPort(host); splitErr != nil {
		addr = net.JoinHostPort(host, "3306")
	}
	// parseTime is required for TIMESTAMP scan; the runner splits statements, so
	// multiStatements is not needed.
	dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC&timeout=10s",
		user, pass, addr, name)
	return dsn, fmt.Sprintf("%s/%s", addr, name), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "migrate: "+format+"\n", args...)
	os.Exit(1)
}
