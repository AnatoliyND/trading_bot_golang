package connector

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"trading-bot/data"
	"trading-bot/logger"

	"go.uber.org/zap"
)

// Структура для хранения конфигурации Transaq Connector
type TransaqConfig struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// Структура для представления подключения к Transaq Connector
type TransaqConnector struct {
	config   *TransaqConfig
	conn     net.Conn
	messages chan string
	stop     chan bool
}

// Функция для создания нового объекта TransaqConnector
func NewTransaqConnector(config *TransaqConfig) (*TransaqConnector, error) {
	return &TransaqConnector{
		config:   config,
		messages: make(chan string, 100), // Буферизованный канал для сообщений
		stop:     make(chan bool),
	}, nil
}

// Функция для подключения к серверу Transaq
func (t *TransaqConnector) Connect() error {
	// Логирование попытки подключения
	logger.Logger.Info("Connecting to Transaq Connector...",
		zap.String("host", t.config.Host),
		zap.Int("port", t.config.Port))
	address := fmt.Sprintf("%s:%d", t.config.Host, t.config.Port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		// Логирование ошибки при подключении
		logger.Logger.Error("Connection failed.",
			zap.Error(err))
		return fmt.Errorf("error connecting to Transaq Connector: %w", err)
	}

	t.conn = conn
	logger.Logger.Info("Connected to Transaq Connector.")
	return nil
}

// Функция для отправки сообщения на сервер
func (t *TransaqConnector) SendMessage(message string) error {
	// Добавляем завершающий символ новой строки, если его нет
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}

	// Логирование отправки сообщения
	logger.Logger.Debug("Sending message to Transaq Connector...",
		zap.String("message", message))
	_, err := t.conn.Write([]byte(message))
	if err != nil {
		logger.Logger.Error("Failed to send message.",
			zap.Error(err))
		return fmt.Errorf("error sending message to Transaq Connector: %w", err)
	}

	return nil
}

// Функция для получения сообщения с сервера с таймаутом
func (t *TransaqConnector) GetMessageWithTimeout(timeout time.Duration) (string, error) {
	select {
	case message := <-t.messages:
		return message, nil
	case <-t.stop:
		return "", fmt.Errorf("connector stopped")
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout reading message from Transaq Connector")
	}
}

// Функция для обработки событий (вызывается в main.go)
func (t *TransaqConnector) HandleEvent(message string) {
	// Парсинг XML событий и вызов соответствующих функций
	// из других пакетов
	// Пример обработки события new_quote:
	if strings.Contains(message, "<event id=\"new_quote\"") {
		var quote data.Quote
		err := xml.Unmarshal([]byte(message), &quote)
		if err != nil {
			logger.Logger.Error("Failed to parse new_quote event",
				zap.Error(err))
			return
		}
		err = data.UpdateQuotes(&quote) // Функция из пакета data для обновления котировок
		if err != nil {
			logger.Logger.Error("Failed to update quotes",
				zap.Error(err))
		}
	}
	// ... Обработка других событий (new_order, order_cancelled, etc.) ...
}

// Функция для закрытия подключения
func (t *TransaqConnector) Close() {
	close(t.stop)
	if t.conn != nil {
		logger.Logger.Info("Closing connection...")
		t.conn.Close()
	}
}

// Функция для авторизации на сервере
func (t *TransaqConnector) Authorize() error {
	authMessage := fmt.Sprintf("<command id=\"connect\" token=\"%s\"/>", t.config.Token)
	err := t.SendMessage(authMessage)
	if err != nil {
		return err
	}

	// Чтение ответа на авторизацию
	authResponse, err := t.GetMessageWithTimeout(10 * time.Second)
	if err != nil {
		return err
	}

	// Проверка успешной авторизации
	if strings.Contains(authResponse, "connected") {
		logger.Logger.Info("Authorization successful.")
		return nil
	}

	// Логирование ошибки при авторизации
	logger.Logger.Error("Authorization failed.",
		zap.String("response", authResponse))
	return fmt.Errorf("authorization failed: %s", authResponse)
}

// Функция для отправки heartbeat сообщения
func (t *TransaqConnector) sendHeartbeat() error {
	return t.SendMessage("<command id=\"heartbeat\"/>")
}

// Запуск heartbeat
func (t *TransaqConnector) StartHeartbeat(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-ticker.C:
				if err := t.sendHeartbeat(); err != nil {
					logger.Logger.Error("Failed to send heartbeat message",
						zap.Error(err))
				}
			}
		}
	}()
}

// Функция для загрузки конфигурации из JSON файла
func LoadConfigFromFile(filename string) (*TransaqConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening config file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var config TransaqConfig
	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("error decoding config file: %w", err)
	}

	return &config, nil
}

// Функция для загрузки конфигурации из переменных окружения
func LoadConfigFromEnv() (*TransaqConfig, error) {
	host := os.Getenv("TRANSAQ_HOST")
	portStr := os.Getenv("TRANSAQ_PORT")
	token := os.Getenv("TRANSAQ_TOKEN")

	// Проверка наличия всех необходимых переменных
	if host == "" || portStr == "" || token == "" {
		return nil, fmt.Errorf("missing environment variables: TRANSAQ_HOST, TRANSAQ_PORT, TRANSAQ_TOKEN")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("error converting TRANSAQ_PORT to integer: %w", err)
	}

	return &TransaqConfig{
		Host:  host,
		Port:  port,
		Token: token,
	}, nil
}

// Функция для запуска цикла чтения сообщений с сервера
func (t *TransaqConnector) StartReading() {
	go func() {
		defer close(t.messages)
		scanner := bufio.NewScanner(t.conn)
		for scanner.Scan() {
			message := scanner.Text()
			t.messages <- message
			logger.Logger.Debug("Message received from Transaq Connector",
				zap.String("message", message))
		}
		if err := scanner.Err(); err != nil {
			// Логирование ошибки при чтении
			logger.Logger.Error("Error reading message from Transaq Connector",
				zap.Error(err))
		}
	}()
}
