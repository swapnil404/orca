package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenLifetime = 24 * time.Hour

type userIDContextKey struct{}

// JWTManager issues and validates control-plane user tokens.
type JWTManager struct {
	secret []byte
	now    func() time.Time
}

// NewJWTManager creates a JWT manager using an HS256 signing secret.
func NewJWTManager(secret string) (*JWTManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("ORCA_JWT_SECRET is required")
	}
	return &JWTManager{secret: []byte(secret), now: time.Now}, nil
}

// IssueToken creates a signed user token with a fixed lifetime.
func (m *JWTManager) IssueToken(userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", errors.New("user ID is required")
	}
	now := m.now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenLifetime)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Middleware requires an Authorization bearer token and adds its user ID to the request context.
func (m *JWTManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.Fields(value)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeUnauthorized(w)
			return
		}
		userID, err := m.Validate(parts[1])
		if err != nil {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
	})
}

// Validate verifies a token and returns its subject user ID.
func (m *JWTManager) Validate(value string) (string, error) {
	token, err := jwt.ParseWithClaims(value, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected JWT signing algorithm")
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(m.now))
	if err != nil || !token.Valid {
		return "", errors.New("invalid JWT")
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || strings.TrimSpace(claims.Subject) == "" {
		return "", errors.New("JWT subject is required")
	}
	return claims.Subject, nil
}

// WithUserID associates an authenticated user ID with a context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// UserIDFromContext returns the authenticated user ID stored in ctx.
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	return userID, ok && strings.TrimSpace(userID) != ""
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("{\"error\":\"authentication required\"}\n"))
}
