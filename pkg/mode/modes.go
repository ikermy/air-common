package mode

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ikermy/air_common/pkg/com"
)

func SetTextMode(enabled bool) {
	textMode = enabled
}
func IsTextModeEnabled() bool {
	return textMode
}
func SetVoiceCall(enabled bool) {
	voiceCallMode = enabled
}
func IsVoiceCallModeEnabled() bool {
	return voiceCallMode
}
func SetTestMode(enabled bool) {
	testAnswerMode = enabled
}
func IsTestModeEnabled() bool {
	return testAnswerMode
}
func SetAudioMode(enabled bool) {
	audioMode = enabled
}
func IsAudioModeEnabled() bool {
	return audioMode
}
func SetSendErrMsgToUser(enabled bool) {
	sendErrMsgToUser = enabled
}
func IsSendErrMsgToUser() bool {
	return sendErrMsgToUser
}
func SetRealHost(host string) {
	realHost = host
}
func GetRealHost() string {
	return realHost
}
func SetUserModelTTL(ttl time.Duration) {
	userModelTTl = ttl
}
func SetEndDialogChannel(channel chan uint64) {
	endDialogCh = channel
}
func GetEndDialogChannel() chan uint64 {
	return endDialogCh
}
func GetCarpinteroChannel() chan com.CarpCh {
	return carpinteroCh
}
func GetInstantMsgCh() chan com.InstMsg {
	return instantCh
}
func SetOperatorResponseTimeout(seconds time.Duration) {
	operatorResponseTimeout = seconds
}
func GetOperatorResponseTimeout() time.Duration {
	return operatorResponseTimeout
}
func GetUserModeTTL() time.Duration {
	return userModelTTl
}
func GetSQLTimeToCancel() time.Duration {
	return sqlTimeToCancel
}
func SetLogLevel(level string) error {
	if _, ok := validLevels[level]; !ok {
		return fmt.Errorf("invalid log level: %s", level)
	}
	return nil
}
func GetLogLevel() string {
	return logLevel
}
func SetLogPath(logPath string) error {
	if logPath == "" {
		return fmt.Errorf("log path is empty")
	}

	absPath, err := filepath.Abs(logPath)
	if err != nil {
		return fmt.Errorf("invalid log path: %w", err)
	}

	dir := filepath.Dir(absPath)

	// Проверяем, существует ли директория
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Пытаемся создать директорию
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Проверяем возможность записи в файл
	f, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot write to log file: %w", err)
	}
	f.Close()

	return nil
}
