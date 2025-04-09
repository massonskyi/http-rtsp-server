package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/utils"
	"syscall"
	"time"
)

// runServer запускает HTTP-сервер в отдельной горутине
func runServer(cfg *config.Config, logger *utils.Logger) error {
	// Создаем сервер
	srv := &http.Server{
		Addr: ":" + fmt.Sprintf("%d", cfg.ServerPort),
	}

	// Регистрируем обработчики
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("healthHandler", "main.go", "Health check endpoint called")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Server is running"))
	})

	// Запускаем сервер в горутине
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("runServer", "main.go", "Recovered from panic: %v", r)
			}
		}()
		logger.Infof("runServer", "main.go", "Starting server on port %s", fmt.Sprintf("%d", cfg.ServerPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("runServer", "main.go", "Server failed: %v", err)
		}
	}()

	// Настройка graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	logger.Info("main", "main.go", "Received shutdown signal, shutting down server...")

	// Даем серверу 5 секунд на завершение текущих запросов
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("main", "main.go", "Server shutdown failed: %v", err)
		return err
	}
	logger.Info("main", "main.go", "Server shut down gracefully")
	return nil
}

func main() {
	// Инициализация логгера
	loggerCfg := utils.DefaultLoggerConfig()
	loggerCfg.LogToFile = true
	loggerCfg.LogFilePath = "logs/server.log"
	logger, err := utils.NewLogger(loggerCfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Обработка паник в main
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("main", "main.go", "Recovered from panic: %v", r)
			os.Exit(1)
		}
	}()

	// Загрузка конфигурации
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Errorf("main", "main.go", "Failed to load config: %v", err)
		os.Exit(1)
	}
	logger.Info("main", "main.go", "Configuration loaded successfully")

	// Подключение к базе данных
	db, err := database.NewDB(cfg)
	if err != nil {
		logger.Errorf("main", "main.go", "Failed to connect to database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("main", "main.go", "Connected to database")

	// Запуск сервера
	if err := runServer(cfg, logger); err != nil {
		logger.Errorf("main", "main.go", "Failed to run server: %v", err)
		os.Exit(1)
	}
}
