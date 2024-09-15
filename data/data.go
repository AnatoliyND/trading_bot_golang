package data

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"trading-bot/logger" // Импортируем пакет logger

	"github.com/go-gota/gota/dataframe"
	"github.com/go-gota/gota/series" // Для FloatType
	"go.uber.org/zap"
	"gonum.org/v1/gonum/floats" // Для Min и Max
)

// Структура для хранения данных о текущих котировках
type Quote struct {
	Symbol         string    `json:"symbol" xml:"secid"` // Тикер инструмента
	TradingSession string    `xml:"tradingsession"`      // ID торговой сессии
	Board          string    `xml:"board"`               // Код площадки
	Price          float64   `json:"price" xml:"value"`  // Цена
	Volume         int64     `json:"volume" xml:"vol"`   // Объем
	Time           time.Time `json:"time" xml:"time"`    // Время
}

// Структура для хранения истории котировок
type QuoteHistory struct {
	*ring.Ring
}

// Структура для представления данных, полученных от API Финам (Trade API)
type FinamData struct {
	C []struct {
		O float64 `json:"o"` // Цена открытия
		C float64 `json:"c"` // Цена закрытия
		H float64 `json:"h"` // Максимальная цена
		L float64 `json:"l"` // Минимальная цена
		V float64 `json:"v"` // Объем
		T int64   `json:"t"` // Время (unix timestamp)
	} `json:"c"`
	S string `json:"s"` // Статус ответа
}

// Хранилище для текущих котировок (с разделением по торговым сессиям)
var CurrentQuotes = make(map[string]map[string]Quote)

// Хранилище для истории котировок
var QuotesHistory = make(map[string]*QuoteHistory)

// Мьютекс для синхронизации доступа к CurrentQuotes и QuotesHistory
var quotesMutex sync.RWMutex

// Функция для обновления котировок
func UpdateQuotes(newQuote *Quote) error {
	symbol := newQuote.Symbol
	tradingSession := newQuote.TradingSession

	// --- Проверка валидности данных ---

	if newQuote.Price <= 0 {
		logger.Logger.Warn("Invalid quote price, ignoring",
			zap.String("symbol", symbol),
			zap.Float64("price", newQuote.Price))
		return nil // Не фатальная ошибка, можно продолжить
	}

	if newQuote.Volume <= 0 {
		logger.Logger.Warn("Invalid quote volume, ignoring",
			zap.String("symbol", symbol),
			zap.Int64("volume", newQuote.Volume))
		return nil // Не фатальная ошибка
	}

	// ... (добавьте другие проверки, например, на валидность
	//       торговой сессии, кода площадки и т.д.)

	// --- Блокировка мьютекса для записи ---
	quotesMutex.Lock()
	defer quotesMutex.Unlock()

	// --- Обработка торговых сессий ---

	// Если нет котировок для этой торговой сессии, создаем новую map
	if _, ok := CurrentQuotes[tradingSession]; !ok {
		CurrentQuotes[tradingSession] = make(map[string]Quote)
	}

	// --- Обновление CurrentQuotes ---
	CurrentQuotes[tradingSession][symbol] = *newQuote

	// --- Сохранение истории котировок ---

	// Максимальное количество котировок в истории (настраивается)
	const maxHistorySize = 100

	// Получаем ring buffer для текущего символа
	history, ok := QuotesHistory[symbol]
	if !ok {
		// Ring buffer еще не создан, создаем новый
		history = &QuoteHistory{
			Ring: ring.New(maxHistorySize),
		}
		QuotesHistory[symbol] = history
	}

	// Добавляем новую котировку в ring buffer
	history.Value = *newQuote
	history.Ring = history.Next()

	logger.Logger.Info("Quote updated",
		zap.String("symbol", symbol),
		zap.String("tradingSession", tradingSession),
		zap.Float64("price", newQuote.Price),
		zap.Time("time", newQuote.Time))

	return nil
}

