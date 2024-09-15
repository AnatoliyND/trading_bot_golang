package monitoring

import (
	"time"

	"trading-bot/data"
	"trading-bot/logger"

	"go.uber.org/zap"
)

// Функция для мониторинга статуса серверов
func MonitorServerStatus(interval time.Duration) {
	for {
		// Проверка статуса сервера Финам
		finamAvailable, err := data.CheckFinamServerStatus()
		if err != nil {
			logger.Logger.Error("Failed to check Finam server status",
				zap.Error(err))
		} else if !finamAvailable {
			logger.Logger.Warn("Finam server is unavailable")
		} else {
			logger.Logger.Debug("Finam server is available")
		}

		// Проверка статуса сервера Московской биржи
		_, err = data.GetInstrumentInfo("SBER") // Используем любой известный тикер
		if err != nil {
			logger.Logger.Error("Failed to check Moscow Exchange server status",
				zap.Error(err))
		} else {
			logger.Logger.Debug("Moscow Exchange server is available")
		}

		// Пауза перед следующей проверкой
		time.Sleep(interval)
	}
}
