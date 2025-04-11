package api

import (
	"net/http"
	"rstp-rsmt-server/internal/config"
	"rstp-rsmt-server/internal/stream"
	"rstp-rsmt-server/internal/utils"

	"github.com/gorilla/mux"
)

// Router настраивает маршруты для API
type Router struct {
	logger  *utils.Logger
	cfg     *config.Config
	handler *Handler
}

// NewRouter создает новый Router
func NewRouter(cfg *config.Config, logger *utils.Logger, streamManager *stream.StreamManager, hlsManager *stream.HLSManager) *Router {
	handler := NewHandler(logger, cfg, streamManager, hlsManager)
	return &Router{
		logger:  logger,
		cfg:     cfg,
		handler: handler,
	}
}

// SetupRoutes настраивает маршруты и возвращает http.Handler
func (r *Router) SetupRoutes() http.Handler {
	router := mux.NewRouter()

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
	router.HandleFunc("/health", r.chainMiddleware(r.handler.HealthHandler, logging, errorHandling, cors)).Methods("GET")
	router.HandleFunc("/start-stream", r.chainMiddleware(r.handler.StartStreamHandler, logging, errorHandling, cors)).Methods("POST")
	router.HandleFunc("/stop-stream", r.chainMiddleware(r.handler.StopStreamHandler, logging, errorHandling, cors)).Methods("POST")
	router.HandleFunc("/list-streams", r.chainMiddleware(r.handler.ListStreamsHandler, logging, errorHandling, cors)).Methods("GET")
	router.HandleFunc("/stream/{stream_name}", r.chainMiddleware(r.handler.StreamHandler, logging, errorHandling, cors)).Methods("GET", "OPTIONS")
	router.HandleFunc("/stream/{stream_name}/{segment}", r.chainMiddleware(r.handler.StreamHandler, logging, errorHandling, cors)).Methods("GET", "OPTIONS")
	router.HandleFunc("/archive/list", r.chainMiddleware(r.handler.ListArchivedStreamsHandler, logging, errorHandling, cors)).Methods("GET")
	router.HandleFunc("/archive/{stream_name}", r.chainMiddleware(r.handler.ArchiveHandler, logging, errorHandling, cors)).Methods("GET", "OPTIONS")
	router.HandleFunc("/archive/{stream_name}/{segment}", r.chainMiddleware(r.handler.ArchiveHandler, logging, errorHandling, cors)).Methods("GET", "OPTIONS")
	router.HandleFunc("/preview/{stream_name}", r.chainMiddleware(r.handler.PreviewHandler, logging, errorHandling, cors)).Methods("GET", "OPTIONS")
	router.HandleFunc("/update-config", r.chainMiddleware(r.handler.UpdateConfigHandler, logging, errorHandling, cors)).Methods("POST")

	return router
}

// chainMiddleware применяет цепочку middleware к обработчику
func (r *Router) chainMiddleware(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
