package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping DB test: DATABASE_URL is not set")
	}

	return dbURL
}

func TestTransferTxRollbackOnInsufficientBalance(t *testing.T) {
	db, err := New(testDatabaseURL(t))
	if err != nil {
		t.Skipf("Skipping DB test: postgres instance unreachable: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	ownerA := fmt.Sprintf("rollback_a_%d", suffix)
	ownerB := fmt.Sprintf("rollback_b_%d", suffix)

	if _, err := db.CreateUser(ctx, ownerA, "test-hash", ownerA+"@example.com"); err != nil {
		t.Fatalf("create first test user: %v", err)
	}
	if _, err := db.CreateUser(ctx, ownerB, "test-hash", ownerB+"@example.com"); err != nil {
		t.Fatalf("create second test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE username IN ($1, $2)",
			ownerA,
			ownerB,
		); err != nil {
			t.Errorf("clean up test users: %v", err)
		}
	})

	const initialBalance int64 = 1_000

	accountA, err := db.CreateAccount(ctx, ownerA, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create first test account: %v", err)
	}
	accountB, err := db.CreateAccount(ctx, ownerB, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create second test account: %v", err)
	}

	// The transfer must fail because the requested amount exceeds the source balance.
	_, err = db.TransferTx(ctx, TransferParams{
		FromAccountID: accountA.ID,
		ToAccountID:   accountB.ID,
		Amount:        initialBalance + 1,
		InitiatedBy:   ownerA,
	})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	gotA, err := db.GetAccount(ctx, accountA.ID)
	if err != nil {
		t.Fatalf("read source account: %v", err)
	}
	gotB, err := db.GetAccount(ctx, accountB.ID)
	if err != nil {
		t.Fatalf("read destination account: %v", err)
	}

	if gotA.Balance != initialBalance {
		t.Errorf("rollback failed on sender: got %d, want %d", gotA.Balance, initialBalance)
	}
	if gotB.Balance != initialBalance {
		t.Errorf("rollback failed on receiver: got %d, want %d", gotB.Balance, initialBalance)
	}
}

func TestTransferTxConcurrentOpposingTransfers(t *testing.T) {
	dbURL := testDatabaseURL(t)

	db, err := New(dbURL)
	if err != nil {
		t.Skipf("Skipping DB test: postgres instance unreachable: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use unique usernames so this test does not collide with existing data.
	suffix := time.Now().UnixNano()
	ownerA := fmt.Sprintf("concurrency_a_%d", suffix)
	ownerB := fmt.Sprintf("concurrency_b_%d", suffix)

	if _, err := db.CreateUser(ctx, ownerA, "test-hash", ownerA+"@example.com"); err != nil {
		t.Fatalf("create first test user: %v", err)
	}
	if _, err := db.CreateUser(ctx, ownerB, "test-hash", ownerB+"@example.com"); err != nil {
		t.Fatalf("create second test user: %v", err)
	}

	// Delete the users at the end; ON DELETE CASCADE removes their accounts.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE username IN ($1, $2)",
			ownerA,
			ownerB,
		)
		if err != nil {
			t.Errorf("clean up test users: %v", err)
		}
	})

	const initialBalance int64 = 1_000_000
	const transferAmount int64 = 1

	accountA, err := db.CreateAccount(ctx, ownerA, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create first test account: %v", err)
	}
	accountB, err := db.CreateAccount(ctx, ownerB, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create second test account: %v", err)
	}

	const transfersPerDirection = 50

	type transferOutcome struct {
		err error
	}

	outcomes := make(chan transferOutcome, 2*transfersPerDirection)

	var ready sync.WaitGroup
	ready.Add(2)

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)

	runTransfers := func(fromID, toID int64, initiatedBy string) {
		defer workers.Done()
		ready.Done()

		// Release both workers together to increase lock contention.
		<-start

		for i := 0; i < transfersPerDirection; i++ {
			_, err := db.TransferTx(ctx, TransferParams{
				FromAccountID: fromID,
				ToAccountID:   toID,
				Amount:        transferAmount,
				InitiatedBy:   initiatedBy,
			})
			outcomes <- transferOutcome{err: err}
			if err != nil {
				return
			}
		}
	}

	go runTransfers(accountA.ID, accountB.ID, ownerA)
	go runTransfers(accountB.ID, accountA.ID, ownerB)

	// Don't start until both workers are ready.
	ready.Wait()
	// Release both workers from the starting gate.
	close(start)

	workersDone := make(chan struct{})
	go func() {
		// Don't continue until both workers finish.
		workers.Wait()
		close(workersDone)
	}()

	select {
	case <-workersDone:
		// Both workers completed before the shared deadline.
	case <-ctx.Done():
		// Cancel in-flight database operations and wait for both workers to exit.
		cancel()
		<-workersDone
		t.Fatal("opposing transfers did not finish before the deadline; possible deadlock")
	}

	close(outcomes)

	for outcome := range outcomes {
		if outcome.err != nil {
			t.Errorf("concurrent transfer failed: %v", outcome.err)
		}
	}

	var balanceA, balanceB int64
	if err := db.QueryRowContext(
		ctx, "SELECT balance FROM accounts WHERE id = $1", accountA.ID,
	).Scan(&balanceA); err != nil {
		t.Fatalf("read first account balance: %v", err)
	}
	if err := db.QueryRowContext(
		ctx, "SELECT balance FROM accounts WHERE id = $1", accountB.ID,
	).Scan(&balanceB); err != nil {
		t.Fatalf("read second account balance: %v", err)
	}

	if balanceA+balanceB != 2*initialBalance {
		t.Errorf(
			"total balance changed: got %d, want %d",
			balanceA+balanceB,
			2*initialBalance,
		)
	}

	if balanceA != initialBalance || balanceB != initialBalance {
		t.Errorf(
			"expected balances to return to their initial values (%d, %d); got (%d, %d)",
			initialBalance, initialBalance, balanceA, balanceB,
		)
	}
}

