package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
)

var (
	ErrInvalidTransferAmount = errors.New("amount must be positive")
	ErrIdenticalAccounts     = errors.New("cannot transfer to the same account")
	ErrInsufficientBalance   = errors.New("insufficient balance")
	ErrAccountDoesNotExist   = errors.New("account does not exist")
)

type TransferDTO struct {
	FromAccountID int64
	ToAccountID   int64
	Amount        int64
	InitiatedBy   string
}

// TransferServiceInterface defines atomic money transfer operations
type TransferServiceInterface interface {
	ExecuteTransfer(ctx context.Context, req TransferDTO) (database.TransferResult, error)
}

type TransferService struct {
	db *database.DB
}

func NewTransferService(db *database.DB) TransferServiceInterface {
	return &TransferService{db: db}
}

func (s *TransferService) ExecuteTransfer(ctx context.Context, req TransferDTO) (database.TransferResult, error) {
	// Centralized Transfer Validation & Business Rules
	if req.Amount <= 0 {
		return database.TransferResult{}, ErrInvalidTransferAmount
	}
	if req.FromAccountID == req.ToAccountID {
		return database.TransferResult{}, ErrIdenticalAccounts
	}

	result, err := s.db.TransferTx(ctx, database.TransferParams{
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
	})
	if err != nil {
		if errors.Is(err, database.ErrInsufficientFunds) {
			return database.TransferResult{}, ErrInsufficientBalance
		}
		if errors.Is(err, database.ErrAccountNotFound) {
			return database.TransferResult{}, ErrAccountDoesNotExist
		}
		return database.TransferResult{}, fmt.Errorf("transfer failed: %w", err)
	}

	return result, nil
}
