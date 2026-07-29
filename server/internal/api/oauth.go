package api

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	"github.com/swapnil404/orca/server/internal/auth"
)

type oauthStore interface {
	UserIDForOAuthIdentity(context.Context, string, string, string, string) (string, error)
	UserIsActive(context.Context, string) (bool, error)
}

// OAuthConfig configures the supported Goth providers and callback origin.
type OAuthConfig struct {
	CallbackBaseURL    string
	CookieSecret       string
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
}

// OAuthHandler serves GitHub and Google OAuth redirects and callbacks.
type OAuthHandler struct {
	store        oauthStore
	tokens       *auth.JWTManager
	random       io.Reader
	secureCookie bool
}

// NewOAuthHandler configures Goth and creates an OAuth authentication handler.
func NewOAuthHandler(users oauthStore, tokens *auth.JWTManager, config OAuthConfig) *OAuthHandler {
	providers := make([]goth.Provider, 0, 2)
	if config.GitHubClientID != "" {
		providers = append(providers, github.New(config.GitHubClientID, config.GitHubClientSecret, config.CallbackBaseURL+"/auth/github/callback"))
	}
	if config.GoogleClientID != "" {
		providers = append(providers, google.New(config.GoogleClientID, config.GoogleClientSecret, config.CallbackBaseURL+"/auth/google/callback"))
	}
	goth.UseProviders(providers...)

	cookies := sessions.NewCookieStore([]byte(config.CookieSecret))
	cookies.Options = &sessions.Options{
		Path: "/", HttpOnly: true, Secure: strings.HasPrefix(config.CallbackBaseURL, "https://"), SameSite: http.SameSiteLaxMode,
	}
	gothic.Store = cookies
	return &OAuthHandler{
		store: users, tokens: tokens, random: rand.Reader,
		secureCookie: strings.HasPrefix(config.CallbackBaseURL, "https://"),
	}
}

// RegisterRoutes registers public OAuth initiation and callback routes.
func (h *OAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/{provider}", h.begin)
	mux.HandleFunc("GET /auth/{provider}/callback", h.callback)
}

func (h *OAuthHandler) begin(w http.ResponseWriter, r *http.Request) {
	if !supportedOAuthProvider(r.PathValue("provider")) {
		http.Redirect(w, r, "/login?oauth_error=provider_unavailable", http.StatusSeeOther)
		return
	}
	gothic.BeginAuthHandler(w, r)
}

func (h *OAuthHandler) callback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !supportedOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "OAuth provider is not configured")
		return
	}
	providerUser, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		h.redirectFailure(w, r)
		return
	}
	if strings.TrimSpace(providerUser.UserID) == "" {
		h.redirectFailure(w, r)
		return
	}
	newUserID, err := randomID(h.random)
	if err != nil {
		h.redirectFailure(w, r)
		return
	}

	// Linking another provider to an already-authenticated user is deliberately
	// deferred; it needs a separate authenticated, CSRF-protected linking flow.
	userID, err := h.store.UserIDForOAuthIdentity(r.Context(), provider, providerUser.UserID, providerUser.Email, newUserID)
	if err != nil {
		h.redirectFailure(w, r)
		return
	}
	active, err := h.store.UserIsActive(r.Context(), userID)
	if err != nil {
		h.redirectFailure(w, r)
		return
	}
	if !active {
		h.redirectFailure(w, r)
		return
	}
	token, err := h.tokens.IssueToken(userID)
	if err != nil {
		h.redirectFailure(w, r)
		return
	}
	setSessionCookie(w, token, h.secureCookie)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *OAuthHandler) redirectFailure(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login?oauth_error=authentication_failed", http.StatusSeeOther)
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.SessionCookieName, Value: token, Path: "/", MaxAge: 24 * 60 * 60,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func supportedOAuthProvider(provider string) bool {
	_, err := goth.GetProvider(provider)
	return err == nil
}
