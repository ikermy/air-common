package comdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/ikermy/air-common/pkg/comdom"
)

// Тайм-аут на операции с БД фактически для тяжолых операций добавляются коэфициентч x2, x3...
const sqlTimeToCancel = 5

// ChatType определяет тип чата (используется в БД)
type ChatType uint8

const (
	TelegramBot ChatType = 0
	Web         ChatType = 1
	Telegram    ChatType = 2
	Avito       ChatType = 3
	Widget      ChatType = 4
	WhatsApp    ChatType = 5
	Instagram   ChatType = 6
)

type CreatorType uint8

const (
	AI                 CreatorType = 1 // Право
	User               CreatorType = 2 // Лево
	UserVoice          CreatorType = 3 // Лево
	Operator           CreatorType = 4 // Прав
	SpeechRealTimeAI   CreatorType = 5 // Прав
	SpeechRealTimeUser CreatorType = 6 // Лево
)

// Используем типы из пакета model/create для совместимости с интерфейсом comdom.DB
type (
	Ids             = comdom.Ids
	VecIds          = comdom.VecIds
	UserModelRecord = comdom.UserModelRecord
	ProviderType    = comdom.ProviderType
)

// DB представляет соединение с базой данных
type DB struct {
	dsn               string
	MasterKeyResolver MasterKeyResolver
	conn              *sql.DB
	mainCTX           context.Context
	ctx               context.Context
	cancel            context.CancelFunc
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
		mainCTX: parent,
		ctx:     ctx,
		cancel:  cancel,
		dsn:     dsn,
		conn:    conn,
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