func TestTransferTxRejectsUnauthorizedInitiator(t *testing.T) {
	dbURL := testDatabaseURL(t)

	db, err := New(dbURL)
	if err != nil {
		t.Skipf("Skipping DB test: postgres instance unreachable: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	ownerA := fmt.Sprintf("unauthorized_a_%d", suffix)
	ownerB := fmt.Sprintf("unauthorized_b_%d", suffix)

	if _, err := db.CreateUser(ctx, ownerA, "test-hash", ownerA+"@example.com"); err != nil {
		t.Fatalf("create first test user: %v", err)
	}
	if _, err := db.CreateUser(ctx, ownerB, "test-hash", ownerB+"@example.com"); err != nil {
		t.Fatalf("create second test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE username IN ($1, $2)",
			ownerA,
			ownerB,
		); err != nil {
			t.Errorf("clean up test users: %v", err)
		}
	})

	const initialBalance int64 = 1_000_000

	accountA, err := db.CreateAccount(ctx, ownerA, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create first test account: %v", err)
	}
	accountB, err := db.CreateAccount(ctx, ownerB, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create second test account: %v", err)
	}

	_, err = db.TransferTx(ctx, TransferParams{
		FromAccountID: accountA.ID,
		ToAccountID:   accountB.ID,
		Amount:        100,
		InitiatedBy:   ownerB, // Not the owner of the source account.
	})

	if !errors.Is(err, ErrUnauthorizedAccount) {
		t.Fatalf("expected ErrUnauthorizedAccount, got %v", err)
	}

	gotA, err := db.GetAccount(ctx, accountA.ID)
	if err != nil {
		t.Fatalf("read first account: %v", err)
	}
	gotB, err := db.GetAccount(ctx, accountB.ID)
	if err != nil {
		t.Fatalf("read second account: %v", err)
	}

	if gotA.Balance != initialBalance {
		t.Errorf("unauthorized transfer changed source balance: got %d, want %d",
			gotA.Balance, initialBalance)
	}
	if gotB.Balance != initialBalance {
		t.Errorf("unauthorized transfer changed destination balance: got %d, want %d",
			gotB.Balance, initialBalance)
	}
}

func TestTransferTxRejectsCurrencyMismatch(t *testing.T) {
	dbURL := testDatabaseURL(t)

	db, err := New(dbURL)
	if err != nil {
		t.Skipf("Skipping DB test: postgres instance unreachable: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	ownerA := fmt.Sprintf("currency_a_%d", suffix)
	ownerB := fmt.Sprintf("currency_b_%d", suffix)

	if _, err := db.CreateUser(ctx, ownerA, "test-hash", ownerA+"@example.com"); err != nil {
		t.Fatalf("create first test user: %v", err)
	}
	if _, err := db.CreateUser(ctx, ownerB, "test-hash", ownerB+"@example.com"); err != nil {
		t.Fatalf("create second test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			"DELETE FROM users WHERE username IN ($1, $2)",
			ownerA,
			ownerB,
		); err != nil {
			t.Errorf("clean up test users: %v", err)
		}
	})

	const initialBalance int64 = 1_000_000

	fromAccount, err := db.CreateAccount(ctx, ownerA, initialBalance, "INR")
	if err != nil {
		t.Fatalf("create source account: %v", err)
	}
	toAccount, err := db.CreateAccount(ctx, ownerB, initialBalance, "USD")
	if err != nil {
		t.Fatalf("create destination account: %v", err)
	}

	_, err = db.TransferTx(ctx, TransferParams{
		FromAccountID: fromAccount.ID,
		ToAccountID:   toAccount.ID,
		Amount:        100,
		InitiatedBy:   ownerA,
	})

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}

	gotFrom, err := db.GetAccount(ctx, fromAccount.ID)
	if err != nil {
		t.Fatalf("read source account: %v", err)
	}
	gotTo, err := db.GetAccount(ctx, toAccount.ID)
	if err != nil {
		t.Fatalf("read destination account: %v", err)
	}

	if gotFrom.Balance != initialBalance {
		t.Errorf("source balance changed: got %d, want %d",
			gotFrom.Balance, initialBalance)
	}
	if gotTo.Balance != initialBalance {
		t.Errorf("destination balance changed: got %d, want %d",
			gotTo.Balance, initialBalance)
	}
}

func TestMapBalanceUpdateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "Postgres check violation maps to insufficient funds",
			err: &pgconn.PgError{
				Code: "23514",
			},
			want: ErrInsufficientFunds,
		},
		{
			name: "missing account maps to account not found",
			err:  sql.ErrNoRows,
			want: ErrAccountNotFound,
		},
		{
			name: "unrelated error is preserved",
			err:  errors.New("connection lost"),
			want: errors.New("connection lost"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapBalanceUpdateError(tt.err)

			if tt.name == "unrelated error is preserved" {
				if got == nil || got.Error() != tt.want.Error() {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				return
			}

			if !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want error %v", got, tt.want)
			}
		})
	}
}
