package handler

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"github.com/paranik-jpg/bank-ledger-api/internal/database"
	"github.com/paranik-jpg/bank-ledger-api/internal/service"
)

type UserHandler struct {
	userService service.UserServiceInterface
}

func NewUserHandler(userService service.UserServiceInterface) *UserHandler {
	return &UserHandler{userService: userService}
}

type RegisterUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
		return
	}

	// Validate fields
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if req.Username == "" || len(req.Username) < 3 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Username must be at least 3 characters"})
		return
	}

	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid email address"})
		return
	}

	if len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Password must be at least 6 characters"})
		return
	}

	user, err := h.userService.Register(r.Context(), service.RegisterUserInput{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		// Handle duplicate username/email (Postgres unique violation)
		if strings.Contains(err.Error(), "duplicate key value") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Username or email already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		return
	}

	res := UserResponse{
		Username:  user.Username,
		Email:     user.Email,
		CreatedAt: database.FormatIST(user.CreatedAt),
	}

	writeJSON(w, http.StatusCreated, res)
}
