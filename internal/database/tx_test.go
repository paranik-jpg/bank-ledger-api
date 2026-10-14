package database

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestTransferTxConcurrentOpposingTransfers(t *testing.T) {
	dbURL := "postgresql://nikhil:Mahi1234@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia/Kolkata"

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

	runTransfers := func(fromID, toID int64) {
		defer workers.Done()
		ready.Done()

		// Release both workers together to increase lock contention.
		<-start

		for i := 0; i < transfersPerDirection; i++ {
			_, err := db.TransferTx(ctx, TransferParams{
				FromAccountID: fromID,
				ToAccountID:   toID,
				Amount:        transferAmount,
			})
			outcomes <- transferOutcome{err: err}
			if err != nil {
				return
			}
		}
	}

	go runTransfers(accountA.ID, accountB.ID)
	go runTransfers(accountB.ID, accountA.ID)

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
