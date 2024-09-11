package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"trading-bot/api"
	"trading-bot/connector"
	"trading-bot/data"
	"trading-bot/logger"
	"trading-bot/monitoring"
	"trading-bot/order"
	"trading-bot/strategy"

	"github.com/go-gota/gota/dataframe"
)

func main() {

	tradingSymbol := "SBER"
	//	positionSizePercent := 0.1 //  10%  от  капитала

	// Загрузка конфигурации
	config, err := connector.LoadConfigFromFile("connector/transaq.json")
	if err != nil {
		logger.Logger.Warn().Err(err).Msg("Failed to load config from file. Trying to load from environment variables...")
		config, err = connector.LoadConfigFromEnv()
		if err != nil {
			logger.Logger.Fatal().Err(err).Msg("Failed to load config from environment variables.")
		}
	}

	// Загрузка конфигурации Finam API из файла connector/transaq.json
	finamConfig := &api.FinamConfig{}
	if err := loadFinamConfig("connector/transaq.json", finamConfig); err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to load Finam API config.")
	}

	// Создание объекта FinamAPI
	finamAPI, err := api.NewFinamAPI(finamConfig)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to create Finam API object.")
	}

	// Создание объекта TransaqConnector
	transaqConnector, err := connector.NewTransaqConnector(config)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to create Transaq Connector.")
	}

	// Подключение к Transaq Connector
	if err := transaqConnector.Connect(); err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to connect to Transaq Connector.")
	}
	defer transaqConnector.Close()

	// Авторизация
	if err := transaqConnector.Authorize(); err != nil {
		logger.Logger.Fatal().Err(err).Msg("Failed to authorize on Transaq Connector.")
	}

	// Запуск чтения сообщений
	transaqConnector.StartReading()

	// Запуск heartbeat
	transaqConnector.StartHeartbeat(30 * time.Second)

	// Запускаем мониторинг в отдельной горутине
	go monitoring.MonitorServerStatus(1 * time.Minute)

	// --- Trading Robot Logic ---

	// 1. Получение списка инструментов
	instruments, err := finamAPI.GetInstruments()
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("Ошибка при получении списка инструментов")
	}

	// 2. Вывод списка инструментов в лог (для проверки)
	for _, instrument := range instruments {
		logger.Logger.Info().
			Int("ID", instrument.ID).
			Str("Symbol", instrument.Symbol).
			Str("Name", instrument.Name).
			Msg("Instrument")
	}

	//  Выбор  стратегии
	chosenStrategy := "simple_trend" //  Измените  эту  переменную,  чтобы  выбрать  другую  стратегию

	//  Создание  объекта  стратегии
	var chosenStrategyName strategy.Strategy
	var simpleTrendStrategy *strategy.SimpleTrendStrategy //  Добавлена  переменная
	switch chosenStrategy {
	case "simple_trend":
		simpleTrendStrategy = &strategy.SimpleTrendStrategy{Period: 14} //  Присваиваем  конкретную  стратегию  интерфейсу
		chosenStrategyName = simpleTrendStrategy
	// case "другая_стратегия":
	//     strategy = &strategy.ДругаяСтратегия{ /* ...  параметры  ...  */ }
	default:
		logger.Logger.Fatal().Msg("Неизвестная  стратегия")
	}

	// Получение списка инструментов
	err = transaqConnector.SendMessage("<command id=\"get_securities\"/>")
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to send get_securities command")
	}

	// Цикл работы робота
	for {
		// 1. Получаем сообщение от Transaq Connector
		message, err := transaqConnector.GetMessageWithTimeout(10 * time.Second)
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Error getting message from connector.")
			// Попытка переподключения...
			continue
		}

		if strings.HasPrefix(message, "<event>") {
			transaqConnector.HandleEvent(message)
		} else {
			// Обработка ответа на команду
			// ...
			if strings.Contains(message, "securities") {
				type Security struct {
					Secid     string `xml:"secid"`
					Board     string `xml:"board"`
					Shortname string `xml:"shortname"`
				}

				var securities struct {
					Securities []Security `xml:"security"`
				}

				err = xml.Unmarshal([]byte(message), &securities)
				if err != nil {
					logger.Logger.Error().Err(err).Msg("Failed to parse securities response")
				} else {
					for _, s := range securities.Securities {
						logger.Logger.Info().
							Str("secid", s.Secid).
							Str("board", s.Board).
							Str("shortname", s.Shortname).
							Msg("Security")
					}
				}
			}
		}

		// 2. Получение текущей  котировки  для  выбранного  инструмента
		quote, err := data.GetCurrentQuotes(tradingSymbol)
		if err != nil {
			logger.Logger.Error().Err(err).Msg("Error getting current quotes.")
			continue //  Пропускаем  текущую  итерацию  и  переходим  к  следующей
		}

		// 3.  Получение  исторических  данных  для  выбранного  инструмента
		if simpleTrendStrategy != nil { //  Проверка  на nil
			startDate := time.Now().AddDate(0, 0, -simpleTrendStrategy.Period)
			endDate := time.Now()
			historicalData, err := finamAPI.LoadHistoricalData(tradingSymbol, startDate, endDate, "1d")
			if err != nil {
				logger.Logger.Error().Err(err).Msg("Error loading historical data.")
				continue
			}

			//  4.  Преобразование  FinamData  в  dataframe.DataFrame  для  стратегии
			// Создаём пустой слайс для хранения данных в формате [][]string
			var records [][]string

			// Преобразуем каждую структуру из historicalData.C в слайс строк
			for _, candle := range historicalData.C {
				record := []string{
					fmt.Sprintf("%f", candle.O),
					fmt.Sprintf("%f", candle.C),
					fmt.Sprintf("%f", candle.H),
					fmt.Sprintf("%f", candle.L),
					fmt.Sprintf("%f", candle.V),
					fmt.Sprintf("%d", candle.T),
				}
				records = append(records, record)
			}

			// Создаём DataFrame из преобразованных данных
			historyDf := dataframe.LoadRecords(records)

			// 5. Получение информации о  портфеле
			portfolio, err := order.GetPortfolioInfo(finamConfig.AccessToken)
			if err != nil {
				logger.Logger.Error().Err(err).Msg("Error getting portfolio info.")
				continue
			}

			// 6. Генерация торговых сигналов на основе стратегии
			signals, err := chosenStrategyName.GetSignals(quote, &historyDf, portfolio)
			if err != nil {
				logger.Logger.Error().Err(err).Msg("Error getting trading signals.")
				continue
			}

			// 7. Логика принятия  решений  и  отправки  ордеров
			for _, signal := range signals {
				// 7.1. Реализация  логики  принятия  решений

				//  Проверка  наличия  достаточных  средств
				if signal.Side == "buy" && portfolio.Balances["RUB"].Available < signal.Price {
					logger.Logger.Warn().
						Str("symbol", signal.Symbol).
						Msg("Недостаточно средств для покупки")
					continue //  Пропускаем  ордер
				}

				//  Проверка,  что  у  нас  нет  уже  открытой  позиции  по  этому  инструменту
				hasOpenPosition := false
				for _, position := range portfolio.Positions {
					if position.Symbol == signal.Symbol && position.Quantity > 0 {
						hasOpenPosition = true
						break
					}
				}
				if hasOpenPosition {
					logger.Logger.Warn().
						Str("symbol", signal.Symbol).
						Msg("Позиция по инструменту уже открыта")
					continue // Пропускаем ордер
				}

				// 7.2. Создание  и  отправка  ордера
				if signal.Side == "buy" {
					//  Создание  ордера  на  покупку
					orderRequest := &order.OrderRequest{
						Symbol:      signal.Symbol,
						Side:        signal.Side,
						Quantity:    1,        //  Пример:  покупаем  1  лот
						OrderType:   "market", //  Пример:  рыночный  ордер
						AccessToken: finamConfig.AccessToken,
						// ... (добавьте  другие  поля,  если  необходимо)
					}
					_, err := order.CreateOrder(orderRequest)
					if err != nil {
						logger.Logger.Error().Err(err).Msg("Error creating buy order.")
					}
				} else if signal.Side == "sell" {
					//  Создание  ордера  на  продажу
					orderRequest := &order.OrderRequest{
						Symbol:      signal.Symbol,
						Side:        signal.Side,
						Quantity:    1,        //  Пример:  продаём  1  лот
						OrderType:   "market", //  Пример:  рыночный  ордер
						AccessToken: finamConfig.AccessToken,
						// ...  (добавьте  другие  поля,  если  необходимо)
					}
					_, err := order.CreateOrder(orderRequest)
					if err != nil {
						logger.Logger.Error().Err(err).Msg("Error  creating  sell  order.")
					}
				}
			}
		}
	}
}

// Функция для загрузки конфигурации Finam API из файла connector/transaq.json
func loadFinamConfig(filename string, config *api.FinamConfig) error {
	data, err := os.ReadFile(filename) // Используем os.ReadFile
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var transaqConfig map[string]interface{}
	if err := json.Unmarshal(data, &transaqConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	config.AccessToken = transaqConfig["access_token"].(string)

	return nil
}
