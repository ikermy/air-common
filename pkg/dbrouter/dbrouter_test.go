package dbrouter

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"
)

func testConfig() Config {
	return Config{
		Generation: 1,
		Slots: []SlotRange{
			{From: 0, To: 511, Shard: "shard-a"},
			{From: 512, To: 1023, Shard: "shard-b"},
		},
		Shards: map[string]ShardConfig{
			"shard-a": {DSNEnv: "DB_SHARD_A_DSN"},
			"shard-b": {DSNEnv: "DB_SHARD_B_DSN"},
		},
		Reference: "shard-a",
	}
}

func TestSlotForUserDeterministic(t *testing.T) {
	slot := SlotForUser(12345)
	if slot >= SlotCount {
		t.Fatalf("slot out of range: %d", slot)
	}
	if again := SlotForUser(12345); again != slot {
		t.Fatalf("slot is not deterministic: %d != %d", slot, again)
	}
	if other := SlotForUser(12346); other == slot {
		t.Logf("adjacent ids share a slot (%d) — collision is possible but not required", slot)
	}
}

func TestNewRejectsIncompleteSlotMap(t *testing.T) {
	cfg := testConfig()
	cfg.Slots = cfg.Slots[:1]
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for uncovered slots")
	}
}

func TestNewRejectsOverlap(t *testing.T) {
	cfg := testConfig()
	cfg.Slots = append(cfg.Slots, SlotRange{From: 0, To: 10, Shard: "shard-a"})
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for overlapping slots")
	}
}

func TestNewRejectsUnknownShard(t *testing.T) {
	cfg := testConfig()
	cfg.Slots[0].Shard = "missing"
	if _, err := New(cfg); err == nil {
		t.Fatal("expected error for unknown shard")
	}
}

func TestShardForUser(t *testing.T) {
	r, err := New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for userID := uint64(1); userID <= 100; userID++ {
		slot, want := r.SlotForUserDebug(userID)
		shard := r.ShardForUser(userID)
		if shard != want {
			t.Fatalf("user %d slot %d: shard=%q want %q", userID, slot, shard, want)
		}
		if shard != "shard-a" && shard != "shard-b" {
			t.Fatalf("unexpected shard: %q", shard)
		}
	}

	m := r.Metrics()
	if m.Requests != 100 {
		t.Fatalf("requests=%d want 100", m.Requests)
	}
	if m.ByShard["shard-a"]+m.ByShard["shard-b"] != 100 {
		t.Fatalf("per-shard requests do not sum to 100: %v", m.ByShard)
	}
}

func TestDBForUserPropagatesOpenError(t *testing.T) {
	openErr := errors.New("boom")
	r, err := NewForTest(testConfig(), func(string, string) (*sql.DB, error) {
		return nil, openErr
	})
	if err != nil {
		t.Fatalf("NewForTest: %v", err)
	}
	t.Setenv("DB_SHARD_A_DSN", "dsn")
	t.Setenv("DB_SHARD_B_DSN", "dsn")

	if _, err := r.DBForUser(context.Background(), 1); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
	if m := r.Metrics(); m.Errors == 0 {
		t.Fatal("error metric was not incremented")
	}
}

