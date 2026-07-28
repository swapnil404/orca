package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

type userAuthStore interface {
	CreatePasswordUser(context.Context, string, string, string) (store.User, error)
	UserByEmail(context.Context, string) (store.User, error)
}

// UserAuthHandler serves email/password registration and login.
type UserAuthHandler struct {
	store  userAuthStore
	tokens *auth.JWTManager
	random io.Reader
}

// NewUserAuthHandler creates an email/password authentication handler.
func NewUserAuthHandler(users userAuthStore, tokens *auth.JWTManager) *UserAuthHandler {
	return &UserAuthHandler{store: users, tokens: tokens, random: rand.Reader}
}

// RegisterRoutes registers public email/password authentication routes.
func (h *UserAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *UserAuthHandler) register(w http.ResponseWriter, r *http.Request) {
	request, ok := readCredentials(w, r)
	if !ok {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to secure password")
		return
	}
	id, err := randomID(h.random)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate user ID")
		return
	}
	user, err := h.store.CreatePasswordUser(r.Context(), id, request.Email, string(hash))
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(w, http.StatusConflict, "email is already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	h.writeToken(w, http.StatusCreated, user.ID)
}

func (h *UserAuthHandler) login(w http.ResponseWriter, r *http.Request) {
	request, ok := readCredentials(w, r)
	if !ok {
		return
	}
	user, err := h.store.UserByEmail(r.Context(), request.Email)
	if err != nil || !user.PasswordHash.Valid || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(request.Password)) != nil {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.writeToken(w, http.StatusOK, user.ID)
}

func (h *UserAuthHandler) writeToken(w http.ResponseWriter, status int, userID string) {
	token, err := h.tokens.IssueToken(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(w, status, struct {
		Token string `json:"token"`
	}{Token: token})
}

func readCredentials(w http.ResponseWriter, r *http.Request) (credentialsRequest, bool) {
	var request credentialsRequest
	if !decodeJSON(w, r, &request) {
		return credentialsRequest{}, false
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if request.Email == "" || !strings.Contains(request.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return credentialsRequest{}, false
	}
	if request.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return credentialsRequest{}, false
	}
	if len(request.Password) > 72 {
		writeError(w, http.StatusBadRequest, "password must not exceed 72 bytes")
		return credentialsRequest{}, false
	}
	return request, true
}
