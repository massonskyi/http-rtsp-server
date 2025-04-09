package api

import (
	"net/http"
	"rstp-rsmt-server/internal/utils"
	"time"
)

// Middleware представляет собой функцию промежуточного слоя
type Middleware func(http.HandlerFunc) http.HandlerFunc

// LoggingMiddleware логирует входящие запросы
func LoggingMiddleware(logger *utils.Logger) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logger.Infof("Request", "middleware.go", "Received %s request for %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			next(w, r)
			logger.Infof("Request", "middleware.go", "Completed %s %s in %v", r.Method, r.URL.Path, time.Since(start))
		}
	}
}

// ErrorMiddleware обрабатывает ошибки и возвращает их в формате JSON
func ErrorMiddleware(logger *utils.Logger) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Errorf("Panic", "middleware.go", "Recovered from panic: %v", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next(w, r)
		}
	}
}
