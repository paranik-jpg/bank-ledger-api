package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

type RecoveryMiddleware struct {
	logger *slog.Logger
}

func NewRecoveryMiddleware() *RecoveryMiddleware {
	return &RecoveryMiddleware{
		logger: slog.Default(),
	}
}

func (m *RecoveryMiddleware) RecoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stackTrace := string(debug.Stack())
				m.logger.Error("Panic recovered",
					slog.Any("error", rec),
					slog.String("path", r.URL.Path),
					slog.String("stack_trace", stackTrace),
				)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"Internal server error"}`))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
