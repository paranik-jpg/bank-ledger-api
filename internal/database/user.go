package database

import (
	"context"
	"time"
)

type User struct {
	Username       string    `json:"username"`
	HashedPassword string    `json:"-"` // Omit hash from JSON serialization
	Email          string    `json:"email"`
	CreatedAt      time.Time `json:"created_at"`
}

func (db *DB) CreateUser(ctx context.Context, username, hashedPassword, email string) (*User, error) {
	query := `
		INSERT INTO users (username, hashed_password, email)
		VALUES ($1, $2, $3)
		RETURNING username, hashed_password, email, created_at;
	`

	var user User
	err := db.QueryRowContext(ctx, query, username, hashedPassword, email).Scan(
		&user.Username,
		&user.HashedPassword,
		&user.Email,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (db *DB) GetUser(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT username, hashed_password, email, created_at
		FROM users
		WHERE username = $1;
	`

	var user User
	err := db.QueryRowContext(ctx, query, username).Scan(
		&user.Username,
		&user.HashedPassword,
		&user.Email,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
