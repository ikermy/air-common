package comdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ikermy/air-common/pkg/idgen"
)

// Тайм-аут на операции с БД фактически для тяжолых операций добавляются коэфициентч x2, x3...
const sqlTimeToCancel = 5

// DB представляет соединение с базой данных
type DB struct {
	dsn                 string
	MasterKeyResolver   MasterKeyResolver
	IDGen               func() uint64
	DialogV2ReadEnabled bool
	conn                *sql.DB
	mainCTX             context.Context
	ctx                 context.Context
	cancel              context.CancelFunc
	shardResolver       ShardResolver
	referenceConn       *sql.DB
}

// ShardResolver resolves the database for a user (implemented by dbrouter.Router).
type ShardResolver interface {
	DBForUser(ctx context.Context, userID uint64) (*sql.DB, error)
}

// SetShardResolver enables per-user shard routing for methods that use
// ConnForUser. Without a resolver everything stays on the primary connection.
func (d *DB) SetShardResolver(r ShardResolver) {
	d.shardResolver = r
}

// ConnForUser returns the shard connection for userID, or the primary
// connection when routing is not configured.
func (d *DB) ConnForUser(ctx context.Context, userID uint32) (*sql.DB, error) {
	if d.shardResolver == nil || userID == 0 {
		return d.Conn(), nil
	}
	return d.shardResolver.DBForUser(ctx, uint64(userID))
}

// SetReferenceConn sets the connection used for reference-data tables
// (gpt_models, realtime_models, voice_models, ...). Without it the primary
// connection is used.
func (d *DB) SetReferenceConn(conn *sql.DB) {
	d.referenceConn = conn
}

// RefConn returns the reference-data connection or the primary one.
func (d *DB) RefConn() *sql.DB {
	if d.referenceConn != nil {
		return d.referenceConn
	}
	return d.Conn()
}

// connForUserCtx is a convenience wrapper for methods that previously called
// d.Conn() directly and need the standard SQL timeout.
func (d *DB) connForUserCtx(userID uint32) (*sql.DB, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(d.Context(), sqlTimeToCancel*time.Second)
	conn, err := d.ConnForUser(ctx, userID)
	if err != nil {
		cancel()
		return nil, nil, func() {}, err
	}
	return conn, ctx, cancel, nil
}

var (
	defaultGenOnce sync.Once
	defaultGen     *idgen.Generator
)

func defaultIDGen() uint64 {
	defaultGenOnce.Do(func() {
		gen, err := idgen.NewFromEnv()
		if err != nil {
			gen = idgen.MustNew(0)
		}
		defaultGen = gen
	})
	return defaultGen.Next()
}

// SetIDGen overrides the identifier generator used for new rows.
func (d *DB) SetIDGen(gen func() uint64) {
	d.IDGen = gen
}

func (d *DB) nextID() uint64 {
	if d.IDGen != nil {
		return d.IDGen()
	}
	return defaultIDGen()
}

// MasterKeyResolver returns the user's decrypted MasterKey from cache or remote.
type MasterKeyResolver func(userId uint32) ([32]byte, bool)

// Метод инъекции:
func (d *DB) SetMasterKeyResolver(r MasterKeyResolver) {
	d.MasterKeyResolver = r
}

// New создает новое подключение к базе данных
func New(parent context.Context) (*DB, error) {
	host := os.Getenv("DB_HOST")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		user, pass, host, name)

	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Пул соединений
	conn.SetMaxOpenConns(100)
	conn.SetMaxIdleConns(100)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	return &DB{
		mainCTX:             parent,
		ctx:                 ctx,
		cancel:              cancel,
		dsn:                 dsn,
		conn:                conn,
		DialogV2ReadEnabled: strings.EqualFold(os.Getenv("DIALOG_MESSAGES_READ_ENABLED"), "true"),
	}, nil
}

// Close закрывает соединения с базой данных и отменяет контекст
func (d *DB) Close() error {
	// Отменяем контекст базы данных
	if d.cancel != nil {
		d.cancel()
	}

	// Закрываем соединение с базой данных
	if d.conn != nil {
		if err := d.conn.Close(); err != nil {
			return err
		}
	}

	return nil
}

// Conn возвращает базовое подключение к БД для расширенного использования в приложениях
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// Context возвращает контекст БД для использования в пользовательских методах
func (d *DB) Context() context.Context {
	return d.ctx
}

// MainCTX возвращает главный контекст приложения
func (d *DB) MainCTX() context.Context {
	return d.mainCTX
}
