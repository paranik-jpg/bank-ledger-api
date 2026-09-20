package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrInsufficientFunds = errors.New("insufficient funds in source account")
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidAmount     = errors.New("transfer amount must be positive")
	ErrSameAccount       = errors.New("cannot transfer money to the same account")
)

type TransferParams struct {
	FromAccountID int64 `json:"from_account_id"`
	ToAccountID   int64 `json:"to_account_id"`
	Amount        int64 `json:"amount"`
}

type TransferResult struct {
	Transfer    Transfer `json:"transfer"`
	FromAccount Account  `json:"from_account"`
	ToAccount   Account  `json:"to_account"`
	FromEntry   Entry    `json:"from_entry"`
	ToEntry     Entry    `json:"to_entry"`
}

func (db *DB) TransferTx(ctx context.Context, arg TransferParams) (TransferResult, error) {
	var result TransferResult

	if arg.Amount <= 0 {
		return result, ErrInvalidAmount
	}
	if arg.FromAccountID == arg.ToAccountID {
		return result, ErrSameAccount
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	// 1. Record transfer row
	transferQuery := `
		INSERT INTO transfers (from_account_id, to_account_id, amount)
		VALUES ($1, $2, $3)
		RETURNING id, from_account_id, to_account_id, amount, created_at;
	`
	err = tx.QueryRowContext(ctx, transferQuery, arg.FromAccountID, arg.ToAccountID, arg.Amount).Scan(
		&result.Transfer.ID,
		&result.Transfer.FromAccountID,
		&result.Transfer.ToAccountID,
		&result.Transfer.Amount,
		&result.Transfer.CreatedAt,
	)
	if err != nil {
		return result, fmt.Errorf("failed to create transfer: %w", err)
	}

	// 2. Record debit & credit entries
	entryQuery := `
		INSERT INTO entries (account_id, amount)
		VALUES ($1, $2)
		RETURNING id, account_id, amount, created_at;
	`
	// Debit entry (negative amount)
	err = tx.QueryRowContext(ctx, entryQuery, arg.FromAccountID, -arg.Amount).Scan(
		&result.FromEntry.ID,
		&result.FromEntry.AccountID,
		&result.FromEntry.Amount,
		&result.FromEntry.CreatedAt,
	)
	if err != nil {
		return result, fmt.Errorf("failed to record debit entry: %w", err)
	}

	// Credit entry (positive amount)
	err = tx.QueryRowContext(ctx, entryQuery, arg.ToAccountID, arg.Amount).Scan(
		&result.ToEntry.ID,
		&result.ToEntry.AccountID,
		&result.ToEntry.Amount,
		&result.ToEntry.CreatedAt,
	)
	if err != nil {
		return result, fmt.Errorf("failed to record credit entry: %w", err)
	}

	// 3. Deterministic order locking & balance update to avoid deadlocks
	if arg.FromAccountID < arg.ToAccountID {
		result.FromAccount, result.ToAccount, err = updateBalances(ctx, tx, arg.FromAccountID, -arg.Amount, arg.ToAccountID, arg.Amount)
	} else {
		result.ToAccount, result.FromAccount, err = updateBalances(ctx, tx, arg.ToAccountID, arg.Amount, arg.FromAccountID, -arg.Amount)
	}
	if err != nil {
		return result, err
	}

	return result, tx.Commit()
}

func updateBalances(
	ctx context.Context,
	tx *sql.Tx,
	acc1ID, amount1 int64,
	acc2ID, amount2 int64,
) (acc1 Account, acc2 Account, err error) {
	// Update and lock first account
	query := `
		UPDATE accounts
		SET balance = balance + $1
		WHERE id = $2
		RETURNING id, owner, balance, currency, created_at;
	`
	err = tx.QueryRowContext(ctx, query, amount1, acc1ID).Scan(
		&acc1.ID, &acc1.Owner, &acc1.Balance, &acc1.Currency, &acc1.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return acc1, acc2, ErrAccountNotFound
		}
		return acc1, acc2, err
	}
	if acc1.Balance < 0 {
		return acc1, acc2, ErrInsufficientFunds
	}

	// Update and lock second account
	err = tx.QueryRowContext(ctx, query, amount2, acc2ID).Scan(
		&acc2.ID, &acc2.Owner, &acc2.Balance, &acc2.Currency, &acc2.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return acc1, acc2, ErrAccountNotFound
		}
		return acc1, acc2, err
	}
	if acc2.Balance < 0 {
		return acc1, acc2, ErrInsufficientFunds
	}

	return acc1, acc2, nil
}
