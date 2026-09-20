package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

type LoggerMiddleware struct {
	logger *slog.Logger
}

func NewLoggerMiddleware() *LoggerMiddleware {
	// JSON handler writing to stdout
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &LoggerMiddleware{
		logger: slog.New(jsonHandler),
	}
}

func (m *LoggerMiddleware) LogRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		m.logger.Info("HTTP request processed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_ip", r.RemoteAddr),
			slog.Int("status", rw.statusCode),
			slog.Duration("duration", duration),
		)
	})
}
