// Package dbrouter maps users to shards by slot.
//
// The slot is derived from the user id with xxhash64: slot = hash(userID) & 1023.
// A config generation maps slot ranges to shard ids and shard ids to DSN env
// variables, so the slot map can be swapped without changing call sites.
package dbrouter

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/cespare/xxhash/v2"
)

// SlotCount is the number of routing slots.
const SlotCount = 1024

// ShardForSlot returns the shard id mapped to an inclusive slot.
func (c Config) ShardForSlot(slot uint16) (string, bool) {
	if int(slot) >= SlotCount {
		return "", false
	}
	for _, sr := range c.Slots {
		if slot >= sr.From && slot <= sr.To {
			return sr.Shard, true
		}
	}
	return "", false
}

// ShardConfig describes one shard.
type ShardConfig struct {
	// DSNEnv is the environment variable that holds the DSN.
	DSNEnv string `json:"dsn_env"`
	// Driver is the database/sql driver name (default "mysql").
	Driver string `json:"driver,omitempty"`
}

// SlotRange maps an inclusive slot range to a shard id.
type SlotRange struct {
	From  uint16 `json:"from"`
	To    uint16 `json:"to"`
	Shard string `json:"shard"`
}

// Config is the slot map generation.
type Config struct {
	Generation int                    `json:"generation"`
	Slots      []SlotRange            `json:"slots"`
	Shards     map[string]ShardConfig `json:"shards"`
	// Reference is the shard that holds reference data (optional).
	Reference string `json:"reference,omitempty"`
}

// Metrics is a point-in-time snapshot of router counters.
type Metrics struct {
	Generation int
	Requests   uint64
	Errors     uint64
	ByShard    map[string]uint64
	// ErrorsByShard counts failed shard resolutions and connection errors.
	ErrorsByShard map[string]uint64
}

type shardMetrics struct {
	requests atomic.Uint64
	errors   atomic.Uint64
}

// Router resolves user and reference shards and caches connections.
type Router struct {
	stateMu sync.RWMutex
	cfg     Config
	bySlot  [SlotCount]string
	drivers map[string]string
	metrics map[string]*shardMetrics

	open func(driver, dsn string) (*sql.DB, error)

	mu   sync.Mutex
	dbs  map[string]*sql.DB
	dsns map[string]string

	totalRequests atomic.Uint64
	totalErrors   atomic.Uint64
}

// New validates cfg and creates a router. Connections are opened lazily.
func New(cfg Config) (*Router, error) {
	return NewForTest(cfg, sql.Open)
}

// NewForTest creates a router with a custom connection opener.
func NewForTest(cfg Config, open func(driver, dsn string) (*sql.DB, error)) (*Router, error) {
	r := &Router{
		open: open,
		dbs:  make(map[string]*sql.DB),
		dsns: make(map[string]string),
	}
	if err := r.applyConfig(cfg, nil); err != nil {
		return nil, err
	}
	return r, nil
}

// applyConfig validates cfg and swaps the routing state.
//
// previousMetrics (optional) is used to preserve counters across reloads.
func (r *Router) applyConfig(cfg Config, previousMetrics map[string]*shardMetrics) error {
	bySlot, drivers, metrics, err := validate(cfg)
	if err != nil {
		return err
	}
	if previousMetrics != nil {
		for id, m := range metrics {
			if old, ok := previousMetrics[id]; ok {
				m.requests.Store(old.requests.Load())
				m.errors.Store(old.errors.Load())
			}
		}
	}

	r.stateMu.Lock()
	r.cfg = cfg
	r.bySlot = bySlot
	r.drivers = drivers
	r.metrics = metrics
	r.stateMu.Unlock()
	return nil
}

