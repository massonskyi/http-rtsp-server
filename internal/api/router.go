package api

import (
	"net/http"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/stream"
	"rstp-rsmt-server/internal/utils"
)

// Router настраивает маршруты для API
type Router struct {
	logger  *utils.Logger
	cfg     *config.Config
	handler *Handler
}

// NewRouter создает новый Router
func NewRouter(cfg *config.Config, logger *utils.Logger, streamManager *stream.StreamManager, hlsManager *stream.HLSManager) *Router {
	handler := NewHandler(logger, streamManager, hlsManager)
	return &Router{
		logger:  logger,
		cfg:     cfg,
		handler: handler,
	}
}

// SetupRoutes настраивает маршруты и возвращает http.Handler
func (r *Router) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Применяем middleware ко всем маршрутам
	logging := LoggingMiddleware(r.logger)
	errorHandling := ErrorMiddleware(r.logger)

	// Регистрируем маршруты
	mux.HandleFunc("/health", r.chainMiddleware(r.handler.HealthHandler, logging, errorHandling))
	mux.HandleFunc("/start-stream", r.chainMiddleware(r.handler.StartStreamHandler, logging, errorHandling))
	mux.HandleFunc("/stop-stream", r.chainMiddleware(r.handler.StopStreamHandler, logging, errorHandling))
	mux.HandleFunc("/list-streams", r.chainMiddleware(r.handler.ListStreamsHandler, logging, errorHandling))
	mux.HandleFunc("/stream/", r.chainMiddleware(r.handler.StreamHandler, logging, errorHandling))
	mux.HandleFunc("/videos", r.chainMiddleware(r.handler.ListVideosHandler, logging, errorHandling))
	mux.HandleFunc("/video/", r.chainMiddleware(r.handler.VideoHLSHandler, logging, errorHandling))
	mux.HandleFunc("/thumbnails/", r.chainMiddleware(r.handler.ThumbnailHandler, logging, errorHandling))

	// Удаляем старый маршрут для HLS, так как теперь используем /video/{video_id}/hls/*
	// mux.Handle("/hls/", http.StripPrefix("/hls/", http.FileServer(http.Dir(r.cfg.HLSDir))))

	return mux
}

// chainMiddleware применяет цепочку middleware к обработчику
func (r *Router) chainMiddleware(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