// Пример функции для получения истории котировок
func GetQuoteHistory(symbol string) ([]Quote, error) {
	quotesMutex.RLock() // Блокировка для чтения
	defer quotesMutex.RUnlock()

	history, ok := QuotesHistory[symbol]
	if !ok {
		return nil, fmt.Errorf("no history found for symbol: %s", symbol)
	}

	// Преобразуем ring buffer в срез
	quotes := make([]Quote, 0, history.Len())
	history.Do(func(p interface{}) {
		if quote, ok := p.(Quote); ok {
			quotes = append(quotes, quote)
		}
	})

	return quotes, nil
}

// Функция для нормализации DataFrame
func normalizeDataframe(df *dataframe.DataFrame) *dataframe.DataFrame {
	for _, colName := range df.Names() {
		if df.Col(colName).Type() != series.Float {
			continue // Пропускаем нечисловые столбцы
		}
		colData := df.Col(colName).Float()
		min := floats.Min(colData)
		max := floats.Max(colData)
		for i := 0; i < len(colData); i++ {
			colData[i] = (colData[i] - min) / (max - min)
		}

		// Создаём новый объект series.Series
		newSeries := series.New(colData, series.Float, colName)

		// Используем newSeries в Mutate
		*df = df.Mutate(newSeries)
	}
	return df
}

// --- Новые функции ---

// Структура для хранения данных о текущих котировках
type FinamQuote struct {
	Symbol string    `json:"symbol"`
	Price  float64   `json:"price"`
	Volume int64     `json:"volume"`
	Time   time.Time `json:"time"`
}

