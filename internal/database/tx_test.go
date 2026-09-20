package database

import (
	"context"
	"testing"
)

func TestTransferTxRollbackOnInsufficientBalance(t *testing.T) {
	dbURL := "postgresql://nikhil:Mahi1234@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia/Kolkata"
	db, err := New(dbURL)
	if err != nil {
		t.Skip("Skipping DB test: postgres instance unreachable")
	}
	defer db.Close()

	ctx := context.Background()

	// 1. Fetch initial balances for accounts 1 and 2
	var balance1Before, balance2Before int64
	err = db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&balance1Before)
	if err != nil {
		t.Skip("Account 1 not seeded; skipping live DB test")
	}
	err = db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = 2").Scan(&balance2Before)
	if err != nil {
		t.Skip("Account 2 not seeded; skipping live DB test")
	}

	// 2. Attempt a transfer exceeding available funds (causes error and triggers defer tx.Rollback())
	excessiveAmount := balance1Before + 10000000
	_, err = db.TransferTx(ctx, TransferParams{
		FromAccountID: 1,
		ToAccountID:   2,
		Amount:        excessiveAmount,
	})

	if err == nil {
		t.Fatalf("expected error due to insufficient funds, got nil")
	}

	// 3. Confirm balances were rolled back and remain unchanged
	var balance1After, balance2After int64
	_ = db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&balance1After)
	_ = db.QueryRowContext(ctx, "SELECT balance FROM accounts WHERE id = 2").Scan(&balance2After)

	if balance1After != balance1Before {
		t.Fatalf("rollback failed on sender: initial %d, after %d", balance1Before, balance1After)
	}
	if balance2After != balance2Before {
		t.Fatalf("rollback failed on receiver: initial %d, after %d", balance2Before, balance2After)
	}
}
