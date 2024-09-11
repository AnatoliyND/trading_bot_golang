package strategy

import (
	"trading-bot/data"
	"trading-bot/order"

	"github.com/go-gota/gota/dataframe"
)

// Интерфейс для торговых стратегий
type Strategy interface {
	// GetSignals принимает данные и возвращает сигналы на покупку или продажу
	GetSignals(quotes *data.Quote, history *dataframe.DataFrame, portfolio *order.PortfolioInfo) ([]TradingSignal, error)
}

// Структура для торгового сигнала
type TradingSignal struct {
	Symbol string  `json:"symbol"` //  Тикер  инструмента
	Side   string  `json:"side"`   //  Направление:  "buy"  или  "sell"
	Price  float64 `json:"price"`  //  Цена,  по  которой  нужно  открыть  позицию
}
