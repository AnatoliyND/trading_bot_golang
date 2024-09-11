package order

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
	"trading-bot/logger"
)

// Структура для создания нового ордера
type OrderRequest struct {
	Symbol      string  `json:"symbol"`       // Тикер инструмента
	Side        string  `json:"side"`         // Направление: "buy" или "sell"
	Quantity    int     `json:"quantity"`     // Количество лотов
	OrderType   string  `json:"order_type"`   // Тип ордера: "market" или "limit"
	Price       float64 `json:"price"`        // Цена (для лимитных ордеров)
	StopPrice   float64 `json:"stop_price"`   // Стоп-цена (для стоп-ордеров)
	Validity    string  `json:"validity"`     // Срок действия ордера: "day", "fill-or-kill"
	ClientCode  string  `json:"client_code"`  // Код клиента (если требуется)
	BrokerCode  string  `json:"broker_code"`  // Код брокера (если требуется)
	AccountID   string  `json:"account_id"`   // Идентификатор счета (если требуется)
	Comment     string  `json:"comment"`      // Комментарий к ордеру
	AccessToken string  `json:"access_token"` // Токен доступа
}

// Структура для ответа от API при создании/получении статуса ордера
type OrderResponse struct {
	OrderID int    `json:"order_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Структура для хранения информации о портфеле
type PortfolioInfo struct {
	Account   AccountInfo        `json:"account"`
	Balances  map[string]Balance `json:"balances"`
	Positions []Position         `json:"positions"`
}

type AccountInfo struct {
	AccountID   string `json:"accountId"`
	ClientCode  string `json:"clientCode"`
	BrokerCode  string `json:"brokerCode"`
	AccountName string `json:"accountName"`
	Currency    string `json:"currency"`
}

type Balance struct {
	Value     float64 `json:"value"`
	Available float64 `json:"available"`
	Blocked   float64 `json:"blocked"`
}

type Position struct {
	Symbol        string    `json:"symbol"`
	Quantity      int       `json:"quantity"`
	AveragePrice  float64   `json:"averagePrice"`
	CurrentPrice  float64   `json:"currentPrice"`
	MarketValue   float64   `json:"marketValue"`
	ExpectedYield float64   `json:"expectedYield"` // Ожидаемая доходность. рассчитывается на основе текущей цены и средней цены покупки.
	ProfitLoss    float64   `json:"profitLoss"`    // Прибыль/убыток. Аналогично ожидаемой доходности, для расчета текущей прибыли/убытка по позиции
	OpenDate      time.Time `json:"openDate"`      // Дата открытия позиции. Эта информация может быть полезна для анализа эффективности ваших торговых решений в зависимости от времени.
	LastTradeTime time.Time `json:"lastTradeTime"` // Время последней сделки. Позволит отслеживать активность по инструменту.
}

// Структура для запроса истории ордеров
type OrdersHistoryRequest struct {
	From   time.Time `json:"from"`   // Начало периода
	To     time.Time `json:"to"`     // Конец периода
	Symbol string    `json:"symbol"` // Тикер инструмента (необязательно)
	Status string    `json:"status"` // Статус ордера (необязательно)
	// ... другие поля, если необходимо
}

// Структура для ответа с историей ордеров
type OrdersHistoryResponse struct {
	Orders []OrderHistory `json:"orders"`
}

// Структура для одного ордера в истории
type OrderHistory struct {
	OrderID       int       `json:"order_id"`
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"`
	Quantity      int       `json:"quantity"`
	OrderType     string    `json:"order_type"`
	Price         float64   `json:"price"`
	StopPrice     float64   `json:"stop_price"`
	Status        string    `json:"status"`
	ExecutionTime time.Time `json:"execution_time"`
	// ... другие поля, если необходимо
}

// Структура для запроса изменения ордера
type ModifyOrderRequest struct {
	OrderID     int     `json:"order_id"`
	Quantity    int     `json:"quantity"`     // Новое количество (необязательно)
	Price       float64 `json:"price"`        // Новая цена (необязательно)
	StopPrice   float64 `json:"stop_price"`   // Новая стоп-цена (необязательно)
	AccessToken string  `json:"access_token"` // Токен доступа
	// ... другие поля, если необходимо
}