func validate(cfg Config) ([SlotCount]string, map[string]string, map[string]*shardMetrics, error) {
	var bySlot [SlotCount]string
	if cfg.Generation <= 0 {
		return bySlot, nil, nil, fmt.Errorf("dbrouter: generation must be positive")
	}
	if len(cfg.Shards) == 0 {
		return bySlot, nil, nil, fmt.Errorf("dbrouter: no shards configured")
	}
	for id, shard := range cfg.Shards {
		if shard.DSNEnv == "" {
			return bySlot, nil, nil, fmt.Errorf("dbrouter: shard %q has no dsn_env", id)
		}
	}
	if cfg.Reference != "" {
		if _, ok := cfg.Shards[cfg.Reference]; !ok {
			return bySlot, nil, nil, fmt.Errorf("dbrouter: reference shard %q is not configured", cfg.Reference)
		}
	}

	drivers := make(map[string]string, len(cfg.Shards))
	metrics := make(map[string]*shardMetrics, len(cfg.Shards))
	for id, shard := range cfg.Shards {
		driver := shard.Driver
		if driver == "" {
			driver = "mysql"
		}
		drivers[id] = driver
		metrics[id] = &shardMetrics{}
	}

	var covered [SlotCount]bool
	for _, sr := range cfg.Slots {
		if sr.From > sr.To || int(sr.To) >= SlotCount {
			return bySlot, nil, nil, fmt.Errorf("dbrouter: invalid slot range %d..%d", sr.From, sr.To)
		}
		if _, ok := cfg.Shards[sr.Shard]; !ok {
			return bySlot, nil, nil, fmt.Errorf("dbrouter: slot range %d..%d references unknown shard %q", sr.From, sr.To, sr.Shard)
		}
		for s := sr.From; s <= sr.To; s++ {
			if covered[s] {
				return bySlot, nil, nil, fmt.Errorf("dbrouter: slot %d is covered more than once", s)
			}
			covered[s] = true
			bySlot[s] = sr.Shard
		}
	}
	for s := 0; s < SlotCount; s++ {
		if !covered[s] {
			return bySlot, nil, nil, fmt.Errorf("dbrouter: slot %d is not mapped", s)
		}
	}

	return bySlot, drivers, metrics, nil
}

// UpdateConfig atomically swaps the slot map. Connections for shards that are
// still present with the same DSN env are kept; other connections are closed.
func (r *Router) UpdateConfig(cfg Config) error {
	r.stateMu.RLock()
	previous := r.metrics
	oldShards := r.cfg.Shards
	r.stateMu.RUnlock()

	if err := r.applyConfig(cfg, previous); err != nil {
		return err
	}

	var toClose []string
	for id := range oldShards {
		newShard, ok := cfg.Shards[id]
		if !ok {
			toClose = append(toClose, id)
			continue
		}
		if newShard.DSNEnv != oldShards[id].DSNEnv {
			toClose = append(toClose, id)
		}
	}

	r.mu.Lock()
	for _, id := range toClose {
		if db, ok := r.dbs[id]; ok {
			_ = db.Close()
			delete(r.dbs, id)
		}
		delete(r.dsns, id)
	}
	r.mu.Unlock()
	return nil
}

// SlotForUser returns the slot for a user id.
func SlotForUser(userID uint64) uint16 {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], userID)
	return uint16(xxhash.Sum64(b[:]) & (SlotCount - 1))
}

// ShardForUser returns the shard id for a user id.
func (r *Router) ShardForUser(userID uint64) string {
	slot := SlotForUser(userID)

	r.stateMu.RLock()
	shard := r.bySlot[slot]
	metrics := r.metrics[shard]
	r.stateMu.RUnlock()

	if metrics != nil {
		metrics.requests.Add(1)
	}
	r.totalRequests.Add(1)
	return shard
}

// ForUser is an alias for ShardForUser.
func (r *Router) ForUser(userID uint64) string { return r.ShardForUser(userID) }

// DBForUser returns the cached connection for the user's shard.
func (r *Router) DBForUser(ctx context.Context, userID uint64) (*sql.DB, error) {
	shard := r.ShardForUser(userID)

	r.stateMu.RLock()
	metrics := r.metrics[shard]
	r.stateMu.RUnlock()

	db, err := r.dbForShard(shard)
	if err != nil {
		if metrics != nil {
			metrics.errors.Add(1)
		}
		r.totalErrors.Add(1)
		return nil, err
	}
	return db, nil
}

// DBForReference returns the cached reference-data connection.
func (r *Router) DBForReference() (*sql.DB, error) {
	r.stateMu.RLock()
	reference := r.cfg.Reference
	r.stateMu.RUnlock()
	if reference == "" {
		return nil, errors.New("dbrouter: reference shard is not configured")
	}
	return r.dbForShard(reference)
}

