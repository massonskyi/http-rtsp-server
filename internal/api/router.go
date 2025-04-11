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

	// Middleware
	logging := LoggingMiddleware(r.logger)
	errorHandling := ErrorMiddleware(r.logger)
	cors := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Оборачиваем в chain
	chain := func(h http.HandlerFunc) http.Handler {
		return r.chainMiddleware(h, logging, errorHandling, cors)
	}

	// Маршруты
	router.Handle("/health", chain(r.handler.HealthHandler)).Methods("GET")
	router.Handle("/start-stream", chain(r.handler.StartStreamHandler)).Methods("POST")
	router.Handle("/stop-stream", chain(r.handler.StopStreamHandler)).Methods("POST")
	router.Handle("/list-streams", chain(r.handler.ListStreamsHandler)).Methods("GET")
	router.Handle("/stream/{stream_name}", chain(r.handler.StreamHandler)).Methods("GET", "OPTIONS")
	router.Handle("/stream/{stream_name}/{segment}", chain(r.handler.StreamHandler)).Methods("GET", "OPTIONS")
	router.Handle("/archive/list", chain(r.handler.ListArchivedStreamsHandler)).Methods("GET")
	router.Handle("/archive/{stream_name}", chain(r.handler.ArchiveHandler)).Methods("GET", "OPTIONS")
	router.Handle("/archive/{stream_name}/{segment}", chain(r.handler.ArchiveHandler)).Methods("GET", "OPTIONS")
	router.Handle("/preview/{stream_name}", chain(r.handler.PreviewHandler)).Methods("GET", "OPTIONS")
	router.Handle("/update-config", chain(r.handler.UpdateConfigHandler)).Methods("POST")
	router.Handle("/get-config", chain(r.handler.GetConfigHandler)).Methods("GET")
	return router
}

// chainMiddleware применяет цепочку middleware к обработчику
func (r *Router) chainMiddleware(handler http.HandlerFunc, middlewares ...Middleware) http.Handler {
	var h http.Handler = handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
