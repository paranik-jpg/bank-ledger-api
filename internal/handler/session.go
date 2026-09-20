package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
	"github.com/paranik-jpg/bank-ledger-api/internal/service"
)

type SessionHandler struct {
	authService service.AuthServiceInterface
}

func NewSessionHandler(authService service.AuthServiceInterface) *SessionHandler {
	return &SessionHandler{authService: authService}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SessionResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

func (h *SessionHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Username and password are required"})
		return
	}

	loginResult, err := h.authService.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid username or password"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	res := SessionResponse{
		AccessToken: loginResult.AccessToken,
		ExpiresAt:   database.FormatIST(loginResult.ExpiresAt),
	}

	writeJSON(w, http.StatusOK, res)
}
