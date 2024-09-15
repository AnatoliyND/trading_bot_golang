package order

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
	"trading-bot/logger"

	"go.uber.org/zap"
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

type OrderInfo struct {
	ID        string
	Status    string
	CreatedAt time.Time
	// Добавьте другие поля по необходимости
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
	logger.Logger.Info("Sending order creation request",
		zap.String("symbol", order.Symbol),
		zap.String("side", order.Side),
		zap.Int("quantity", order.Quantity),
		zap.String("order_type", order.OrderType))

	// Отправка HTTP запроса
	resp, err := http.Post(orderURL, "application/json", bytes.NewBuffer(orderJSON))
	if err != nil {
		logger.Logger.Error("Error making order creation request",
			zap.Error(err))
		return nil, fmt.Errorf("error making order creation request: %w", err)
	}
	defer resp.Body.Close()

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Error reading order creation response body",
			zap.Error(err))
		return nil, fmt.Errorf("error reading order creation response body: %w", err)
	}

	// Парсинг JSON ответа
	var orderResp OrderResponse
	err = json.Unmarshal(body, &orderResp)
	if err != nil {
		logger.Logger.Error("Error parsing order creation JSON response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing order creation JSON response: %w", err)
	}

	// Обработка статуса ответа
	if orderResp.Status != "success" {
		logger.Logger.Error("Order creation failed",
			zap.String("status", orderResp.Status),
			zap.String("message", orderResp.Message))
		return nil, fmt.Errorf("order creation failed: %s", orderResp.Message)
	}

	// Логирование успешного создания ордера
	logger.Logger.Info("Order created successfully",
		zap.Int("order_id", orderResp.OrderID))

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
	logger.Logger.Info("Sending order cancellation request",
		zap.Int("order_id", orderID))

	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Error making order cancellation request",
			zap.Error(err))
		return fmt.Errorf("error making order cancellation request: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа (опционально, если требуется обработка ответов)
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки при отмене ордера
		logger.Logger.Error("Finam API error - CancelOrder",
			zap.Int("status_code", resp.StatusCode),
			zap.Int("order_id", orderID))
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
	logger.Logger.Info("Sending get order status request",
		zap.Int("order_id", orderID))

	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Error making get order status request",
			zap.Error(err))
		return nil, fmt.Errorf("error making get order status request: %w", err)
	}
	defer resp.Body.Close()

	// Проверка статуса ответа
	if resp.StatusCode != http.StatusOK {
		// Логирование ошибки при получении статуса ордера
		logger.Logger.Error("Finam API error - GetOrderStatus",
			zap.Int("status_code", resp.StatusCode),
			zap.Int("order_id", orderID))

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
			return nil, fmt.Errorf("trade API request failed with status code: %d", resp.StatusCode)
		}
	}

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Error reading get order status response body",
			zap.Error(err))
		return nil, fmt.Errorf("error reading get order status response body: %w", err)
	}

	// Парсинг JSON ответа
	var orderResp OrderResponse
	err = json.Unmarshal(body, &orderResp)
	if err != nil {
		logger.Logger.Error("Error parsing get order status JSON response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing get order status JSON response: %w", err)
	}

	// Логирование успешного получения статуса ордера
	logger.Logger.Info("Get order status successfully",
		zap.Int("order_id", orderResp.OrderID),
		zap.String("status", orderResp.Status))
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
	logger.Logger.Info("Sending get portfolio info request")
	// Выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Error making get portfolio info request",
			zap.Error(err))
		return nil, fmt.Errorf("error making get portfolio info request: %w", err)
	}
	defer resp.Body.Close()

	// Чтение ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Error reading get portfolio info response body",
			zap.Error(err))
		return nil, fmt.Errorf("error reading get portfolio info response body: %w", err)
	}
	// Парсинг JSON ответа
	var portfolioInfo PortfolioInfo
	err = json.Unmarshal(body, &portfolioInfo)
	if err != nil {
		logger.Logger.Error("Error parsing get portfolio info JSON response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing get portfolio info JSON response: %w", err)
	}

	// Логирование успешного получения статуса ордера
	logger.Logger.Info("Get portfolio info successfully")
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
	logger.Logger.Info("Sending orders history request",
		zap.Time("from", req.From),
		zap.Time("to", req.To),
		zap.String("symbol", req.Symbol))

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
		logger.Logger.Error("Error making orders history request",
			zap.Error(err))
		return nil, fmt.Errorf("error making orders history request: %w", err)
	}
	defer resp.Body.Close()

	//  Проверка  статуса  ответа
	if resp.StatusCode != http.StatusOK {
		//  Логирование  ошибки  API  Финам
		logger.Logger.Error("Finam  API  error  -  GetOrdersHistory",
			zap.Int("status_code", resp.StatusCode))

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
		logger.Logger.Error("Error reading orders history response body",
			zap.Error(err))
		return nil, fmt.Errorf("error reading orders history response body: %w", err)
	}

	// Парсинг JSON ответа
	var historyResp OrdersHistoryResponse
	err = json.Unmarshal(body, &historyResp)
	if err != nil {
		logger.Logger.Error("Error parsing orders history JSON response",
			zap.Error(err))
		return nil, fmt.Errorf("error parsing orders history JSON response: %w", err)
	}

	// Логирование успешного получения истории ордеров
	logger.Logger.Info("Orders history received successfully",
		zap.Int("orders_count", len(historyResp.Orders)))

	return &historyResp, nil
}

