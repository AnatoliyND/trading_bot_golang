package monitoring

import (
	"time"

	"trading-bot/data"
	"trading-bot/logger"
)

// Функция для мониторинга статуса серверов
func MonitorServerStatus(interval time.Duration) {
	for {
		// Проверка статуса сервера Финам
		finamAvailable, err := data.CheckFinamServerStatus()
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to check Finam server status")
		} else if !finamAvailable {
			logger.Logger.Warn().Msg("Finam server is unavailable")
		} else {
			logger.Logger.Debug().Msg("Finam server is available")
		}

		// Проверка статуса сервера Московской биржи
		_, err = data.GetInstrumentInfo("SBER") // Используем любой известный тикер
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Failed to check Moscow Exchange server status")
		} else {
			logger.Logger.Debug().Msg("Moscow Exchange server is available")
		}

		// Пауза перед следующей проверкой
		time.Sleep(interval)
	}
}