// Функция для получения текущих котировок
func GetCurrentQuotes(symbol string) (*Quote, error) { //  Изменено:  возвращаем  *Quote
	// Формирование URL запроса
	quoteURL := fmt.Sprintf("https://trade-api.finam.ru/v1/quotes?symbols=%s", symbol)

	// Логирование отправки запроса
	logger.Logger.Info("Sending request for quotes",
		zap.String("symbol", symbol),
		zap.String("url", quoteURL))

	// Выполнение запроса
	resp, err := http.Get(quoteURL)
	if err != nil {
		// Логирование ошибки при отправке запроса
		logger.Logger.Error("Error making request to Trade API",
			zap.Error(err))
		return nil, fmt.Errorf("error making request to Trade API: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки API Финам
		logger.Logger.Error("Finam API error - GetCurrentQuotes",
			zap.Int("status_code", resp.StatusCode),
			zap.String("symbol", symbol))

		// Обработка ошибок на основе кода статуса
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("too many requests to Trade API, try again later")
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("unauthorized access to Trade API, check your credentials")
		case http.StatusBadRequest:
			return nil, fmt.Errorf("bad request: check your request parameters and JSON format")
		case http.StatusForbidden:
			return nil, fmt.Errorf("forbidden: check your access token and permissions")
		case http.StatusInternalServerError:
			return nil, fmt.Errorf("internal server error: try again later or contact Finam support")
		default:
			return nil, fmt.Errorf("trade API request failed with status code: %d", resp.StatusCode)
		}
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// Логирование ошибки при чтении ответа
		logger.Logger.Error("Error reading response body from Trade API",
			zap.Error(err))
		return nil, fmt.Errorf("error reading response body from Trade API: %w", err)
	}

	// Парсинг JSON ответа
	var quotes []Quote
	err = json.Unmarshal(body, &quotes)
	if err != nil {
		// Логирование ошибки при парсинге JSON ответа
		logger.Logger.Error("Error parsing JSON response from Trade API",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing JSON response from Trade API: %w", err)
	}

	if len(quotes) == 0 {
		// Логирование отсутствия котировок для символа
		logger.Logger.Warn("No quotes found for symbol",
			zap.String("symbol", symbol))
		return nil, fmt.Errorf("no quotes found for symbol: %s", symbol)
	}

	// Логирование успешного получения котировок
	logger.Logger.Info("Quotes received successfully",
		zap.String("symbol", symbol),
		zap.Float64("price", quotes[0].Price),
		zap.Int64("volume", quotes[0].Volume))

	return &quotes[0], nil
}

// Структура для хранения информации об инструменте
type FinamInstrumentInfo struct {
	Symbol    string  `json:"symbol"`
	LotSize   int     `json:"lot_size"`
	PriceStep float64 `json:"price_step"`
	Decimals  int     `json:"decimals"`
	// ... другие поля
}

// Функция для получения информации об инструменте
func GetInstrumentInfo(symbol string) (*FinamInstrumentInfo, error) {
	// Для получения информации об инструменте используем ISS API Московской Биржи.
	// Подробная документация: https://iss.moex.com/iss/reference/
	// Формирование URL запроса к ISS API
	infoURL := fmt.Sprintf("https://iss.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities/%s.json", symbol)

	// Логирование отправки запроса
	logger.Logger.Info("Sending request for instrument info",
		zap.String("symbol", symbol),
		zap.String("url", infoURL))

	// Выполнение запроса
	resp, err := http.Get(infoURL)
	if err != nil {
		// Логирование ошибки при отправке запроса
		logger.Logger.Error("Error making request to ISS API",
			zap.Error(err))
		return nil, fmt.Errorf("error making request to ISS API: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки API Московской биржи
		logger.Logger.Error("Moscow Exchange API error",
			zap.Int("statusCode", resp.StatusCode),
			zap.String("symbol", symbol))

		// Обработка ошибок API на основе кода статуса
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("too many requests to ISS API, try again later")
		default:
			return nil, fmt.Errorf("ISS API request failed with status code: %d", resp.StatusCode)
		}
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// Логирование ошибки при чтении ответа
		logger.Logger.Error("Error reading response body from ISS API",
			zap.Error(err))
		return nil, fmt.Errorf("error reading response body from ISS API: %w", err)
	}

	// Парсинг JSON ответа
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		// Логирование ошибки при парсинге JSON ответа
		logger.Logger.Error("Error parsing JSON response from ISS API",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing JSON response from ISS API: %w", err)
	}

	// Извлечение необходимой информации
	securities := data["securities"].([]interface{})
	if len(securities) == 0 {
		// Логирование отсутствия данных для символа
		logger.Logger.Warn("No securities found for symbol",
			zap.String("symbol", symbol))
		return nil, fmt.Errorf("no securities found for symbol: %s", symbol)
	}

	security := securities[0].(map[string]interface{})

	lotSize, err := strconv.ParseInt(security["LOTSIZE"].(string), 10, 64)
	if err != nil {
		// Логирование ошибки при парсинге LOTSIZE
		logger.Logger.Error("Error parsing LOTSIZE from ISS API response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing LOTSIZE from ISS API response: %w", err)
	}

	priceStep, err := strconv.ParseFloat(security["MINSTEP"].(string), 64)
	if err != nil {
		// Логирование ошибки при парсинге MINSTEP
		logger.Logger.Error("Error parsing MINSTEP from ISS API response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing MINSTEP from ISS API response: %w", err)
	}

	decimals, err := strconv.Atoi(security["DECIMALS"].(string))
	if err != nil {
		// Логирование ошибки при парсинге DECIMALS
		logger.Logger.Error("Error parsing DECIMALS from ISS API response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing DECIMALS from ISS API response: %w", err)
	}

	// Создание структуры с информацией об инструменте
	info := &FinamInstrumentInfo{
		Symbol:    symbol,
		LotSize:   int(lotSize),
		PriceStep: priceStep,
		Decimals:  decimals,
		// ... другие поля, если необходимо
	}

	// Логирование успешного получения информации об инструменте
	logger.Logger.Info("Instrument info received successfully",
		zap.String("symbol", symbol),
		zap.Int("lotSize", info.LotSize),
		zap.Float64("priceStep", info.PriceStep),
		zap.Int("decimals", info.Decimals))

	return info, nil
}

// Функция для проверки статуса сервера Финам
func CheckFinamServerStatus() (bool, error) {
	// Используем Trade API для проверки статуса

	// Логирование проверки статуса сервера
	logger.Logger.Info("Checking Finam server status")

	_, err := GetCurrentQuotes("SBER") // Можно использовать любой ликвидный инструмент
	if err != nil {
		// Логирование ошибки при проверке статуса сервера
		logger.Logger.Error("Error checking Finam server status",
			zap.Error(err))
		return false, err // Возвращаем ошибку из GetCurrentQuotes
	}

	// Логирование успешной проверки статуса сервера
	logger.Logger.Info("Finam server is available")

	return true, nil
}
