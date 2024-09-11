package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Инициализируем логгер
var Logger zerolog.Logger

func init() {
	// Создаем директорию для логов, если она не существует
	logDir := "logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.Mkdir(logDir, 0755)
	}

	// Формируем имя файла лога с датой
	logFileName := fmt.Sprintf("%s/trading-bot-%s.log", logDir, time.Now().Format("2006-01-02"))

	// Настройка lumberjack для ротации лог-файла по дням
	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFileName,
		MaxSize:    500,  // Максимальный размер файла (в мегабайтах)
		MaxBackups: 7,    // Максимальное количество резервных копий
		MaxAge:     30,   // Максимальный возраст файла (в днях)
		Compress:   true, // Сжимать резервные копии
	}

	// Настройка zerolog для использования lumberjack
	Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        lumberjackLogger,
		TimeFormat: time.RFC3339,
	}).With().Timestamp().Logger()
}
