package mode

import (
	"os"
	"strconv"
	"time"
)

// InitFromEnv загружает инфраструктурные настройки из переменных окружения.
//
// Критичные значения (WEB_LAND_PORT, REAL_URL) имеют дефолты и никогда не вызовут fatal.
// fatal предназначен для будущих настроек без разумного дефолта.
// Некритичные порты (Oper, CRM, Demo, Pay) остаются пустыми — их отсутствие означает
// недоступность соответствующего сервиса.
//
// Пример: mode.InitFromEnv(logger.Fatalf)
func InitFromEnv(fatal func(format string, args ...any)) {
	// Домен — дефолт: localhost (для dev-окружения)
	realHost = envVal("REAL_URL", "localhost")

	// TTL модели пользователя (минуты) — дефолт 1 час
	if v := os.Getenv("GLOB_USER_MODEL_TTL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			fatal("mode.InitFromEnv: GLOB_USER_MODEL_TTL содержит некорректное значение: %q", v)
		} else {
			userModelTTl = time.Duration(n) * time.Minute
		}
	}

	// Контекст отмены sql операций (не везде для явно долгих операций в бд есть коэффициенты х2б х3..)
	if v := os.Getenv("GLOB_SQL_CTX_CANCEL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			fatal("mode.InitFromEnv: GLOB_SQL_CTX_CANCEL содержит некорректное значение: %q", v)
		} else {
			sqlTimeToCancel = time.Duration(n) * time.Second
		}
	}

	// Логирование — дефолты из var
	logLevel = envVal("LOG_LEVEL", logLevel)
	logPath = envVal("LOG_PATH", logPath)

	// URL хоста (для S3, action_handler и т.п.).
	// Если REAL_HOST_URL задан — используем его напрямую,
	// иначе RealHost остаётся как hostname из REAL_URL.
	if v := os.Getenv("REAL_HOST_URL"); v != "" {
		realHost = v
	}
}

// envVal возвращает значение переменной окружения key,
// или def если переменная не задана или пуста.
func envVal(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
