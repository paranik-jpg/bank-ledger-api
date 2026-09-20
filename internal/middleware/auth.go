package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/paranik-jpg/bank-ledger-api/internal/service"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user_id"

type AuthMiddleware struct {
	secretKey []byte
}

func NewAuthMiddleware(secretKey string) *AuthMiddleware {
	return &AuthMiddleware{
		secretKey: []byte(secretKey),
	}
}

func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "Authorization header is required")
			return
		}

		fields := strings.Fields(authHeader)
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			writeError(w, http.StatusUnauthorized, "Invalid authorization header format (must be Bearer <token>)")
			return
		}

		tokenString := fields[1]

		claims := &service.TokenClaims{}
		token, err := jwt.ParseWithClaims(
			tokenString,
			claims,
			func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return m.secretKey, nil
			},
		)

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				writeError(w, http.StatusUnauthorized, "Token has expired")
				return
			}
			writeError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		if !token.Valid {
			writeError(w, http.StatusUnauthorized, "Token is not valid")
			return
		}

		// Inject user identity into request context
		ctx := context.WithValue(r.Context(), UserContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUserID retrieves the authenticated username/id from request context
func GetUserID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(UserContextKey).(string)
	return val, ok
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
