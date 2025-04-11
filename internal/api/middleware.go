package api

import (
	"net/http"
	"rstp-rsmt-server/internal/utils"
	"time"
)

type Middleware func(http.Handler) http.Handler

// LoggingMiddleware логирует входящие запросы
func LoggingMiddleware(logger *utils.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logger.Infof("Request", "middleware.go", "Received %s request for %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			next.ServeHTTP(w, r)
			logger.Infof("Request", "middleware.go", "Completed %s %s in %v", r.Method, r.URL.Path, time.Since(start))
		})
	}
}

// ErrorMiddleware обрабатывает ошибки и возвращает их в формате JSON
func ErrorMiddleware(logger *utils.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Errorf("Panic", "middleware.go", "Recovered from panic: %v", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
