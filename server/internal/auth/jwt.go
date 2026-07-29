package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenLifetime = 24 * time.Hour
	// SessionCookieName is the shared browser session cookie used by REST and WebSocket authentication.
	SessionCookieName = "orca.session"
)

type userIDContextKey struct{}

type userStore interface {
	UserIsActive(context.Context, string) (bool, error)
}

// JWTManager issues and validates control-plane user tokens.
type JWTManager struct {
	secret        []byte
	now           func() time.Time
	users         userStore
	browserOrigin string
}

// NewJWTManager creates a JWT manager using an HS256 signing secret and browser origin.
func NewJWTManager(secret string, users userStore, browserOrigin string) (*JWTManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("ORCA_JWT_SECRET is required")
	}
	if users == nil {
		return nil, errors.New("user store is required")
	}
	parsedOrigin, err := url.Parse(browserOrigin)
	if err != nil || (parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https") || parsedOrigin.Host == "" {
		return nil, errors.New("browser origin must be an http:// or https:// URL")
	}
	return &JWTManager{
		secret: []byte(secret), now: time.Now, users: users,
		browserOrigin: parsedOrigin.Scheme + "://" + parsedOrigin.Host,
	}, nil
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

// Middleware requires a bearer token or browser session cookie and adds its user ID to the request context.
func (m *JWTManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, cookieAuthenticated, err := m.authenticateRequest(r)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		if cookieAuthenticated && !sameOriginMutation(r, m.browserOrigin) {
			writeForbidden(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
	})
}

// AuthenticateRequest validates the explicit bearer token or browser session cookie on r.
func (m *JWTManager) AuthenticateRequest(r *http.Request) (string, error) {
	userID, _, err := m.authenticateRequest(r)
	return userID, err
}

func (m *JWTManager) authenticateRequest(r *http.Request) (string, bool, error) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if value != "" {
		parts := strings.Fields(value)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return "", false, errors.New("invalid authorization header")
		}
		userID, err := m.Authenticate(r.Context(), parts[1])
		return userID, false, err
	}

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false, errors.New("authentication required")
	}
	userID, err := m.Authenticate(r.Context(), cookie.Value)
	return userID, true, err
}

// Authenticate validates a token and verifies that its subject is still an active user.
func (m *JWTManager) Authenticate(ctx context.Context, value string) (string, error) {
	userID, err := m.validate(value)
	if err != nil {
		return "", err
	}
	active, err := m.users.UserIsActive(ctx, userID)
	if err != nil || !active {
		return "", errors.New("invalid JWT")
	}
	return userID, nil
}

func (m *JWTManager) validate(value string) (string, error) {
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

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("{\"error\":\"cross-origin request rejected\"}\n"))
}

func sameOriginMutation(r *http.Request, browserOrigin string) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Scheme+"://"+parsed.Host, browserOrigin)
}
