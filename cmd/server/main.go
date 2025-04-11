package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"rstp-rsmt-server/internal/api"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/database"
	"rstp-rsmt-server/internal/protocol"
	"rstp-rsmt-server/internal/storage"
	"rstp-rsmt-server/internal/stream"
	"rstp-rsmt-server/internal/utils"
	"syscall"
	"time"
)

// runServer запускает HTTP-сервер в отдельной горутине
func runServer(cfg *config.Config, logger *utils.Logger, storage *storage.Storage) error {
	// Инициализируем RTSP-клиент
	rtspClient := protocol.NewRTSPClient(cfg, logger, storage, nil)

	// Инициализируем StreamManager
	streamManager := stream.NewStreamManager(cfg, logger, storage, rtspClient)
	defer streamManager.Shutdown()

	// Инициализируем HLSManager
	hlsManager := stream.NewHLSManager(cfg, logger)

	// Инициализируем маршрутизацию
	router := api.NewRouter(cfg, logger, streamManager, hlsManager)

	// Создаем сервер
	srv := &http.Server{
		Addr:    ":" + fmt.Sprintf("%d", cfg.GetServerPort()),
		Handler: router.SetupRoutes(),
	}

	// Запускаем сервер в горутине
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("runServer", "main.go", fmt.Sprintf("Recovered from panic: %v", r))
			}
		}()
		logger.Info("runServer", "main.go", fmt.Sprintf("Starting server on port %d", cfg.GetServerPort()))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("runServer", "main.go", fmt.Sprintf("Server failed: %v", err))
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
		logger.Error("main", "main.go", fmt.Sprintf("Server shutdown failed: %v", err))
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
		logger.Errorf("main", "main.go", "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Обработка паник в main
	defer func() {
		if r := recover(); r != nil {
			logger.Error("main", "main.go", fmt.Sprintf("Recovered from panic: %v", r))
			os.Exit(1)
		}
	}()

	// Загрузка конфигурации
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("main", "main.go", fmt.Sprintf("Failed to load config: %v", err))
		os.Exit(1)
	}
	logger.Info("main", "main.go", "Configuration loaded successfully")

	// Подключение к базе данных
	db, err := database.NewDB(cfg)
	if err != nil {
		logger.Error("main", "main.go", fmt.Sprintf("Failed to connect to database: %v", err))
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("main", "main.go", "Connected to database")

	// Инициализация хранилища
	store := storage.NewStorage(db.Pool, logger)

	// Запуск сервера
	if err := runServer(cfg, logger, store); err != nil {
		logger.Error("main", "main.go", fmt.Sprintf("Failed to run server: %v", err))
		os.Exit(1)
	}
}
