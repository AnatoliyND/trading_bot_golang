package backtest

import (
	"fmt"
	"math"
	"time"

	"trading-bot/api"
	"trading-bot/data"
	"trading-bot/logger"
	"trading-bot/order"
	"trading-bot/strategy"

	"github.com/go-gota/gota/dataframe"
	"github.com/go-gota/gota/series"
)

// Структура для хранения результатов бэктеста
type BacktestResult struct {
	TotalTrades        int         // Общее количество сделок
	ProfitableTrades   int         // Количество прибыльных сделок
	UnprofitableTrades int         // Количество убыточных сделок
	TotalProfit        float64     // Общая прибыль (в рублях)
	AverageProfit      float64     // Средняя прибыль/убыток по сделкам
	MaxDrawdown        float64     // Максимальная просадка (в процентах)
	SharpeRatio        float64     // Коэффициент Шарпа
	StartDate          time.Time   // Дата начала бэктеста
	EndDate            time.Time   // Дата окончания бэктеста
	TradingLog         []TradeInfo // Лог сделок
}

// Структура для хранения информации о каждой сделке
type TradeInfo struct {
	Symbol     string    `json:"symbol"`
	OpenTime   time.Time `json:"openTime"`
	OpenPrice  float64   `json:"openPrice"`
	CloseTime  time.Time `json:"closeTime"`
	ClosePrice float64   `json:"closePrice"`
	Profit     float64   `json:"profit"`
	Side       string    `json:"side"`
}

