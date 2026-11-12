package web

import (
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/db"
	"github.com/fmuoria/CV-Review-agent/internal/gmail"
	"github.com/fmuoria/CV-Review-agent/internal/store"
)

func validRole(r string) bool {
	return r == store.RoleAdmin || r == store.RoleHR || r == store.RoleHiringManager
}

func (s *Server) usersPage(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, "users", "People with access", map[string]any{"Users": users, "Domain": s.Cfg.AllowedEmailDomain})
}

func (s *Server) userCreate(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	name := strings.TrimSpace(r.FormValue("name"))
	role := r.FormValue("role")
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		s.redirect(w, r, "/admin/users", "error", "Enter a valid email address.")
		return
	}
	if d := s.Cfg.AllowedEmailDomain; d != "" && !strings.HasSuffix(email, "@"+d) {
		s.redirect(w, r, "/admin/users", "error", "Only @"+d+" accounts can sign in.")
		return
	}
	if !validRole(role) {
		s.redirect(w, r, "/admin/users", "error", "Choose a role.")
		return
	}
	_, err := s.Store.CreateUser(r.Context(), email, name, role)
	if db.IsUniqueViolation(err) {
		s.redirect(w, r, "/admin/users", "error", email+" already has access.")
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/admin/users", "ok", email+" can now sign in with Google.")
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	me, _ := currentUser(r)
	target, err := s.Store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	role := r.FormValue("role")
	active := r.FormValue("active") == "yes"
	if !validRole(role) {
		s.redirect(w, r, "/admin/users", "error", "Choose a role.")
		return
	}
	if target.ID == me.ID && (role != store.RoleAdmin || !active) {
		s.redirect(w, r, "/admin/users", "error", "You can't remove your own admin access. Ask another admin.")
		return
	}
	if err := s.Store.UpdateUser(r.Context(), id, strings.TrimSpace(r.FormValue("name")), role, active); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/admin/users", "ok", "Saved "+target.Email+".")
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Google": s.Cfg.GoogleSignInEnabled()}
	if integ, err := s.Store.GetIntegration(r.Context(), "gmail"); err == nil {
		data["Gmail"] = integ
	}
	s.render(w, r, "settings", "Settings", data)
}

func (s *Server) gmailConnect(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.GoogleSignInEnabled() {
		s.redirect(w, r, "/settings", "error", "Set up Google sign-in (GOOGLE_CLIENT_ID / SECRET) before connecting Gmail.")
		return
	}
	extra := url.Values{"access_type": {"offline"}, "prompt": {"consent select_account"}, "include_granted_scopes": {"true"}}
	s.oauthStart(w, r, "gmail", "/settings/gmail/callback", "openid email "+gmail.ReadonlyScope, extra)
}

func (s *Server) gmailCallback(w http.ResponseWriter, r *http.Request) {
	tok, id, _, err := s.oauthFinish(r, "gmail", "/settings/gmail/callback")
	if err != nil {
		s.redirect(w, r, "/settings", "error", err.Error())
		return
	}
	if tok.RefreshToken == "" {
		s.redirect(w, r, "/settings", "error", "Google didn't grant offline access. Remove the app at myaccount.google.com/permissions and connect again.")
		return
	}
	cipher, err := s.Box.Encrypt(tok.RefreshToken)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	u, _ := currentUser(r)
	if err := s.Store.SaveIntegration(r.Context(), "gmail", id.Email, cipher, u.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings", "ok", "Connected Gmail inbox "+id.Email+" (read-only).")
}

func (s *Server) gmailDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteIntegration(r.Context(), "gmail"); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.redirect(w, r, "/settings", "ok", "Gmail disconnected. To revoke access fully, also remove the app at myaccount.google.com/permissions.")
}
