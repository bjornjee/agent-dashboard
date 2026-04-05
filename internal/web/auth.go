package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// authHandler manages Google OAuth flow and session cookies.
type authHandler struct {
	oauthConfig  *oauth2.Config
	allowedEmail string
	secret       []byte
}

type sessionClaims struct {
	Email   string `json:"email"`
	Expires int64  `json:"exp"`
}

func newAuthHandler(opts ServerOptions) *authHandler {
	return &authHandler{
		oauthConfig: &oauth2.Config{
			ClientID:     opts.GoogleClientID,
			ClientSecret: opts.GoogleClientSecret,
			RedirectURL:  "", // set dynamically from request
			Scopes:       []string{"openid", "email"},
			Endpoint:     google.Endpoint,
		},
		allowedEmail: opts.AllowedEmail,
		secret:       []byte(opts.SessionSecret),
	}
}

func (a *authHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agent Dashboard - Login</title>
<style>
body{font-family:system-ui;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#1e1e2e;color:#cdd6f4}
.card{text-align:center;padding:2rem;border-radius:12px;background:#313244;max-width:400px}
h1{margin-bottom:0.5rem}
p{color:#a6adc8;margin-bottom:1.5rem}
a.btn{display:inline-block;padding:12px 24px;background:#89b4fa;color:#1e1e2e;text-decoration:none;border-radius:8px;font-weight:600}
a.btn:hover{background:#74c7ec}
.error{color:#f38ba8;margin-top:1rem}
</style></head><body>
<div class="card">
<h1>Agent Dashboard</h1>
<p>Sign in to access your dashboard</p>
<a class="btn" href="/auth/google">Sign in with Google</a>
`)
	if r.URL.Query().Get("error") == "denied" {
		fmt.Fprint(w, `<p class="error">Access denied. Your email is not authorized.</p>`)
	}
	fmt.Fprint(w, `</div></body></html>`)
}

func (a *authHandler) handleGoogleRedirect(w http.ResponseWriter, r *http.Request) {
	cfg := *a.oauthConfig
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	cfg.RedirectURL = fmt.Sprintf("%s://%s/auth/callback", scheme, r.Host)

	// Generate random state nonce to prevent CSRF on OAuth flow
	nonce, err := generateNonce()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Store nonce in a short-lived cookie for verification in callback
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth-state",
		Value:    nonce,
		Path:     "/auth/callback",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})

	url := cfg.AuthCodeURL(nonce, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (a *authHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/auth/login?error=denied", http.StatusTemporaryRedirect)
		return
	}

	// Verify OAuth state nonce against cookie
	stateParam := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie("oauth-state")
	if err != nil || stateCookie.Value == "" || stateParam != stateCookie.Value {
		http.Redirect(w, r, "/auth/login?error=denied", http.StatusTemporaryRedirect)
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth-state",
		Value:    "",
		Path:     "/auth/callback",
		MaxAge:   -1,
		HttpOnly: true,
	})

	cfg := *a.oauthConfig
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	cfg.RedirectURL = fmt.Sprintf("%s://%s/auth/callback", scheme, r.Host)

	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/auth/login?error=denied", http.StatusTemporaryRedirect)
		return
	}

	// Fetch email from Google userinfo endpoint (verified, no JWT parsing needed)
	email, err := fetchUserEmail(r, token, &cfg)
	if err != nil {
		http.Redirect(w, r, "/auth/login?error=denied", http.StatusTemporaryRedirect)
		return
	}

	if email != a.allowedEmail {
		http.Redirect(w, r, "/auth/login?error=denied", http.StatusTemporaryRedirect)
		return
	}

	sessionToken, err := a.createToken(email)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   7 * 24 * 3600, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (a *authHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]string{"ok": "logged out"})
}

// createToken creates an HMAC-signed session token containing the email and expiry.
func (a *authHandler) createToken(email string) (string, error) {
	claims := sessionClaims{
		Email:   email,
		Expires: time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := a.sign(encoded)
	return encoded + "." + sig, nil
}

// validateSession checks the session cookie and returns the email if valid.
func (a *authHandler) validateSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	expected := a.sign(parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", false
	}
	if time.Now().Unix() > claims.Expires {
		return "", false
	}
	return claims.Email, true
}

func (a *authHandler) sign(data string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// generateNonce creates a cryptographically random hex string for OAuth state.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// fetchUserEmail calls Google's userinfo endpoint to get the verified email.
// This avoids parsing/verifying the ID token JWT ourselves.
func fetchUserEmail(r *http.Request, token *oauth2.Token, cfg *oauth2.Config) (string, error) {
	client := cfg.Client(r.Context(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return "", fmt.Errorf("fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read userinfo: %w", err)
	}

	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("parse userinfo: %w", err)
	}
	if info.Email == "" || !info.EmailVerified {
		return "", fmt.Errorf("email not verified")
	}
	return info.Email, nil
}
