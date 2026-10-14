package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInsufficientFunds   = errors.New("insufficient funds in source account")
	ErrAccountNotFound     = errors.New("account not found")
	ErrInvalidAmount       = errors.New("transfer amount must be positive")
	ErrSameAccount         = errors.New("cannot transfer money to the same account")
	ErrUnauthorizedAccount = errors.New("unauthorized access to source account")
	ErrCurrencyMismatch    = errors.New("source and destination account currencies do not match")
)

type TransferParams struct {
	FromAccountID int64  `json:"from_account_id"`
	ToAccountID   int64  `json:"to_account_id"`
	Amount        int64  `json:"amount"`
	InitiatedBy   string `json:"initiated_by"`
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

	// Lock both accounts before any transfer or balance mutations.
	fromAccount, toAccount, err := lockAccounts(ctx, tx, arg.FromAccountID, arg.ToAccountID)
	if err != nil {
		return result, err
	}

	// Ensure the initiator owns the source account.
	if fromAccount.Owner != arg.InitiatedBy {
		return result, ErrUnauthorizedAccount
	}

	if fromAccount.Currency != toAccount.Currency {
		return result, ErrCurrencyMismatch
	}

	if fromAccount.Balance < arg.Amount {
		return result, ErrInsufficientFunds
	}

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

func mapBalanceUpdateError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ErrInsufficientFunds
	}

	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountNotFound
	}

	return err
}

func updateBalances(ctx context.Context, tx *sql.Tx, acc1ID, amount1 int64, acc2ID, amount2 int64) (acc1 Account, acc2 Account, err error) {
	// Update the first account's balance.
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
		return acc1, acc2, mapBalanceUpdateError(err)
	}
	if acc1.Balance < 0 {
		return acc1, acc2, ErrInsufficientFunds
	}

	// Update the second account's balance.
	err = tx.QueryRowContext(ctx, query, amount2, acc2ID).Scan(
		&acc2.ID, &acc2.Owner, &acc2.Balance, &acc2.Currency, &acc2.CreatedAt,
	)
	if err != nil {
		return acc1, acc2, mapBalanceUpdateError(err)
	}
	if acc2.Balance < 0 {
		return acc1, acc2, ErrInsufficientFunds
	}

	return acc1, acc2, nil
}

func lockAccounts(ctx context.Context, tx *sql.Tx, fromAccountID, toAccountID int64) (fromAccount Account, toAccount Account, err error) {
	firstID, secondID := fromAccountID, toAccountID

	// Always acquire row locks in ascending account-ID order.
	if firstID > secondID {
		firstID, secondID = secondID, firstID
	}

	query := `
        SELECT id, owner, balance, currency, created_at
        FROM accounts
        WHERE id = $1
        FOR NO KEY UPDATE;
    `

	lockOne := func(id int64) (Account, error) {
		var account Account

		err := tx.QueryRowContext(ctx, query, id).Scan(
			&account.ID,
			&account.Owner,
			&account.Balance,
			&account.Currency,
			&account.CreatedAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return account, ErrAccountNotFound
		}
		if err != nil {
			return account, err
		}

		return account, nil
	}

	first, err := lockOne(firstID)
	if err != nil {
		return fromAccount, toAccount, err
	}

	second, err := lockOne(secondID)
	if err != nil {
		return fromAccount, toAccount, err
	}

	// Return the locked accounts in the caller's original order.
	if fromAccountID == firstID {
		return first, second, nil
	}

	return second, first, nil
}
