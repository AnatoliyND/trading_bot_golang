package model

import (
	"fmt"
	"os"

	"encoding/gob"

	"github.com/go-gota/gota/dataframe"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// Структура для представления модели нейронной сети.
type Model struct {
	graph   *gorgonia.ExprGraph
	vm      gorgonia.VM
	input   *gorgonia.Node
	output  *gorgonia.Node
	weights map[string]*gorgonia.Node
}

// Функция для создания новой модели.
func NewModel() (*Model, error) {
	// Создание графа Gorgonia:
	graph := gorgonia.NewGraph()

	// Входной слой (создание ноды для входных данных). Определение входного слоя:
	input := gorgonia.NewTensor(graph, tensor.Float32, 2, gorgonia.WithShape(-1, 3), gorgonia.WithName("input")) //Размер входного слоя: gorgonia.WithShape(-1, 3) в input означает, что модель ожидает входные данные с 3 признаками. Убедитесь, что это соответствует вашим данным. Если у вас другое количество признаков, измените значение 3 на соответствующее число.

	// Скрытый слой. Инициализация весов и смещений. Создание скрытого слоя с весами, смещением и ReLU активацией:
	hiddenWeights := gorgonia.NewMatrix(graph, tensor.Float32, gorgonia.WithShape(3, 100), gorgonia.WithName("hidden_weights"), gorgonia.WithInit(gorgonia.GlorotU(1)))
	hiddenBias := gorgonia.NewVector(graph, tensor.Float32, gorgonia.WithShape(100), gorgonia.WithName("hidden_bias"), gorgonia.WithInit(gorgonia.Zeroes()))

	hiddenLayer := gorgonia.Must(gorgonia.Add(gorgonia.Must(gorgonia.Mul(input, hiddenWeights)), hiddenBias))
	hiddenLayer = gorgonia.Must(gorgonia.Rectify(hiddenLayer)) // ReLU активация

	// Выходной слой. Создание выходного слоя с весами и смещением:
	outputWeights := gorgonia.NewMatrix(graph, tensor.Float32, gorgonia.WithShape(100, 1), gorgonia.WithName("output_weights"), gorgonia.WithInit(gorgonia.GlorotU(1)))
	outputBias := gorgonia.NewVector(graph, tensor.Float32, gorgonia.WithShape(1), gorgonia.WithName("output_bias"), gorgonia.WithInit(gorgonia.Zeroes()))

	output := gorgonia.Must(gorgonia.Add(gorgonia.Must(gorgonia.Mul(hiddenLayer, outputWeights)), outputBias))

	// Создание виртуальной машины (VM) для выполнения графа
	vmachine := gorgonia.NewTapeMachine(graph)

	model := &Model{
		graph:   graph,
		vm:      vmachine,
		input:   input,
		output:  output,
		weights: map[string]*gorgonia.Node{"hiddenWeights": hiddenWeights, "hiddenBias": hiddenBias, "outputWeights": outputWeights, "outputBias": outputBias},
	}

	return model, nil
}

// Функция для обучения модели Train должна выполнять следующие действия: 1. Подготовить входные и выходные данные. 2. Определить функцию потерь (например, среднеквадратичную ошибку). 3. Выполнить оптимизацию (например, градиентный спуск) для обновления весов модели. 4. Выполнить несколько эпох для улучшения модели.
func (m *Model) Train(inputData, outputData *dataframe.DataFrame, epochs int, learningRate float64) error {
	// Преобразуем входные данные в тензор. Подготовка данных: Преобразуем входные и выходные данные из dataframe.DataFrame в тензоры Gorgonia, используя tensor.New и gorgonia.WithValue.
	xTensor := tensor.New(tensor.Of(tensor.Float32), tensor.WithShape(inputData.Nrow(), inputData.Ncol()), tensor.WithBacking(inputData.Records()))
	yTensor := tensor.New(tensor.Of(tensor.Float32), tensor.WithShape(outputData.Nrow(), 1), tensor.WithBacking(outputData.Records()))

	// Создаем ноды для входных и выходных данных
	x := gorgonia.NewTensor(m.graph, tensor.Float32, 2, gorgonia.WithShape(inputData.Nrow(), inputData.Ncol()), gorgonia.WithValue(xTensor))
	y := gorgonia.NewTensor(m.graph, tensor.Float32, 2, gorgonia.WithShape(outputData.Nrow(), 1), gorgonia.WithValue(yTensor))

	// Связываем входные данные с моделью
	gorgonia.Let(m.input, x)

	// Определяем функцию потерь: Используется среднеквадратичная ошибка (MSE) - распространенный выбор для задач регрессии.
	loss := gorgonia.Must(gorgonia.Mean(gorgonia.Must(gorgonia.Square(gorgonia.Must(gorgonia.Sub(m.output, y))))))

	// Вычисляем градиенты
	grads, err := gorgonia.Grad(loss, m.weights["hiddenWeights"], m.weights["hiddenBias"], m.weights["outputWeights"], m.weights["outputBias"])
	if err != nil {
		return err
	}

	// Используем оптимизатор для обновления весов. Применён оптимизатор Adam (gorgonia.NewAdamSolver)
	solver := gorgonia.NewAdamSolver(gorgonia.WithLearnRate(learningRate))

	// Оптимизация. Цикл обучения: Выполняется указанное количество эпох (epochs), на каждой эпохе вычисляются градиенты, выполняется шаг оптимизации, и сбрасывается виртуальная машина.
	for i := 0; i < epochs; i++ {
		// Прогоняем модель
		if err := m.vm.RunAll(); err != nil {
			return err
		}

		// Обновляем веса
		if err := solver.Step(gorgonia.NodesToValueGrads(grads)); err != nil {
			return err
		}

		// Вычисляем ошибку на тренировочном наборе
		if err := m.vm.RunAll(); err != nil {
			return err
		}
		trainLoss := loss.Value().Data().(float32)
		fmt.Printf("Epoch %d, training loss: %f\n", i+1, trainLoss)

		// Сбрасываем виртуальную машину перед следующим прогоном
		m.vm.Reset()
	}

	return nil
}

// Функция для прогнозирования Predict будет принимать новые входные данные и выполнять прогноз на основе обученной модели. Она должна: 1. Подготовить входные данные. 2. Выполнить прогноз (прямой проход). 3. Вернуть результат в формате DataFrame.
func (m *Model) Predict(inputData *dataframe.DataFrame) (*dataframe.DataFrame, error) {
	// Подготовка входных данных: Входные данные преобразуются в тензор Gorgonia.
	xTensor := tensor.New(tensor.Of(tensor.Float32), tensor.WithShape(inputData.Nrow(), inputData.Ncol()), tensor.WithBacking(inputData.Records()))

	// Связываем тензор с нодой модели
	gorgonia.Let(m.input, xTensor)

	//  Прогноз: Выполняется прямой проход через модель (m.vm.RunAll()), и результат извлекается из выходного узла (m.output.Value())
	if err := m.vm.RunAll(); err != nil {
		return nil, err
	}

	// Извлекаем результат
	outputVal := m.output.Value()

	// Преобразуем результат в DataFrame. Преобразование результата: Результат прогноза преобразуется в dataframe.DataFrame для удобства использования.
	outputTensor, ok := outputVal.(*tensor.Dense)
	if !ok {
		return nil, fmt.Errorf("не удалось преобразовать выходной результат в тензор")
	}
	records := outputTensor.Data().([]float32)

	// Создаем DataFrame из результатов прогноза
	outputDf := dataframe.LoadRecords([][]string{
		{"Predictions"}, // Имя столбца
	})

	for _, record := range records {
		outputDf = outputDf.RBind(dataframe.LoadRecords([][]string{
			{fmt.Sprintf("%f", record)},
		}))
	}

	m.vm.Reset() // Сбрасываем виртуальную машину перед следующим запуском
	return &outputDf, nil
}

// Функция для сохранения модели. Создает gob.Encoder для кодирования данных в файл.
func (m *Model) Save(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	encoder := gob.NewEncoder(f)

	// Кодирует граф модели (m.graph) и веса (m.weights) с помощью encoder.Encode()
	if err := encoder.Encode(m.graph); err != nil {
		return fmt.Errorf("failed to encode graph: %w", err)
	}

	// Кодируем веса
	if err := encoder.Encode(m.weights); err != nil {
		return fmt.Errorf("failed to encode weights: %w", err)
	}

	return nil
}

// Функция для загрузки модели.
func (m *Model) Load(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	decoder := gob.NewDecoder(f) //Создает gob.Decoder для декодирования данных из файла

	// Декодирует граф модели (m.graph) и веса (m.weights) с помощью decoder.Decode()
	if err := decoder.Decode(&m.graph); err != nil {
		return fmt.Errorf("failed to decode graph: %w", err)
	}

	// Декодируем веса
	if err := decoder.Decode(&m.weights); err != nil {
		return fmt.Errorf("failed to decode weights: %w", err)
	}

	// Создает новую виртуальную машину (m.vm) для загруженного графа
	m.vm = gorgonia.NewTapeMachine(m.graph)

	return nil
}
