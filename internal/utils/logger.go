package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Logger представляет собой асинхронный логгер с уровнями логирования
type Logger struct {
	consoleWriter io.Writer // Для вывода в консоль (с цветом)
	fileWriter    io.Writer // Для вывода в файл (без цвета)
	logFile       *os.File
	logFormat     string
	infoColor     *color.Color
	warnColor     *color.Color
	errorColor    *color.Color
	logChan       chan logEntry  // Канал для асинхронной отправки сообщений
	wg            sync.WaitGroup // Для ожидания завершения обработки сообщений
	closed        bool           // Флаг для предотвращения записи после закрытия
}

// LogLevel определяет уровни логирования
type LogLevel string

const (
	Info    LogLevel = "INFO"
	Warning LogLevel = "WARNING"
	Error   LogLevel = "ERROR"
)

// logEntry представляет собой одно сообщение лога
type logEntry struct {
	level  LogLevel
	caller string
	file   string
	msg    string
}

// LoggerConfig определяет конфигурацию логгера
type LoggerConfig struct {
	LogToFile   bool   // Включить запись в файл
	LogFilePath string // Путь к файлу логов
	LogFormat   string // Формат строки лога
	BufferSize  int    // Размер буфера для канала
}

// DefaultLoggerConfig возвращает конфигурацию по умолчанию
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		LogToFile:   false,
		LogFilePath: "server.log",
		LogFormat:   "time\t||[level]|| func || message || file",
		BufferSize:  1000, // Размер буфера для канала
	}
}

// NewLogger создает новый экземпляр асинхронного логгера с заданной конфигурацией
func NewLogger(cfg LoggerConfig) (*Logger, error) {
	l := &Logger{
		logFormat:  cfg.LogFormat,
		infoColor:  color.New(color.FgGreen),
		warnColor:  color.New(color.FgYellow),
		errorColor: color.New(color.FgRed),
		logChan:    make(chan logEntry, cfg.BufferSize),
		closed:     false,
	}

	// Настройка вывода в консоль (с цветом)
	l.consoleWriter = os.Stdout

	// Настройка вывода в файл (без цвета)
	if cfg.LogToFile {
		// Создание директории для файла логов, если она не существует
		if err := os.MkdirAll(filepath.Dir(cfg.LogFilePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}

		file, err := os.OpenFile(cfg.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %v", err)
		}
		l.logFile = file
		l.fileWriter = file
	}

	// Запускаем горутину для обработки сообщений
	l.wg.Add(1)
	go l.processLogs()

	return l, nil
}

// processLogs обрабатывает сообщения из канала
func (l *Logger) processLogs() {
	defer l.wg.Done()
	for entry := range l.logChan {
		l.writeLog(entry.level, entry.caller, entry.file, entry.msg)
	}
}

// writeLog форматирует и записывает сообщение лога
func (l *Logger) writeLog(level LogLevel, caller string, file string, message string) {
	// Форматирование времени
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Форматирование каждой части с квадратными скобками
	timePart := fmt.Sprintf("%s", timestamp)
	levelPart := fmt.Sprintf("%s", string(level))
	callerPart := fmt.Sprintf("%s", caller)
	filePart := fmt.Sprintf("%s", file)
	messagePart := message

	// Форматирование строки лога для файла (без цвета)
	logEntry := l.logFormat
	logEntry = strings.ReplaceAll(logEntry, "time", timePart)
	logEntry = strings.ReplaceAll(logEntry, "[level]", levelPart)
	logEntry = strings.ReplaceAll(logEntry, "func", callerPart)
	logEntry = strings.ReplaceAll(logEntry, "file", filePart)
	logEntry = strings.ReplaceAll(logEntry, "message", messagePart)

	// Добавляем перенос строки, если его нет
	if !strings.HasSuffix(logEntry, "\n") {
		logEntry += "\n"
	}

	// Запись в файл (без цвета)
	if l.fileWriter != nil {
		_, _ = l.fileWriter.Write([]byte(logEntry))
	}

	// Выбор цвета для уровня лога
	var coloredLevel string
	switch level {
	case Info:
		coloredLevel = l.infoColor.Sprintf("[%s]", level)
	case Warning:
		coloredLevel = l.warnColor.Sprintf("[%s]", level)
	case Error:
		coloredLevel = l.errorColor.Sprintf("[%s]", level)
	default:
		coloredLevel = fmt.Sprintf("[%s]", level)
	}

	// Форматируем строку для консоли, заменяя [level] на цветную версию
	consoleEntry := l.logFormat
	consoleEntry = strings.ReplaceAll(consoleEntry, "time", timePart)
	consoleEntry = strings.ReplaceAll(consoleEntry, "[level]", coloredLevel)
	consoleEntry = strings.ReplaceAll(consoleEntry, "func", callerPart)
	consoleEntry = strings.ReplaceAll(consoleEntry, "file", filePart)
	consoleEntry = strings.ReplaceAll(consoleEntry, "message", messagePart)

	// Добавляем перенос строки для консоли
	if !strings.HasSuffix(consoleEntry, "\n") {
		consoleEntry += "\n"
	}

	// Запись в консоль (с цветом для уровня)
	_, _ = l.consoleWriter.Write([]byte(consoleEntry))
}

// Close закрывает канал и ожидает завершения обработки всех сообщений
func (l *Logger) Close() {
	if l.closed {
		return
	}
	l.closed = true
	close(l.logChan) // Закрываем канал
	l.wg.Wait()      // Ожидаем завершения обработки всех сообщений
	if l.logFile != nil {
		l.logFile.Close()
	}
}

// logMessage отправляет сообщение в канал для асинхронной обработки
func (l *Logger) logMessage(level LogLevel, caller string, file string, message string) {
	if l.closed {
		return
	}
	l.logChan <- logEntry{
		level:  level,
		caller: caller,
		file:   file,
		msg:    message,
	}
}

// Info записывает сообщение уровня INFO
func (l *Logger) Info(caller, file, message string) {
	l.logMessage(Info, caller, file, message)
}

// Infof записывает форматированное сообщение уровня INFO
func (l *Logger) Infof(caller, file, format string, args ...interface{}) {
	l.logMessage(Info, caller, file, fmt.Sprintf(format, args...))
}

// Warning записывает сообщение уровня WARNING
func (l *Logger) Warning(caller, file, message string) {
	l.logMessage(Warning, caller, file, message)
}

// Warningf записывает форматированное сообщение уровня WARNING
func (l *Logger) Warningf(caller, file, format string, args ...interface{}) {
	l.logMessage(Warning, caller, file, fmt.Sprintf(format, args...))
}

// Error записывает сообщение уровня ERROR
func (l *Logger) Error(caller, file, message string) {
	l.logMessage(Error, caller, file, message)
}

// Errorf записывает форматированное сообщение уровня ERROR
func (l *Logger) Errorf(caller, file, format string, args ...interface{}) {
	l.logMessage(Error, caller, file, fmt.Sprintf(format, args...))
}