// Функция для изменения ордера (с отменой и созданием нового ордера). Функция принимает orderID (ID ордера, который нужно изменить) и newOrder (структуру OrderRequest с параметрами нового ордера)
func ModifyOrder(orderID int, newOrder *OrderRequest) (*OrderResponse, error) {
	// Логирование начала изменения ордера
	logger.Logger.Info("Modifying order",
		zap.Int("order_id", orderID))

	// Отмена старого ордера
	err := CancelOrder(orderID, newOrder.AccessToken)
	if err != nil {
		logger.Logger.Error("Error canceling order",
			zap.Error(err))
		return nil, fmt.Errorf("error canceling order: %w", err)
	}

	// Логирование успешной отмены ордера
	logger.Logger.Info("Order canceled successfully",
		zap.Int("order_id", orderID))

	// Создание нового ордера
	orderResp, err := CreateOrder(newOrder)
	if err != nil {
		logger.Logger.Error("Error creating new order",
			zap.Error(err))
		return nil, fmt.Errorf("error creating new order: %w", err)
	}

	// Логирование успешного создания нового ордера
	logger.Logger.Info("New order created successfully",
		zap.Int("new_order_id", orderResp.OrderID))

	return orderResp, nil
}

// Функция для получения информации о состоянии ордера (заявки) с торговой площадки
func GetOrderInfo(orderID string) (OrderInfo, error) {
	// URL для запроса информации о состоянии ордера (заявки)
	url := "https://iss.moex.com/iss/engines/stock/markets/shares/boards/TQBR/securities.xml?q=SECID=" + orderID

	// Отправляем GET-запрос
	resp, err := http.Get(url)
	if err != nil {
		return OrderInfo{}, err
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OrderInfo{}, err
	}

	// Парсим XML
	var xmlData struct {
		Securities struct {
			Security []struct {
				ID        string
				Status    string
				CreatedAt string
			}
		}
	}
	err = xml.Unmarshal(body, &xmlData)
	if err != nil {
		return OrderInfo{}, err
	}

	// Находим информацию о состоянии ордера (заявки)
	for _, security := range xmlData.Securities.Security {
		if security.ID == orderID {
			// Парсим дату создания
			createdAt, err := time.Parse("2006-01-02T15:04:05", security.CreatedAt)
			if err != nil {
				return OrderInfo{}, err
			}

			// Возвращаем информацию о состоянии ордера (заявки)
			return OrderInfo{
				ID:        security.ID,
				Status:    security.Status,
				CreatedAt: createdAt,
			}, nil
		}
	}

	// Если информация о состоянии ордера (заявки) не найдена, возвращаем ошибку
	return OrderInfo{}, fmt.Errorf("информация о состоянии ордера (заявки) не найдена")
}
