package logger

import (
	"go.uber.org/zap"
)

// Инициализируем логгер
var Logger *zap.Logger

func InitLogger() {
	// Конфигурируем zap логгер
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{
		"stdout",
		"./logs/trading_bot.log",
	}

	var err error
	Logger, err = config.Build()
	if err != nil {
		panic("Не удалось инициализировать логгер: " + err.Error())
	}
}

func SyncLogger() {
	_ = Logger.Sync()
}
