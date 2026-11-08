// Command userdir maintains the global user_directory table.
//
// DB_* env vars point at the directory database.
//
// Examples:
//
//	go run ./cmd/userdir -action verify
//	go run ./cmd/userdir -action backfill
//	go run ./cmd/userdir -action recompute -shard-map-file shards.json
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/ikermy/air-common/pkg/comdb"
	"github.com/ikermy/air-common/pkg/dbrouter"
)

func main() {
	action := flag.String("action", "verify", "backfill|verify|recompute")
	shardMapFile := flag.String("shard-map-file", "", "slot map JSON (defaults to DB_SHARDS_CONFIG/_FILE)")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	db, err := comdb.New(ctx)
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer db.Conn().Close()

	switch *action {
	case "backfill":
		stats, err := db.BackfillUserDirectory(ctx, resolverFromEnv(*shardMapFile))
		if err != nil {
			fatalf("backfill: %v", err)
		}
		fmt.Printf("backfill: %+v\n", stats)

	case "verify":
		issues, err := db.VerifyUserDirectory(ctx, resolverFromEnv(*shardMapFile))
		if err != nil {
			fatalf("verify: %v", err)
		}
		fmt.Printf("issues: %d\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("user=%d: %s\n", issue.UserID, issue.Reason)
		}
		if len(issues) > 0 {
			os.Exit(1)
		}

	case "recompute":
		resolver, err := resolverFromFile(*shardMapFile)
		if err != nil {
			fatalf("recompute requires a slot map: %v", err)
		}
		updated, err := db.RecomputeUserDirectoryShards(ctx, resolver)
		if err != nil {
			fatalf("recompute: %v", err)
		}
		fmt.Printf("recompute: updated %d row(s)\n", updated)

	default:
		fatalf("unknown action %q (want backfill|verify|recompute)", *action)
	}
}

// resolverFromEnv loads a slot map when configured; otherwise returns nil
// (single-shard layout, shard 0).
func resolverFromEnv(file string) func(uint16) uint16 {
	resolver, err := resolverFromFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "userdir: no shard map (%v) — using shard 0\n", err)
		return nil
	}
	return resolver
}

func resolverFromFile(file string) (func(uint16) uint16, error) {
	var (
		cfg dbrouter.Config
		err error
	)
	if file != "" {
		cfg, err = dbrouter.LoadConfig(file)
	} else {
		cfg, err = dbrouter.LoadConfigFromEnv()
	}
	if err != nil {
		return nil, err
	}
	if _, err := dbrouter.New(cfg); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(cfg.Shards))
	for id := range cfg.Shards {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	index := make(map[string]uint16, len(ids))
	for i, id := range ids {
		index[id] = uint16(i)
	}

	return func(slot uint16) uint16 {
		shard, ok := cfg.ShardForSlot(slot)
		if !ok {
			return 0
		}
		return index[shard]
	}, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "userdir: "+format+"\n", args...)
	os.Exit(2)
}
