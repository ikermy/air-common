// Command dbverify compares user-scoped rows between a source database and a
// target shard. Intended for slot cutover checks.
//
// Examples:
//
//	DB_SOURCE_DSN=... DB_TARGET_DSN=... go run ./cmd/dbverify -users 1,2,3
//	DB_SOURCE_DSN=... DB_TARGET_DSN=... DB_DIRECTORY_DSN=... go run ./cmd/dbverify -slot 42
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/ikermy/air-common/pkg/dbverify"
)

func main() {
	sourceEnv := flag.String("source-dsn-env", "DB_SOURCE_DSN", "env var with the source DSN")
	targetEnv := flag.String("target-dsn-env", "DB_TARGET_DSN", "env var with the target DSN")
	directoryEnv := flag.String("directory-dsn-env", "DB_DIRECTORY_DSN", "env var with the user_directory DSN (for -slot)")
	usersFlag := flag.String("users", "", "comma-separated user ids")
	usersFile := flag.String("users-file", "", "file with one user id per line")
	slot := flag.Int("slot", -1, "load user ids for this slot from user_directory")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	sourceDsn := strings.TrimSpace(os.Getenv(*sourceEnv))
	targetDsn := strings.TrimSpace(os.Getenv(*targetEnv))
	if sourceDsn == "" || targetDsn == "" {
		fatalf("both %s and %s must be set", *sourceEnv, *targetEnv)
	}

	users, err := collectUsers(ctx, *usersFlag, *usersFile, *slot, *directoryEnv)
	if err != nil {
		fatalf("%v", err)
	}
	if len(users) == 0 {
		fatalf("no users to compare")
	}

	source, err := sql.Open("mysql", sourceDsn)
	if err != nil {
		fatalf("open source: %v", err)
	}
	defer source.Close()
	target, err := sql.Open("mysql", targetDsn)
	if err != nil {
		fatalf("open target: %v", err)
	}
	defer target.Close()

	if err := source.PingContext(ctx); err != nil {
		fatalf("ping source: %v", err)
	}
	if err := target.PingContext(ctx); err != nil {
		fatalf("ping target: %v", err)
	}

	diffs, err := dbverify.CompareUsers(ctx, source, target, users, nil)
	if err != nil {
		fatalf("%v", err)
	}

	fmt.Printf("compared %d user(s), diffs: %d\n", len(users), len(diffs))
	for _, d := range diffs {
		fmt.Printf("user=%d table=%s source={count=%d sum=%d} target={count=%d sum=%d} reason=%s\n",
			d.UserID, d.Table, d.Source.Count, d.Source.Sum, d.Target.Count, d.Target.Sum, d.Reason)
	}
	if len(diffs) > 0 {
		os.Exit(1)
	}
}

func collectUsers(ctx context.Context, list, file string, slot int, directoryEnv string) ([]uint64, error) {
	seen := make(map[uint64]bool)
	var users []uint64

	add := func(id uint64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		users = append(users, id)
	}

	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user id %q: %w", part, err)
		}
		add(id)
	}

	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			id, err := strconv.ParseUint(line, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid user id %q in %s: %w", line, file, err)
			}
			add(id)
		}
	}

	if slot >= 0 {
		dsn := strings.TrimSpace(os.Getenv(directoryEnv))
		if dsn == "" {
			return nil, fmt.Errorf("%s must be set for -slot", directoryEnv)
		}
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("open directory: %w", err)
		}
		defer db.Close()

		rows, err := db.QueryContext(ctx, "SELECT user_id FROM user_directory WHERE slot = ? ORDER BY user_id", slot)
		if err != nil {
			return nil, fmt.Errorf("load slot %d: %w", slot, err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uint64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			add(id)
		}
		return users, rows.Err()
	}

	return users, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "dbverify: "+format+"\n", args...)
	os.Exit(2)
}
