package web

import (
	"net/http"

	"menata.app/internal/authorization"
	"menata.app/internal/config"
	"menata.app/internal/rendering"
)

func showLogin(w http.ResponseWriter, req *http.Request) {
	render(req.Context(), w, rendering.LoginPage(""))
}

func submitLogin(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}
		if !authorization.CheckCredentials(req.FormValue("username"), req.FormValue("password"), cfg.AdminUsername, cfg.AdminPassword) {
			w.WriteHeader(http.StatusUnauthorized)
			render(req.Context(), w, rendering.LoginPage("Invalid username or password"))
			return
		}
		authorization.SetSessionCookie(w, cfg.SessionSecret, cfg.AdminUserID, cfg.SecureCookies)
		http.Redirect(w, req, "/", http.StatusSeeOther)
	}
}

func logout(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		authorization.ClearSessionCookie(w, cfg.SecureCookies)
		http.Redirect(w, req, "/login", http.StatusSeeOther)
	}
}
