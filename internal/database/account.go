package database

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("account not found")
)

func (db *DB) CreateAccount(ctx context.Context, owner string, balance int64, currency string) (*Account, error) {
	query := `
		INSERT INTO accounts (owner, balance, currency)
		VALUES ($1, $2, $3)
		RETURNING id, owner, balance, currency, created_at;
	`

	var acc Account
	err := db.QueryRowContext(ctx, query, owner, balance, currency).Scan(
		&acc.ID,
		&acc.Owner,
		&acc.Balance,
		&acc.Currency,
		&acc.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &acc, nil
}

func (db *DB) GetAccount(ctx context.Context, id int64) (*Account, error) {
	query := `
		SELECT id, owner, balance, currency, created_at
		FROM accounts
		WHERE id = $1;
	`

	var acc Account
	err := db.QueryRowContext(ctx, query, id).Scan(
		&acc.ID,
		&acc.Owner,
		&acc.Balance,
		&acc.Currency,
		&acc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRecordNotFound // custom
		}
		return nil, err
	}

	return &acc, nil
}

func (db *DB) ListAccounts(ctx context.Context, limit, offset int32) ([]Account, error) {
	query := `
		SELECT id, owner, balance, currency, created_at
		FROM accounts
		ORDER BY id
		LIMIT $1 OFFSET $2;
	`

	rows, err := db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var acc Account
		if err := rows.Scan(&acc.ID, &acc.Owner, &acc.Balance, &acc.Currency, &acc.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}
