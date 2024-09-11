package strategy

import (
	"fmt"
	"trading-bot/data"
	"trading-bot/order"

	"github.com/go-gota/gota/dataframe"
)

type RiskManagement struct {
	StopLossPercent   float64
	TakeProfitPercent float64
	// ...  другие  параметры  управления  рисками
}

// Простая  трендовая  стратегия
type SimpleTrendStrategy struct {
	Period            int
	StopLossPercent   float64 //  Процент  для  Stop-Loss
	TakeProfitPercent float64 //  Процент  для  Take-Profit
	RiskManagement    *RiskManagement
}

type SimpleTradingSignal struct {
	Symbol     string
	Side       string
	Price      float64
	StopLoss   float64 //  Цена  Stop-Loss
	TakeProfit float64 //  Цена  Take-Profit
}

// GetSignals  реализует  интерфейс  Strategy
func (s *SimpleTrendStrategy) GetSignals(quotes *data.Quote, history *dataframe.DataFrame, portfolio *order.PortfolioInfo) ([]TradingSignal, error) {
	// 1. Проверка,  достаточно  ли  данных  в  истории
	if history.Nrow() < s.Period {
		return nil, fmt.Errorf("недостаточно данных для расчета скользящей средней: требуется %d, доступно %d", s.Period, history.Nrow())
	}

	// 2. Расчет  скользящей  средней
	sma := calculateSMA(history, s.Period)

	// 3. Формирование  сигналов
	var signals []TradingSignal
	if quotes.Price > sma {
		signals = append(signals, TradingSignal{
			Symbol: quotes.Symbol,
			Side:   "buy",
			Price:  quotes.Price, //  Покупаем  по  текущей  рыночной  цене
		})
	} else if quotes.Price < sma {
		signals = append(signals, TradingSignal{
			Symbol: quotes.Symbol,
			Side:   "sell",
			Price:  quotes.Price, // Продаем по текущей рыночной цене
		})
	}

	return signals, nil
}

// Функция для расчета простой скользящей средней (SMA)
func calculateSMA(df *dataframe.DataFrame, period int) float64 {
	closePrices := df.Col("Close").Float() //  Предполагается,  что  в  DataFrame  есть  столбец  "Close"  с  ценами  закрытия
	sum := 0.0
	for i := len(closePrices) - period; i < len(closePrices); i++ {
		sum += closePrices[i]
	}
	return sum / float64(period)
}
