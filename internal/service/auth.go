package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
)

type LoginResult struct {
	AccessToken string
	ExpiresAt   time.Time
}

// AuthServiceInterface defines credential verification and token issuance
type AuthServiceInterface interface {
	Login(ctx context.Context, username, password string) (*LoginResult, error)
}

type AuthService struct {
	db            *database.DB
	secretKey     []byte
	tokenDuration time.Duration
}

type TokenClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

func NewAuthService(db *database.DB, secretKey string, duration time.Duration) AuthServiceInterface {
	return &AuthService{
		db:            db,
		secretKey:     []byte(secretKey),
		tokenDuration: duration,
	}
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	user, err := s.db.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	// Centralized Credential Verification
	err = bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(password))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Centralized JWT Generation
	expiresAt := database.NowIST().Add(s.tokenDuration)
	claims := TokenClaims{
		UserID: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.Username,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(database.NowIST()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	return &LoginResult{
		AccessToken: tokenString,
		ExpiresAt:   expiresAt,
	}, nil
}