// Структура для ответа на запрос изменения ордера
type ModifyOrderResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Функция для создания нового ордера
func CreateOrder(order *OrderRequest) (*OrderResponse, error) {
	// Формирование URL запроса
	orderURL := "https://trade-api.finam.ru/v1/orders" // TODO: Проверить правильность URL в документации Trade API

	// Преобразование структуры ордера в JSON
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return nil, fmt.Errorf("error marshaling order request: %w", err)
	}

	// Логирование отправки запроса на создание ордера
	logger.Logger.Info().
		Str("symbol", order.Symbol).
		Str("side", order.Side).
		Int("quantity", order.Quantity).
		Str("order_type", order.OrderType).
		Msg("Sending order creation request")

	// Отправка HTTP запроса
	resp, err := http.Post(orderURL, "application/json", bytes.NewBuffer(orderJSON))
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error making order creation request")
		return nil, fmt.Errorf("error making order creation request: %w", err)
	}
	defer resp.Body.Close()

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error reading order creation response body")
		return nil, fmt.Errorf("error reading order creation response body: %w", err)
	}

	// Парсинг JSON ответа
	var orderResp OrderResponse
	err = json.Unmarshal(body, &orderResp)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error parsing order creation JSON response")
		return nil, fmt.Errorf("error parsing order creation JSON response: %w", err)
	}

	// Обработка статуса ответа
	if orderResp.Status != "success" {
		logger.Logger.Error().
			Str("status", orderResp.Status).
			Str("message", orderResp.Message).
			Msg("Order creation failed")
		return nil, fmt.Errorf("order creation failed: %s", orderResp.Message)
	}

	// Логирование успешного создания ордера
	logger.Logger.Info().
		Int("order_id", orderResp.OrderID).
		Msg("Order created successfully")

	return &orderResp, nil
}

// Функция для отмены ордера
func CancelOrder(orderID int, accessToken string) error {
	// Формирование URL запроса
	cancelURL := fmt.Sprintf("https://trade-api.finam.ru/v1/orders/%d", orderID) // TODO: Проверить URL в документации

	// Создание HTTP клиента и запроса с токеном доступа
	client := &http.Client{}
	req, err := http.NewRequest("DELETE", cancelURL, nil)
	if err != nil {
		return fmt.Errorf("error creating cancel order request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Логирование отправки запроса на отмену ордера
	logger.Logger.Info().Int("order_id", orderID).Msg("Sending order cancellation request")

	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error making order cancellation request")
		return fmt.Errorf("error making order cancellation request: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа (опционально, если требуется обработка ответов)
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки при отмене ордера
		logger.Logger.Error().
			Int("status_code", resp.StatusCode).
			Int("order_id", orderID).
			Msg("Finam API error - CancelOrder")
	}
	// Обработка ответа (если требуется)
	// ...

	return nil

}

// Функция для получения статуса ордера
func GetOrderStatus(orderID int, accessToken string) (*OrderResponse, error) {
	// Формирование URL запроса
	statusURL := fmt.Sprintf("https://trade-api.finam.ru/v1/orders/%d", orderID) // TODO: Проверить URL в документации
	// Создание HTTP клиента и запроса с токеном доступа
	client := &http.Client{}
	req, err := http.NewRequest("GET", statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating get order status request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Логирование отправки запроса на получение статуса ордера
	logger.Logger.Info().Int("order_id", orderID).Msg("Sending get order status request")

	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error making get order status request")
		return nil, fmt.Errorf("error making get order status request: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки при получении статуса ордера
		logger.Logger.Error().
			Int("status_code", resp.StatusCode).
			Int("order_id", orderID).
			Msg("Finam API error - GetOrderStatus")

		// Обработка ошибок на основе кода статуса
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("too many requests to Trade API, try again later")
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("unauthorized access to Trade API, check your credentials")
		case http.StatusBadRequest:
			return nil, fmt.Errorf("bad request: check your request parameters")
		case http.StatusForbidden:
			return nil, fmt.Errorf("forbidden: check your access token and permissions")
		case http.StatusInternalServerError:
			return nil, fmt.Errorf("internal server error: try again later or contact Finam support")
		default:
			return nil, fmt.Errorf("Trade API request failed with status code: %d", resp.StatusCode)
		}
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error reading get order status response body")
		return nil, fmt.Errorf("error reading get order status response body: %w", err)
	}

	// Парсинг JSON ответа
	var orderResp OrderResponse
	err = json.Unmarshal(body, &orderResp)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error parsing get order status JSON response")
		return nil, fmt.Errorf("error parsing get order status JSON response: %w", err)
	}

	// Логирование успешного получения статуса ордера
	logger.Logger.Info().
		Int("order_id", orderResp.OrderID).
		Str("status", orderResp.Status).
		Msg("Get order status successfully")
	return &orderResp, nil
}

// Функция для получения информации о портфеле
func GetPortfolioInfo(accessToken string) (*PortfolioInfo, error) {
	// Формирование URL запроса
	portfolioURL := "https://trade-api.finam.ru/v1/portfolio" // TODO: Проверить URL в документации
	// Создание HTTP клиента и запроса с токеном доступа
	client := &http.Client{}
	req, err := http.NewRequest("GET", portfolioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating get portfolio info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Логирование отправки запроса на получение статуса ордера
	logger.Logger.Info().Msg("Sending get portfolio info request")
	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error making get portfolio info request")
		return nil, fmt.Errorf("error making get portfolio info request: %w", err)
	}
	defer resp.Body.Close()

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error reading get portfolio info response body")
		return nil, fmt.Errorf("error reading get portfolio info response body: %w", err)
	}
	// Парсинг JSON ответа
	var portfolioInfo PortfolioInfo
	err = json.Unmarshal(body, &portfolioInfo)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error parsing get portfolio info JSON response")
		return nil, fmt.Errorf("error parsing get portfolio info JSON response: %w", err)
	}

	// Логирование успешного получения статуса ордера
	logger.Logger.Info().Msg("Get portfolio info successfully")
	return &portfolioInfo, nil
}

