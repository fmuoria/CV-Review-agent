package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fmuoria/CV-Review-agent/internal/secure"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

// googleIdentity is the part of Google's ID token we use.
type googleIdentity struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	HostedDomain  string `json:"hd"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); ok {
		http.Redirect(w, r, "/jobs", http.StatusSeeOther)
		return
	}
	data := map[string]any{
		"Google":  s.Cfg.GoogleSignInEnabled(),
		"DevMode": !s.Cfg.IsProduction(),
		"Next":    safeNext(r.URL.Query().Get("next")),
		"Error":   r.URL.Query().Get("error"),
		"Domain":  s.Cfg.AllowedEmailDomain,
	}
	s.render(w, r, "login", "Sign in", data)
}

func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/jobs"
	}
	return next
}

// oauthStart sends the browser to Google. The state is signed and bound to a cookie.
func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request, purpose, redirectPath, scope string, extra url.Values) {
	nonce := secure.RandomToken(16)
	next := safeNext(r.URL.Query().Get("next"))
	state := s.Box.Sign(purpose+"|"+nonce+"|"+next, 10*time.Minute)
	http.SetCookie(w, &http.Cookie{Name: "ats_oauth", Value: nonce, Path: "/", MaxAge: 600, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: s.Cfg.IsProduction()})
	q := url.Values{
		"client_id":     {s.Cfg.GoogleClientID},
		"redirect_uri":  {s.Cfg.BaseURL + redirectPath},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
	}
	for k, v := range extra {
		q[k] = v
	}
	http.Redirect(w, r, googleAuthURL+"?"+q.Encode(), http.StatusSeeOther)
}

// oauthFinish validates the state and exchanges the code for tokens.
func (s *Server) oauthFinish(r *http.Request, purpose, redirectPath string) (tokenResponse, googleIdentity, string, error) {
	if e := r.URL.Query().Get("error"); e != "" {
		return tokenResponse{}, googleIdentity{}, "", fmt.Errorf("Google sign-in was cancelled (%s)", e)
	}
	state, err := s.Box.Verify(r.URL.Query().Get("state"))
	if err != nil {
		return tokenResponse{}, googleIdentity{}, "", errors.New("sign-in link expired, please try again")
	}
	parts := strings.SplitN(state, "|", 3)
	c, cerr := r.Cookie("ats_oauth")
	if len(parts) != 3 || parts[0] != purpose || cerr != nil || c.Value != parts[1] {
		return tokenResponse{}, googleIdentity{}, "", errors.New("sign-in could not be verified, please try again")
	}
	tok, err := s.exchangeCode(r.Context(), r.URL.Query().Get("code"), s.Cfg.BaseURL+redirectPath)
	if err != nil {
		return tok, googleIdentity{}, "", err
	}
	id, err := decodeIDToken(tok.IDToken)
	return tok, id, parts[2], err
}

func (s *Server) exchangeCode(ctx context.Context, code, redirectURI string) (tokenResponse, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {s.Cfg.GoogleClientID},
		"client_secret": {s.Cfg.GoogleClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("contact Google: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("Google rejected the sign-in (%d)", resp.StatusCode)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return tr, err
	}
	return tr, nil
}

// decodeIDToken reads the claims of an ID token received directly from
// Google's token endpoint over TLS (OpenID Connect §3.1.3.7 allows skipping
// signature checks in this case).
func decodeIDToken(tok string) (googleIdentity, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return googleIdentity{}, errors.New("Google did not return an identity")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleIdentity{}, err
	}
	var id googleIdentity
	if err := json.Unmarshal(raw, &id); err != nil {
		return id, err
	}
	id.Email = strings.ToLower(id.Email)
	return id, nil
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.GoogleSignInEnabled() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	extra := url.Values{"prompt": {"select_account"}}
	if s.Cfg.AllowedEmailDomain != "" {
		extra.Set("hd", s.Cfg.AllowedEmailDomain)
	}
	s.oauthStart(w, r, "login", "/auth/callback", "openid email profile", extra)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	_, id, next, err := s.oauthFinish(r, "login", "/auth/callback")
	if err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	if !id.EmailVerified {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Your Google email address is not verified."), http.StatusSeeOther)
		return
	}
	if d := s.Cfg.AllowedEmailDomain; d != "" && (id.HostedDomain != d || !strings.HasSuffix(id.Email, "@"+d)) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Please sign in with your @"+d+" account."), http.StatusSeeOther)
		return
	}
	s.signIn(w, r, id.Email, id.Name, next)
}

// signIn creates the session for a known (or bootstrap-admin) user.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request, email, name, next string) {
	ctx := r.Context()
	u, err := s.Store.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) && s.Cfg.IsAdminEmail(email) {
		u, err = s.Store.CreateUser(ctx, email, name, store.RoleAdmin)
	}
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape(email+" hasn't been given access yet. Ask an admin to add you."), http.StatusSeeOther)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !u.Active {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("This account has been deactivated."), http.StatusSeeOther)
		return
	}
	sess, err := s.Store.CreateSession(ctx, u.ID, sessionTTL)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	_ = s.Store.TouchLogin(ctx, u.ID, name)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sess.ID, Path: "/", Expires: sess.ExpiresAt,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.Cfg.IsProduction()})
	http.SetCookie(w, &http.Cookie{Name: "ats_oauth", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, safeNext(next), http.StatusSeeOther)
}

// devLogin lets you sign in as anyone on your laptop. Disabled in production.
func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.IsProduction() {
		http.NotFound(w, r)
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	role := r.FormValue("role")
	if email == "" || !strings.Contains(email, "@") {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Enter an email address."), http.StatusSeeOther)
		return
	}
	if _, err := s.Store.GetUserByEmail(r.Context(), email); errors.Is(err, store.ErrNotFound) {
		if role != store.RoleHR && role != store.RoleHiringManager {
			role = store.RoleAdmin
		}
		if _, err := s.Store.CreateUser(r.Context(), email, "", role); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	s.signIn(w, r, email, "", r.FormValue("next"))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := currentSession(r); ok {
		if r.FormValue("csrf") != sess.CSRFToken {
			http.Error(w, "Invalid request", http.StatusForbidden)
			return
		}
		_ = s.Store.DeleteSession(r.Context(), sess.ID)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
