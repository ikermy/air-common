// Command backfill runs the Go-side data migrations that SQL cannot express:
// responders.user_id, user_bots/user_bot_events/user_bot_state and
// user_directory. All operations are idempotent.
//
// Usage:
//
//	DB_HOST=host:3306 DB_NAME=air DB_USER=... DB_PASSWORD=... \
//	  go run ./cmd/backfill -action all
//
// Actions: all (default), responders, userbots, userdir, verify.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/ikermy/air-common/v2/pkg/comdb"
)

func main() {
	action := flag.String("action", "all", "all|responders|userbots|userdir|verify")
	timeout := flag.Duration("timeout", 2*time.Hour, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	db, err := comdb.New(ctx)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer db.Close()

	failed := false
	run := func(name string, fn func() error) {
		if err := fn(); err != nil {
			fmt.Printf("%s: ERROR: %v\n", name, err)
			failed = true
			return
		}
	}

	switch *action {
	case "all":
		run("userdir", func() error {
			stats, err := db.BackfillUserDirectory(ctx, nil)
			fmt.Printf("userdir: %+v\n", stats)
			return err
		})
		run("responders", func() error {
			stats, err := db.BackfillResponders(ctx)
			fmt.Printf("responders: %+v\n", stats)
			return err
		})
		run("userbots", func() error {
			stats, err := db.BackfillUserBots(ctx)
			fmt.Printf("userbots: %+v\n", stats)
			return err
		})
	case "userdir":
		run("userdir", func() error {
			stats, err := db.BackfillUserDirectory(ctx, nil)
			fmt.Printf("userdir: %+v\n", stats)
			return err
		})
	case "responders":
		run("responders", func() error {
			stats, err := db.BackfillResponders(ctx)
			fmt.Printf("responders: %+v\n", stats)
			return err
		})
	case "userbots":
		run("userbots", func() error {
			stats, err := db.BackfillUserBots(ctx)
			fmt.Printf("userbots: %+v\n", stats)
			return err
		})
	case "verify":
		issues, err := db.VerifyUserDirectory(ctx, nil)
		if err != nil {
			fatal("verify userdir: %v", err)
		}
		fmt.Printf("userdir issues: %d\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("user=%d: %s\n", issue.UserID, issue.Reason)
		}
		if len(issues) > 0 {
			failed = true
		}
	default:
		fatal("unknown action %q (want all|responders|userbots|userdir|verify)", *action)
	}

	if failed {
		os.Exit(1)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "backfill: "+format+"\n", args...)
	os.Exit(1)
}