func TestDBForUserRequiresDSN(t *testing.T) {
	r, err := NewForTest(testConfig(), func(string, string) (*sql.DB, error) {
		t.Fatal("open must not be called without a DSN")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewForTest: %v", err)
	}
	t.Setenv("DB_SHARD_A_DSN", "")
	t.Setenv("DB_SHARD_B_DSN", "")

	if _, err := r.DBForUser(context.Background(), 1); err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestDBForReference(t *testing.T) {
	r, err := New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Setenv("DB_SHARD_A_DSN", "")
	if _, err := r.DBForReference(); err == nil {
		t.Fatal("expected error for empty reference DSN")
	}

	cfg := testConfig()
	cfg.Reference = ""
	r, err = New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.DBForReference(); err == nil {
		t.Fatal("expected error when reference is not configured")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("DB_SHARDS_CONFIG", `{"generation":1,"slots":[{"from":0,"to":1023,"shard":"shard-0"}],"shards":{"shard-0":{"dsn_env":"DB_SHARD0_DSN"}}}`)
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.Generation != 1 || len(cfg.Shards) != 1 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestShardIDsSorted(t *testing.T) {
	r, err := New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ids := r.ShardIDs()
	if len(ids) != 2 || ids[0] != "shard-a" || ids[1] != "shard-b" {
		t.Fatalf("unexpected shard ids: %v", ids)
	}
}

type fakeDriver struct {
	closed atomic.Int32
}

func (d *fakeDriver) Open(string) (driver.Conn, error) {
	return &fakeConn{driver: d}, nil
}

type fakeConn struct {
	driver *fakeDriver
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (c *fakeConn) Close() error {
	c.driver.closed.Add(1)
	return nil
}
func (c *fakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func TestUpdateConfigReloadsSlotMap(t *testing.T) {
	r, err := New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for userID := uint64(1); userID <= 50; userID++ {
		r.ShardForUser(userID)
	}
	before := r.Metrics()

	cfg := testConfig()
	cfg.Generation = 2
	cfg.Slots = []SlotRange{{From: 0, To: 1023, Shard: "shard-b"}}
	cfg.Shards = map[string]ShardConfig{"shard-b": cfg.Shards["shard-b"]}
	cfg.Reference = "shard-b"

	if err := r.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if r.Generation() != 2 {
		t.Fatalf("generation=%d want 2", r.Generation())
	}
	for userID := uint64(1); userID <= 50; userID++ {
		if shard := r.ShardForUser(userID); shard != "shard-b" {
			t.Fatalf("user %d routed to %q, want shard-b", userID, shard)
		}
	}
	after := r.Metrics()
	if after.Requests != before.Requests+50 {
		t.Fatalf("requests=%d want %d", after.Requests, before.Requests+50)
	}
	if after.ByShard["shard-b"] < before.ByShard["shard-b"] {
		t.Fatal("shard-b metrics were not preserved")
	}
	ids := r.ShardIDs()
	if len(ids) != 1 || ids[0] != "shard-b" {
		t.Fatalf("unexpected shard ids after reload: %v", ids)
	}
}

func TestUpdateConfigRejectsInvalidAndKeepsState(t *testing.T) {
	r, err := New(testConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	bad := testConfig()
	bad.Generation = 3
	bad.Slots = bad.Slots[:1]

	if err := r.UpdateConfig(bad); err == nil {
		t.Fatal("expected validation error")
	}
	if r.Generation() != 1 {
		t.Fatalf("generation changed after failed update: %d", r.Generation())
	}
}

func TestUpdateConfigClosesRemovedShard(t *testing.T) {
	fd := &fakeDriver{}
	name := "dbrouter-fake"
	sql.Register(name, fd)

	cfg := testConfig()
	cfg.Shards = map[string]ShardConfig{
		"shard-a": {DSNEnv: "DB_SHARD_A_DSN", Driver: name},
		"shard-b": {DSNEnv: "DB_SHARD_B_DSN", Driver: name},
	}
	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Setenv("DB_SHARD_A_DSN", "dsn-a")
	t.Setenv("DB_SHARD_B_DSN", "dsn-b")

	ctx := context.Background()
	if err := r.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if fd.closed.Load() != 0 {
		t.Fatal("connections must not be closed before reload")
	}

	next := cfg
	next.Generation = 2
	next.Slots = []SlotRange{{From: 0, To: 1023, Shard: "shard-b"}}
	next.Shards = map[string]ShardConfig{"shard-b": cfg.Shards["shard-b"]}
	next.Reference = "shard-b"
	if err := r.UpdateConfig(next); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if fd.closed.Load() == 0 {
		t.Fatal("removed shard connection was not closed")
	}
}

func TestShardForSlot(t *testing.T) {
	cfg := testConfig()
	if shard, ok := cfg.ShardForSlot(0); !ok || shard != "shard-a" {
		t.Fatalf("slot 0: shard=%q ok=%v", shard, ok)
	}
	if shard, ok := cfg.ShardForSlot(512); !ok || shard != "shard-b" {
		t.Fatalf("slot 512: shard=%q ok=%v", shard, ok)
	}
	if shard, ok := cfg.ShardForSlot(1023); !ok || shard != "shard-b" {
		t.Fatalf("slot 1023: shard=%q ok=%v", shard, ok)
	}
	if _, ok := cfg.ShardForSlot(1024); ok {
		t.Fatal("slot 1024 must be out of range")
	}
}
