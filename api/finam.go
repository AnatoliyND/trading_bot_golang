package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
	"trading-bot/data"
	"trading-bot/logger"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// Структура для хранения конфигурации Finam API
type FinamConfig struct {
	AccessToken string `json:"access_token"`
}

// Структура для работы с API Финам
type FinamAPI struct {
	config *FinamConfig
	token  *oauth2.Token
}

// Структура для хранения информации об инструменте (Trade API)
type Instrument struct {
	ID            int    `json:"id"`
	Symbol        string `json:"symbol"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Exchange      string `json:"exchange"`
	TradingStatus string `json:"tradingStatus"`
	// ... другие  поля,  если  необходимо ...
}

// ApiResponse определение структуры ответа API
type ApiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// Функция для создания нового объекта FinamAPI
func NewFinamAPI(config *FinamConfig) (*FinamAPI, error) {
	token := &oauth2.Token{
		AccessToken: config.AccessToken,
		TokenType:   "Bearer",
	}

	return &FinamAPI{
		config: config,
		token:  token,
	}, nil
}

// Функция для загрузки исторических данных с API Финам (Trade API)
func (f *FinamAPI) LoadHistoricalData(symbol string, startDate time.Time, endDate time.Time, interval string) (*data.FinamData, error) {
	// Формирование URL запроса
	// Документация: https://finamweb.github.io/trade-api-docs/candles/
	url := fmt.Sprintf("https://trade-api.finam.ru/v1/candles?symbol=%s&from=%s&to=%s&interval=%s",
		symbol, startDate.Format(time.RFC3339), endDate.Format(time.RFC3339), interval)

	// Логирование
	logger.Logger.Info("Запрос исторических данных от API",
		zap.String("symbol", string(symbol)),
		zap.String("startDate", string(time.RFC3339)),
		zap.String("endDate", string(time.RFC3339)),
		zap.String("interval", string(interval)),
		zap.String("URL", string(url)))

	// Создание запроса
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token.AccessToken)

	// Отправка запроса
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неуспешный статус ответа: %d", resp.StatusCode)
	}

	// Чтение тела ответа
	body, err := io.ReadAll(resp.Body) //  Используем  io.ReadAll
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела ответа: %w", err)
	}

	// Декодирование JSON ответа
	var candles data.FinamData
	err = json.Unmarshal(body, &candles)
	if err != nil {
		return nil, fmt.Errorf("ошибка декодирования JSON ответа: %w", err)
	}

	// Логирование
	logger.Logger.Info("Исторические данные получены от API",
		zap.String("symbol", string(symbol)),
		zap.Int("количество свечей", len(candles.C)))

	return &candles, nil
}

// Функция для получения списка инструментов с Trade API
func (f *FinamAPI) GetInstruments() ([]Instrument, error) {
	// 1. Формирование URL запроса
	url := "https://trade-api.finam.ru/v1/securities"

	// 2. Логирование
	logger.Logger.Info("Запрос списка инструментов",
		zap.String("URL", url))

	// 3. Создание HTTP запроса
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token.AccessToken)

	// 4. Отправка запроса
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	// 5. Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("неуспешный статус ответа: %d", resp.StatusCode)
	}

	// 6. Чтение тела ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела ответа: %w", err)
	}

	// 7. Декодирование JSON ответа
	var instruments []Instrument
	if err := json.Unmarshal(body, &instruments); err != nil {
		return nil, fmt.Errorf("ошибка декодирования JSON ответа: %w", err)
	}

	// 8. Логирование
	logger.Logger.Info("Список инструментов получен",
		zap.Int("количество инструментов", len(instruments)))

	return instruments, nil
}

func asyncApiRequest(url string, ch chan<- ApiResponse) {
	// Выполняем запрос
	response, err := http.Get(url)
	if err != nil {
		log.Printf("Ошибка запроса: %s", err)
		return
	}
	defer response.Body.Close()

	// Парсим результат
	var result ApiResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		log.Printf("Ошибка парсинга ответа: %s", err)
	}
	ch <- result
}

func fetchMarketData(urls []string) {
	ch := make(chan ApiResponse, len(urls))

	for _, url := range urls {
		go asyncApiRequest(url, ch)
	}

	for range urls {
		result := <-ch
		log.Printf("Результат: %+v", result)
	}
}

// FetchMarketData fetches market data from multiple URLs asynchronously.
func FetchMarketData(urls []string) ([]ApiResponse, error) {
	ch := make(chan ApiResponse, len(urls))
	var results []ApiResponse

	// Асинхронный запрос для каждого URL
	for _, url := range urls {
		go asyncApiRequest(url, ch)
	}

	// Ожидание всех ответов
	for range urls {
		result := <-ch
		if result.Status != "success" {
			return nil, fmt.Errorf("error fetching market data: %s", result.Message)
		}
		results = append(results, result)
	}

	return results, nil
}
