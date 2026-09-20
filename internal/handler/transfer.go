package handler

import (
	"errors"
	"net/http"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
	"github.com/paranik-jpg/bank-ledger-api/internal/middleware"
	"github.com/paranik-jpg/bank-ledger-api/internal/service"
)

type TransferHandler struct {
	transferService service.TransferServiceInterface
}

func NewTransferHandler(transferService service.TransferServiceInterface) *TransferHandler {
	return &TransferHandler{transferService: transferService}
}

type CreateTransferRequest struct {
	FromAccountID int64 `json:"from_account_id"`
	ToAccountID   int64 `json:"to_account_id"`
	Amount        int64 `json:"amount"`
}

type TransferResponse struct {
	TransferID    int64  `json:"transfer_id"`
	FromAccountID int64  `json:"from_account_id"`
	ToAccountID   int64  `json:"to_account_id"`
	Amount        int64  `json:"amount"`
	FromBalance   int64  `json:"from_balance"`
	ToBalance     int64  `json:"to_balance"`
	CreatedAt     string `json:"created_at"`
}

func (h *TransferHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	// 1. Method verification
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 2. Auth context verification
	caller, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// 3. Request decoding
	var req CreateTransferRequest
	if !parseJSON(w, r, &req) {
		return
	}

	// 4. Input validation
	if req.FromAccountID <= 0 || req.ToAccountID <= 0 {
		writeError(w, http.StatusBadRequest, "Account IDs must be positive integers")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "Transfer amount must be greater than zero")
		return
	}
	if req.FromAccountID == req.ToAccountID {
		writeError(w, http.StatusBadRequest, "Source and destination account IDs cannot be identical")
		return
	}

	// 5. Calling service layer
	result, err := h.transferService.ExecuteTransfer(r.Context(), service.TransferDTO{
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		InitiatedBy:   caller,
	})

	// 6. Mapping service errors to HTTP status codes
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInsufficientBalance):
			writeError(w, http.StatusBadRequest, "Insufficient funds in source account")
		case errors.Is(err, service.ErrAccountDoesNotExist):
			writeError(w, http.StatusNotFound, "One or more accounts not found")
		case errors.Is(err, service.ErrInvalidTransferAmount), errors.Is(err, service.ErrIdenticalAccounts):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "An unexpected error occurred")
		}
		return
	}

	// 7. JSON encoding & 200 OK
	res := TransferResponse{
		TransferID:    result.Transfer.ID,
		FromAccountID: result.Transfer.FromAccountID,
		ToAccountID:   result.Transfer.ToAccountID,
		Amount:        result.Transfer.Amount,
		FromBalance:   result.FromAccount.Balance,
		ToBalance:     result.ToAccount.Balance,
		CreatedAt:     database.FormatIST(result.Transfer.CreatedAt),
	}

	writeJSON(w, http.StatusOK, res)
}