// Функция для выполнения бэктеста
func RunBacktest(finamAPI *api.FinamAPI, symbol string, strategy strategy.Strategy, startDate, endDate time.Time, initialCapital float64) (*BacktestResult, error) {
	// 1. Загрузка исторических данных
	historicalData, err := finamAPI.LoadHistoricalData(symbol, startDate, endDate, "1d")
	if err != nil {
		return nil, fmt.Errorf("error loading historical data: %w", err)
	}

	// 2. Преобразование FinamData в dataframe.DataFrame
	var records [][]string
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
	historyDf := dataframe.LoadRecords(records,
		dataframe.WithTypes(map[string]series.Type{
			"0": series.Float,
			"1": series.Float,
			"2": series.Float,
			"3": series.Float,
			"4": series.Float,
			"5": series.Int,
		}),
	)

	logger.Logger.Info().
		Str("symbol", symbol).
		Int("rows", historyDf.Nrow()).
		Msg("Historical data loaded")

	// 3. Инициализация портфеля
	portfolio := &order.PortfolioInfo{
		Balances: map[string]order.Balance{
			"RUB": {Available: initialCapital}, // Используем initialCapital
		},
		Positions: make([]order.Position, 0),
	}

	// 4. Имитация торгов на исторических данных
	var (
		totalTrades        int
		profitableTrades   int
		unprofitableTrades int
		totalProfit        float64
		tradeLog           []TradeInfo
		maxEquity          float64 = initialCapital
		maxDrawdown        float64
	)

	// Определим период для стратегии, возможно, он передается как параметр функции стратегии
	strategyPeriod := 10 // Примерное значение, нужно задать или передать в функцию

	for i := strategyPeriod; i < historyDf.Nrow(); i++ {
		currentBar := historyDf.Subset([]int{i})
		// Преобразуем строку времени из DataFrame в time.Time
		timeString, err := currentBar.Col("5").Int()
		if err != nil || len(timeString) == 0 {
			continue
		}
		currentBarTime, _ := time.Parse("20060102", fmt.Sprintf("%d", timeString[0]))

		// Получаем сигналы от стратегии
		signals, err := strategy.GetSignals(&data.Quote{
			Price: currentBar.Col("1").Float()[0],
			Time:  currentBarTime,
		}, &historyDf, portfolio) // Передаем также ссылку на портфель
		if err != nil {
			logger.Logger.Warn().Err(err).Msg("Error getting signals, skipping bar")
			continue // Пропускаем бар, если есть ошибка в стратегии
		}

		logger.Logger.Debug().
			Int("bar", i).
			Time("time", currentBarTime).
			Interface("signals", signals).
			Msg("Bar processed")

		// Обработка сигналов
		for _, signal := range signals {
			// Проверка, достаточно ли средств для покупки
			if signal.Side == "buy" && portfolio.Balances["RUB"].Available >= signal.Price {
				// Расчет размера позиции (в лотах)
				positionSizeLots := int(math.Floor((portfolio.Balances["RUB"].Available * 0.01) / signal.Price)) // 0.01 как пример
				if positionSizeLots == 0 {
					logger.Logger.Warn().
						Str("symbol", signal.Symbol).
						Msg("Недостаточно средств для открытия позиции")
					continue // Недостаточно средств даже для 1 лота
				}

				// Отправка виртуального ордера на покупку
				totalTrades++
				newPosition := order.Position{
					Symbol:       signal.Symbol,
					Quantity:     positionSizeLots,
					AveragePrice: signal.Price,
					OpenDate:     currentBarTime,
				}
				portfolio.Positions = append(portfolio.Positions, newPosition)
				balance := portfolio.Balances["RUB"]
				balance.Available -= signal.Price * float64(positionSizeLots)
				portfolio.Balances["RUB"] = balance

				tradeLog = append(tradeLog, TradeInfo{
					Symbol:    signal.Symbol,
					Side:      "buy",
					OpenPrice: signal.Price,
					OpenTime:  currentBarTime,
				})
				logger.Logger.Debug().
					Str("symbol", signal.Symbol).
					Str("side", "buy").
					Float64("price", signal.Price).
					Msg("Virtual buy order executed")

			} else if signal.Side == "sell" {
				// Проверяем, есть ли у нас открытая позиция по этому инструменту
				for i, position := range portfolio.Positions {
					if position.Symbol == signal.Symbol && position.Quantity > 0 {
						// Отправка виртуального ордера на продажу и закрытие позиции
						totalTrades++
						profit := (signal.Price - position.AveragePrice) * float64(position.Quantity)
						balance := portfolio.Balances["RUB"]
						balance.Available += signal.Price * float64(position.Quantity)
						portfolio.Balances["RUB"] = balance
						portfolio.Positions = append(portfolio.Positions[:i], portfolio.Positions[i+1:]...) // Удаляем позицию из портфеля

						// Обновляем totalProfit и счетчики сделок
						totalProfit += profit
						if profit > 0 {
							profitableTrades++
						} else {
							unprofitableTrades++
						}

						tradeLog = append(tradeLog, TradeInfo{
							Symbol:     signal.Symbol,
							Side:       "sell",
							OpenPrice:  position.AveragePrice,
							OpenTime:   position.OpenDate,
							ClosePrice: signal.Price,
							CloseTime:  currentBarTime,
							Profit:     profit,
						})
						logger.Logger.Debug().
							Str("symbol", signal.Symbol).
							Str("side", "sell").
							Float64("price", signal.Price).
							Msg("Virtual sell order executed")

						break // Выходим из цикла, т.к. позиция закрыта
					}
				}
			}

			// Обновление максимальной доходности и максимальной просадки
			currentEquity := portfolio.Balances["RUB"].Available
			if currentEquity > maxEquity {
				maxEquity = currentEquity
			}
			drawdown := (maxEquity - currentEquity) / maxEquity * 100
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}

	// 5. Расчет дополнительных показателей
	averageProfit := totalProfit / float64(totalTrades)
	sharpeRatio := calculateSharpeRatio(tradeLog, 0.02) // 0.02 - безрисковая ставка (пример)

	//  6.  Возврат  результатов
	return &BacktestResult{
		TotalTrades:        totalTrades,
		ProfitableTrades:   profitableTrades,
		UnprofitableTrades: unprofitableTrades,
		TotalProfit:        totalProfit,
		AverageProfit:      averageProfit, //  Добавлен  показатель
		MaxDrawdown:        maxDrawdown,
		SharpeRatio:        sharpeRatio, //  Добавлен  показатель
		StartDate:          startDate,
		EndDate:            endDate,
		TradingLog:         tradeLog, //  Добавлен  лог  сделок
	}, nil
}

// Функция  для  расчета  коэффициента  Шарпа
func calculateSharpeRatio(trades []TradeInfo, riskFreeRate float64) float64 {
	if len(trades) == 0 {
		return 0 //  Или  обработайте  ошибку  -  нет  сделок  для  расчета
	}
	returns := make([]float64, len(trades))
	for i, trade := range trades {
		returns[i] = trade.Profit
	}
	stdDev := calculateStandardDeviation(returns)
	meanReturn := calculateMean(returns)

	return (meanReturn - riskFreeRate) / stdDev
}

// Функция  для  расчета  стандартного  отклонения
func calculateStandardDeviation(data []float64) float64 {
	if len(data) == 0 {
		return 0 //  Или  паника,  если  данных  нет
	}
	mean := calculateMean(data)
	sumSquaredDeviations := 0.0
	for _, x := range data {
		sumSquaredDeviations += math.Pow(x-mean, 2)
	}
	variance := sumSquaredDeviations / float64(len(data)-1)
	return math.Sqrt(variance)
}

// Функция  для  расчета  среднего  значения
func calculateMean(data []float64) float64 {
	if len(data) == 0 {
		return 0 // Или паника, если данных нет
	}
	sum := 0.0
	for _, x := range data {
		sum += x
	}
	return sum / float64(len(data))
}
