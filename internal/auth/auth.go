package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

type Config struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	AllowedEmails []string
	SessionKey    string
	SessionKeyOld string
}

type Auth struct {
	Store         *sessions.CookieStore
	OAuthConfig   *oauth2.Config
	Provider      *oidc.Provider
	Verifier      *oidc.IDTokenVerifier
	AllowedEmails []string
	AdminPath     string
}

func Init(cfg Config, adminPath string) *Auth {
	var keyPairs [][]byte
	if cfg.SessionKey != "" {
		keyPairs = append(keyPairs, []byte(cfg.SessionKey))
	}
	if cfg.SessionKeyOld != "" {
		keyPairs = append(keyPairs, []byte(cfg.SessionKeyOld))
	}

	var store *sessions.CookieStore
	if len(keyPairs) > 0 {
		store = sessions.NewCookieStore(keyPairs...)
		store.Options = &sessions.Options{
			Path:     "/",
			MaxAge:   3600 * 8,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		}
	}

	a := &Auth{
		Store:         store,
		AllowedEmails: cfg.AllowedEmails,
		AdminPath:     adminPath,
	}

	if cfg.ClientID == "" {
		log.Println("WARN: OAuth not configured, admin panel will be inaccessible")
		return a
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		log.Printf("OIDC Provider init failed: %v", err)
		return a
	}
	
	a.Provider = provider
	a.OAuthConfig = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	
	a.Verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	log.Println("OAuth initialized")

	return a
}

func WithAuth(store *sessions.CookieStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "Auth not configured", http.StatusServiceUnavailable)
			return
		}
		session, _ := store.Get(r, "auth-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/api/auth/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (a *Auth) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if a.OAuthConfig == nil {
		http.Error(w, "Auth not configured", http.StatusInternalServerError)
		return
	}
	state := make([]byte, 32)
	rand.Read(state)
	stateStr := base64.StdEncoding.EncodeToString(state)

	session, _ := a.Store.Get(r, "auth-session")
	session.Values["state"] = stateStr
	session.Save(r, w)

	http.Redirect(w, r, a.OAuthConfig.AuthCodeURL(stateStr), http.StatusFound)
}

func (a *Auth) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := a.Store.Get(r, "auth-session")
	state, ok := session.Values["state"].(string)
	if !ok || r.URL.Query().Get("state") != state {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	token, err := a.OAuthConfig.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token", http.StatusInternalServerError)
		return
	}

	idToken, err := a.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Token verification failed", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Claims parse failed", http.StatusInternalServerError)
		return
	}

	allowed := false
	for _, email := range a.AllowedEmails {
		if strings.TrimSpace(email) == claims.Email {
			allowed = true
			break
		}
	}

	if !allowed {
		http.Error(w, "Unauthorized: Email not allowed", http.StatusUnauthorized)
		return
	}

	session.Values["authenticated"] = true
	session.Values["email"] = claims.Email
	delete(session.Values, "state")
	session.Save(r, w)

	redirectTo := a.AdminPath
	if redirectTo == "" {
		redirectTo = "/admin"
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}
