package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/paranik-jpg/bank-ledger-api/internal/database"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserAlreadyExists = errors.New("username or email already exists")
	ErrInvalidUserData   = errors.New("invalid username, email, or password")
)

type RegisterUserInput struct {
	Username string
	Email    string
	Password string
}

// UserServiceInterface defines the contract for user domain operations
type UserServiceInterface interface {
	Register(ctx context.Context, input RegisterUserInput) (*database.User, error)
}

type UserService struct {
	db *database.DB
}

func NewUserService(db *database.DB) UserServiceInterface {
	return &UserService{db: db}
}

func (s *UserService) Register(ctx context.Context, input RegisterUserInput) (*database.User, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))

	if input.Username == "" || input.Email == "" || len(input.Password) < 6 {
		return nil, ErrInvalidUserData
	}

	// Centralized Password Hashing
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user, err := s.db.CreateUser(ctx, input.Username, string(hashedPassword), input.Email)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}
