package main

import (
	"os"
	"trading-bot/api" // Импортируем пакет api, где должна быть функция FetchMarketData
	"trading-bot/logger"

	"go.uber.org/zap"
)

func main() {
	// Инициализация логгера
	logger.InitLogger()        // Убираем обработку возврата, так как InitLogger не возвращает значение
	defer logger.Logger.Sync() // Синхронизация логгера для корректного закрытия

	logger.Logger.Info("Запуск торгового робота")

	// Проверка переменных окружения
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		logger.Logger.Error("API ключ не найден", zap.String("error", "API_KEY отсутствует в переменных окружения"))
		return
	}

	// Асинхронные вызовы для получения данных с API
	urls := []string{
		"https://api.example.com/marketdata1",
		"https://api.example.com/marketdata2",
		// Добавьте больше URL для получения данных
	}

	api.FetchMarketData(urls) // Убедитесь, что функция FetchMarketData существует в пакете api

	logger.Logger.Info("Торговый робот завершил свою работу")
}
