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
	cors := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}

	// Регистрируем маршруты
	mux.HandleFunc("/health", r.chainMiddleware(r.handler.HealthHandler, logging, errorHandling, cors))
	mux.HandleFunc("/start-stream", r.chainMiddleware(r.handler.StartStreamHandler, logging, errorHandling, cors))
	mux.HandleFunc("/stop-stream", r.chainMiddleware(r.handler.StopStreamHandler, logging, errorHandling, cors))
	mux.HandleFunc("/list-streams", r.chainMiddleware(r.handler.ListStreamsHandler, logging, errorHandling, cors))
	mux.HandleFunc("/stream/", r.chainMiddleware(r.handler.StreamHandler, logging, errorHandling, cors))
	mux.HandleFunc("/archive/list", r.chainMiddleware(r.handler.ListArchivedStreamsHandler, logging, errorHandling, cors))
	mux.HandleFunc("/archive/", r.chainMiddleware(r.handler.ArchiveHandler, logging, errorHandling, cors))

	return mux
}

// chainMiddleware применяет цепочку middleware к обработчику
func (r *Router) chainMiddleware(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
