package utils

import (
	"fmt"
	"math"
	"time"

	"github.com/go-gota/gota/dataframe"
	"github.com/go-gota/gota/series"
)

// Функция для преобразования строки в time.Time
func ParseTime(timeString string, format string) (time.Time, error) {
	return time.Parse(format, timeString)
}

// Функция для расчета ATR Channels
func CalculateATRChannels(df *dataframe.DataFrame, period int, multiplier float64) (*dataframe.DataFrame, error) {
	// 1. Проверка наличия необходимых столбцов в DataFrame
	columnNames := df.Names()
	hasColumn := func(name string) bool {
		for _, colName := range columnNames {
			if colName == name {
				return true
			}
		}
		return false
	}
	if !hasColumn("High") || !hasColumn("Low") || !hasColumn("Close") {
		return nil, fmt.Errorf("dataframe не содержит столбцов 'High', 'Low' или 'Close'")
	}

	// Создаем копию DataFrame
	result := df.Copy()

	// 2. Расчет Average True Range (ATR)
	high := result.Col("High").Float()
	low := result.Col("Low").Float()
	close := result.Col("Close").Float()

	trueRange := make([]float64, len(high))
	for i := range high {
		if i == 0 {
			trueRange[i] = high[i] - low[i]
		} else {
			trueRange[i] = math.Max(high[i]-low[i], math.Max(math.Abs(high[i]-close[i-1]), math.Abs(low[i]-close[i-1])))
		}
	}

	// Рассчитываем SMA для ATR
	atr := make([]float64, len(trueRange))
	for i := range trueRange {
		if i < period-1 {
			atr[i] = 0 // Пока не накопили достаточно данных для SMA
		} else {
			sum := 0.0
			for j := i - period + 1; j <= i; j++ {
				sum += trueRange[j]
			}
			atr[i] = sum / float64(period)
		}
	}

	// Добавляем ATR в DataFrame
	atrSeries := series.New(atr, series.Float, "ATR")
	result = result.Mutate(atrSeries)

	// 3. Расчет каналов ATR
	// Преобразуем данные в массивы float64 для выполнения арифметических операций
	closePrices := result.Col("Close").Float()
	upperChannel := make([]float64, len(closePrices))
	lowerChannel := make([]float64, len(closePrices))

	for i := range closePrices {
		upperChannel[i] = closePrices[i] + atr[i]*multiplier
		lowerChannel[i] = closePrices[i] - atr[i]*multiplier
	}

	// 4. Добавление каналов ATR в DataFrame
	upperChannelSeries := series.New(upperChannel, series.Float, "UpperChannel")
	lowerChannelSeries := series.New(lowerChannel, series.Float, "LowerChannel")

	result = result.Mutate(upperChannelSeries)
	result = result.Mutate(lowerChannelSeries)

	return &result, nil
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
