package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type activeUsers struct {
	active bool
	err    error
	seen   []string
}

func (s *activeUsers) UserIsActive(_ context.Context, id string) (bool, error) {
	s.seen = append(s.seen, id)
	return s.active, s.err
}

func testJWTManager(t *testing.T) (*JWTManager, *activeUsers, string) {
	t.Helper()
	users := &activeUsers{active: true}
	manager, err := NewJWTManager("test-only-signing-secret", users, "https://orca.example.test")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }
	token, err := manager.IssueToken("test-user")
	if err != nil {
		t.Fatal(err)
	}
	return manager, users, token
}

func TestJWTChecksUserStatusOnEveryAuthentication(t *testing.T) {
	manager, users, token := testJWTManager(t)
	if id, err := manager.Authenticate(context.Background(), token); err != nil || id != "test-user" {
		t.Fatalf("authentication = %q, %v", id, err)
	}
	users.active = false
	if _, err := manager.Authenticate(context.Background(), token); err == nil {
		t.Fatal("deleted user kept access")
	}
	users.active = true
	users.err = errors.New("store unavailable")
	if _, err := manager.Authenticate(context.Background(), token); err == nil {
		t.Fatal("user lookup failure granted access")
	}
	if len(users.seen) != 3 {
		t.Fatal("user status was cached")
	}
}

func TestJWTRejectsExpiredAndWrongAlgorithmTokens(t *testing.T) {
	manager, _, token := testJWTManager(t)
	issued := manager.now()
	manager.now = func() time.Time { return issued.Add(tokenLifetime) }
	if _, err := manager.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expired token accepted")
	}
	manager.now = func() time.Time { return issued }
	wrong, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.RegisteredClaims{Subject: "test-user", ExpiresAt: jwt.NewNumericDate(issued.Add(time.Hour))}).SignedString(manager.secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Authenticate(context.Background(), wrong); err == nil {
		t.Fatal("wrong signing algorithm accepted")
	}
}

func TestJWTMiddlewareCookieOriginAndBearerPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name, method, origin, bearer string
		cookie                       bool
		want                         int
	}{
		{"cookie read", http.MethodGet, "", "", true, http.StatusNoContent},
		{"same origin cookie write", http.MethodPost, "https://orca.example.test", "", true, http.StatusNoContent},
		{"cross origin cookie write", http.MethodPost, "https://other.example.test", "", true, http.StatusForbidden},
		{"cookie write missing origin", http.MethodPost, "", "", true, http.StatusForbidden},
		{"explicit bearer write", http.MethodPost, "", "valid", false, http.StatusNoContent},
		{"invalid bearer cannot fall back to cookie", http.MethodGet, "", "invalid", true, http.StatusUnauthorized},
		{"missing credentials", http.MethodGet, "", "", false, http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager, _, token := testJWTManager(t)
			request := httptest.NewRequest(tt.method, "https://orca.example.test/projects", nil)
			if tt.cookie {
				request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
			}
			if tt.bearer == "valid" {
				request.Header.Set("Authorization", "Bearer "+token)
			} else if tt.bearer != "" {
				request.Header.Set("Authorization", "Bearer invalid")
			}
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			handler := manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, ok := UserIDFromContext(r.Context())
				if !ok || id != "test-user" {
					t.Error("authenticated user missing from context")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
		})
	}
}