// Тест GetPortfolioInfo
func TestGetPortfolioInfo(t *testing.T) {
	// Замените  "your_access_token" на  ваш  реальный  токен  доступа
	accessToken := "your_access_token"

	portfolio, err := GetPortfolioInfo(accessToken)
	if err != nil {
		t.Errorf("GetPortfolioInfo()  returned  error:  %v", err)
	}

	// Проверка  основных  полей
	if portfolio.Account.AccountID == "" {
		t.Errorf("AccountID  is  empty")
	}
	if len(portfolio.Balances) == 0 {
		t.Errorf("Balances  is  empty")
	}
	if len(portfolio.Positions) == 0 {
		t.Errorf("Positions  is  empty")
	}

	// Пример  проверки  данных  позиции
	for _, position := range portfolio.Positions {
		if position.Symbol == "" {
			t.Errorf("Position  Symbol  is  empty")
		}
		if position.Quantity == 0 {
			t.Errorf("Position  Quantity  is  zero")
		}
		// ... (добавьте  другие  проверки  при  необходимости)
	}
}

// Функция для получения истории ордеров
func GetOrdersHistory(req *OrdersHistoryRequest, accessToken string) (*OrdersHistoryResponse, error) {
	// Формирование URL запроса
	historyURL := "https://trade-api.finam.ru/v1/orders/history"

	// Преобразование структуры запроса в JSON
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling orders history request: %w", err)
	}

	// Логирование отправки запроса истории ордеров
	logger.Logger.Info().
		Time("from", req.From).
		Time("to", req.To).
		Str("symbol", req.Symbol).
		Msg("Sending orders history request")

	// Создание HTTP клиента и запроса с токеном доступа
	client := &http.Client{}
	httpReq, err := http.NewRequest("POST", historyURL, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("error creating orders history request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	// Выполнение запроса
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error making orders history request")
		return nil, fmt.Errorf("error making orders history request: %w", err)
	}
	defer resp.Body.Close()

	//  Проверка  статуса  ответа
	if resp.StatusCode != http.StatusOK {
		//  Логирование  ошибки  API  Финам
		logger.Logger.Error().
			Int("status_code", resp.StatusCode).
			Msg("Finam  API  error  -  GetOrdersHistory")

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
			return nil, fmt.Errorf("Trade API request failed with status code: %d", resp.StatusCode)
		}
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error reading orders history response body")
		return nil, fmt.Errorf("error reading orders history response body: %w", err)
	}

	// Парсинг JSON ответа
	var historyResp OrdersHistoryResponse
	err = json.Unmarshal(body, &historyResp)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error parsing orders history JSON response")
		return nil, fmt.Errorf("error parsing orders history JSON response: %w", err)
	}

	// Логирование успешного получения истории ордеров
	logger.Logger.Info().
		Int("orders_count", len(historyResp.Orders)).
		Msg("Orders history received successfully")

	return &historyResp, nil
}

// Функция для изменения ордера (с отменой и созданием нового ордера). Функция принимает orderID (ID ордера, который нужно изменить) и newOrder (структуру OrderRequest с параметрами нового ордера)
func ModifyOrder(orderID int, newOrder *OrderRequest) (*OrderResponse, error) {
	// Логирование начала изменения ордера
	logger.Logger.Info().
		Int("order_id", orderID).
		Msg("Modifying order")

	// Отмена старого ордера
	err := CancelOrder(orderID, newOrder.AccessToken)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error canceling order")
		return nil, fmt.Errorf("error canceling order: %w", err)
	}

	// Логирование успешной отмены ордера
	logger.Logger.Info().
		Int("order_id", orderID).
		Msg("Order canceled successfully")

	// Создание нового ордера
	orderResp, err := CreateOrder(newOrder)
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Error creating new order")
		return nil, fmt.Errorf("error creating new order: %w", err)
	}

	// Логирование успешного создания нового ордера
	logger.Logger.Info().
		Int("new_order_id", orderResp.OrderID).
		Msg("New order created successfully")

	return orderResp, nil
}
