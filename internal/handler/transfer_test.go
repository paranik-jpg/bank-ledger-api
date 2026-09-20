package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
	"github.com/paranik-jpg/bank-ledger-api/internal/middleware"
	"github.com/paranik-jpg/bank-ledger-api/internal/service"
)

// MockTransferService satisfies service.TransferServiceInterface
type MockTransferService struct {
	ExecuteTransferFn func(ctx context.Context, req service.TransferDTO) (database.TransferResult, error)
}

func (m *MockTransferService) ExecuteTransfer(ctx context.Context, req service.TransferDTO) (database.TransferResult, error) {
	return m.ExecuteTransferFn(ctx, req)
}

func TestTransferHandler(t *testing.T) {
	t.Run("Unauthorized - Missing Context Identity", func(t *testing.T) {
		mockSvc := &MockTransferService{}
		h := NewTransferHandler(mockSvc)

		body := bytes.NewBufferString(`{"from_account_id":1,"to_account_id":2,"amount":50}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", body)
		rec := httptest.NewRecorder()

		h.CreateTransfer(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})

	t.Run("Bad Request - Invalid Payload Numbers", func(t *testing.T) {
		mockSvc := &MockTransferService{}
		h := NewTransferHandler(mockSvc)

		body := bytes.NewBufferString(`{"from_account_id":-1,"to_account_id":2,"amount":0}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", body)
		ctx := context.WithValue(req.Context(), middleware.UserContextKey, "testuser")
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.CreateTransfer(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})

	t.Run("Bad Request - Insufficient Balance Mapping", func(t *testing.T) {
		mockSvc := &MockTransferService{
			ExecuteTransferFn: func(ctx context.Context, req service.TransferDTO) (database.TransferResult, error) {
				return database.TransferResult{}, service.ErrInsufficientBalance
			},
		}
		h := NewTransferHandler(mockSvc)

		body := bytes.NewBufferString(`{"from_account_id":1,"to_account_id":2,"amount":99999}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", body)
		ctx := context.WithValue(req.Context(), middleware.UserContextKey, "testuser")
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.CreateTransfer(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}
