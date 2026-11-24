package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"
)

type ctxKey string

const reqIDKey ctxKey = "request_id"

var reqCounter atomic.Uint64

func requestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			id := fmt.Sprintf("request-%d", reqCounter.Add(1))
			ctx := context.WithValue(r.Context(), reqIDKey, id)
			w.Header().Set("X-Request-ID", id)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r.WithContext(ctx))

			logger.Info("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", id),
			)
		})
	}
}

func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					id, _ := r.Context().Value(reqIDKey).(string)
					logger.Error("panic", slog.Any("err", err), slog.String("request_id", id), slog.String("stack", string(debug.Stack())))
					writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error", Code: "INTERNAL_ERROR"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