func (r *Router) dbForShard(shard string) (*sql.DB, error) {
	r.stateMu.RLock()
	shardCfg, ok := r.cfg.Shards[shard]
	driver := r.drivers[shard]
	r.stateMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dbrouter: unknown shard %q", shard)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if db, ok := r.dbs[shard]; ok {
		return db, nil
	}

	dsn := os.Getenv(shardCfg.DSNEnv)
	if dsn == "" {
		return nil, fmt.Errorf("dbrouter: env %s is empty", shardCfg.DSNEnv)
	}
	db, err := r.open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("dbrouter: open shard %q: %w", shard, err)
	}
	r.dbs[shard] = db
	r.dsns[shard] = dsn
	return db, nil
}

// Ping checks every configured shard. Errors of all shards are joined.
func (r *Router) Ping(ctx context.Context) error {
	r.stateMu.RLock()
	ids := make([]string, 0, len(r.cfg.Shards))
	for id := range r.cfg.Shards {
		ids = append(ids, id)
	}
	r.stateMu.RUnlock()
	sort.Strings(ids)

	var errs []error
	for _, shard := range ids {
		db, err := r.dbForShard(shard)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := db.PingContext(ctx); err != nil {
			errs = append(errs, fmt.Errorf("dbrouter: ping shard %q: %w", shard, err))
		}
	}
	return errors.Join(errs...)
}

// Close closes all cached connections.
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for shard, db := range r.dbs {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("dbrouter: close shard %q: %w", shard, err))
		}
		delete(r.dbs, shard)
		delete(r.dsns, shard)
	}
	return errors.Join(errs...)
}

// Metrics returns a snapshot of router counters.
func (r *Router) Metrics() Metrics {
	r.stateMu.RLock()
	m := Metrics{
		Generation:    r.cfg.Generation,
		Requests:      r.totalRequests.Load(),
		Errors:        r.totalErrors.Load(),
		ByShard:       make(map[string]uint64, len(r.metrics)),
		ErrorsByShard: make(map[string]uint64, len(r.metrics)),
	}
	for shard, s := range r.metrics {
		m.ByShard[shard] = s.requests.Load()
		m.ErrorsByShard[shard] = s.errors.Load()
	}
	r.stateMu.RUnlock()
	return m
}

// Generation returns the slot map generation.
func (r *Router) Generation() int {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()
	return r.cfg.Generation
}

// ShardIDs returns configured shard ids sorted for deterministic output.
func (r *Router) ShardIDs() []string {
	r.stateMu.RLock()
	ids := make([]string, 0, len(r.cfg.Shards))
	for id := range r.cfg.Shards {
		ids = append(ids, id)
	}
	r.stateMu.RUnlock()
	sort.Strings(ids)
	return ids
}

// LoadConfig reads a slot map from a JSON file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("dbrouter: parse %s: %w", path, err)
	}
	return cfg, nil
}

// LoadConfigFromEnv reads the slot map from DB_SHARDS_CONFIG (JSON) or
// from the file referenced by DB_SHARDS_CONFIG_FILE.
func LoadConfigFromEnv() (Config, error) {
	if raw := os.Getenv("DB_SHARDS_CONFIG"); raw != "" {
		var cfg Config
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return Config{}, fmt.Errorf("dbrouter: parse DB_SHARDS_CONFIG: %w", err)
		}
		return cfg, nil
	}
	path := os.Getenv("DB_SHARDS_CONFIG_FILE")
	if path == "" {
		return Config{}, errors.New("dbrouter: neither DB_SHARDS_CONFIG nor DB_SHARDS_CONFIG_FILE is set")
	}
	return LoadConfig(path)
}

// SlotForUserDebug returns the slot and shard for diagnostics.
func (r *Router) SlotForUserDebug(userID uint64) (uint16, string) {
	slot := SlotForUser(userID)
	r.stateMu.RLock()
	shard := r.bySlot[slot]
	r.stateMu.RUnlock()
	return slot, shard
}

// ParseUserID parses a decimal user id, useful for configs and tests.
func ParseUserID(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}
