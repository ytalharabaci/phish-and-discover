// Package controllers: authentication and session-credential flows --
// login, logout, settings password change, forced password reset, and
// admin impersonation. Split out of route.go so credential-handling logic
// is grouped separately from route registration and simple page handlers.
package controllers

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	csrf "filippo.io/csrf/gorilla"
	"github.com/gorilla/sessions"
	"github.com/ytalharabaci/phish-and-discover/auth"
	ctx "github.com/ytalharabaci/phish-and-discover/context"
	"github.com/ytalharabaci/phish-and-discover/controllers/api"
	log "github.com/ytalharabaci/phish-and-discover/logger"
	"github.com/ytalharabaci/phish-and-discover/models"
)

// Settings handles the changing of settings
func (as *AdminServer) Settings(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		params := newTemplateParams(r)
		params.Title = "Settings"
		session := ctx.Get(r, "session").(*sessions.Session)
		session.Save(r, w)
		getTemplate(w, "settings").ExecuteTemplate(w, "base", params)
	case r.Method == "POST":
		u := ctx.Get(r, "user").(models.User)
		currentPw := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_new_password")
		// Check the current password
		err := auth.ValidatePassword(currentPw, u.Hash)
		msg := models.Response{Success: true, Message: "Settings Updated Successfully"}
		if err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusBadRequest)
			return
		}
		newHash, err := auth.ValidatePasswordChange(u.Hash, newPassword, confirmPassword)
		if err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusBadRequest)
			return
		}
		u.Hash = string(newHash)
		if err = models.PutUser(&u); err != nil {
			msg.Message = err.Error()
			msg.Success = false
			api.JSONResponse(w, msg, http.StatusInternalServerError)
			return
		}
		api.JSONResponse(w, msg, http.StatusOK)
	}
}

// allowedNextPrefixes is an explicit allowlist of paths "next" may redirect
// to after login. It mirrors the routes registered in registerRoutes for a
// logged-in user, so every legitimate same-origin "next" link keeps
// working, while anything else (off-site, or not a real route) falls back
// to "/".
var allowedNextPrefixes = []string{
	"/", "/campaigns", "/templates", "/groups", "/landing_pages",
	"/sending_profiles", "/settings", "/users", "/webhooks",
}

func (as *AdminServer) nextOrIndex(w http.ResponseWriter, r *http.Request) {
	next := "/"
	parsed, err := url.Parse(r.FormValue("next"))
	if err == nil {
		path := parsed.EscapedPath()
		// Strip ALL leading slashes and backslashes before re-prepending a
		// single "/". This guarantees the result can never start with "//"
		// or "/\", which some browsers normalize into a protocol-relative
		// URL (e.g. "//evil.com") and would otherwise let an off-site
		// redirect slip through despite only the path component being used.
		path = strings.TrimLeft(path, "/\\")
		candidate := "/"
		if path != "" {
			candidate = "/" + path
		}
		for _, prefix := range allowedNextPrefixes {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				next = candidate
				break
			}
		}
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (as *AdminServer) handleInvalidLogin(w http.ResponseWriter, r *http.Request, message string) {
	session := ctx.Get(r, "session").(*sessions.Session)
	Flash(w, r, "danger", message)
	params := struct {
		User    models.User
		Title   string
		Flashes []interface{}
		Token   string
	}{Title: "Login", Token: csrf.Token(r)}
	params.Flashes = session.Flashes()
	session.Save(r, w)
	templates := template.New("template")
	_, err := templates.ParseFiles("templates/login.html", "templates/flashes.html")
	if err != nil {
		log.Error(err)
	}
	// w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	template.Must(templates, err).ExecuteTemplate(w, "base", params)
}

// Impersonate allows an admin to login to a user account without needing the password
func (as *AdminServer) Impersonate(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		username := r.FormValue("username")
		u, err := models.GetUserByUsername(username)
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		session := ctx.Get(r, "session").(*sessions.Session)
		session.Values["id"] = u.Id
		session.Save(r, w)
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// Login handles the authentication flow for a user. If credentials are valid,
// a session is created
func (as *AdminServer) Login(w http.ResponseWriter, r *http.Request) {
	params := struct {
		User    models.User
		Title   string
		Flashes []interface{}
		Token   string
	}{Title: "Login", Token: csrf.Token(r)}
	session := ctx.Get(r, "session").(*sessions.Session)
	switch {
	case r.Method == "GET":
		params.Flashes = session.Flashes()
		session.Save(r, w)
		templates := template.New("template")
		_, err := templates.ParseFiles("templates/login.html", "templates/flashes.html")
		if err != nil {
			log.Error(err)
		}
		template.Must(templates, err).ExecuteTemplate(w, "base", params)
	case r.Method == "POST":
		// Find the user with the provided username
		username, password := r.FormValue("username"), r.FormValue("password")
		u, err := models.GetUserByUsername(username)
		if err != nil {
			log.Error(err)
			as.handleInvalidLogin(w, r, "Invalid Username/Password")
			return
		}
		// Validate the user's password
		err = auth.ValidatePassword(password, u.Hash)
		if err != nil {
			log.Error(err)
			as.handleInvalidLogin(w, r, "Invalid Username/Password")
			return
		}
		if u.AccountLocked {
			as.handleInvalidLogin(w, r, "Account Locked")
			return
		}
		u.LastLogin = time.Now().UTC()
		err = models.PutUser(&u)
		if err != nil {
			log.Error(err)
		}
		// If we've logged in, save the session and redirect to the dashboard
		session.Values["id"] = u.Id
		session.Save(r, w)
		as.nextOrIndex(w, r)
	}
}

// Logout destroys the current user session
func (as *AdminServer) Logout(w http.ResponseWriter, r *http.Request) {
	session := ctx.Get(r, "session").(*sessions.Session)
	delete(session.Values, "id")
	Flash(w, r, "success", "You have successfully logged out")
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ResetPassword handles the password reset flow when a password change is
// required either by the Gophish system or an administrator.
//
// This handler is meant to be used when a user is required to reset their
// password, not just when they want to.
//
// This is an important distinction since in this handler we don't require
// the user to re-enter their current password, as opposed to the flow
// through the settings handler.
//
// To that end, if the user doesn't require a password change, we will
// redirect them to the settings page.
func (as *AdminServer) ResetPassword(w http.ResponseWriter, r *http.Request) {
	u := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)
	if !u.PasswordChangeRequired {
		Flash(w, r, "info", "Please reset your password through the settings page")
		session.Save(r, w)
		http.Redirect(w, r, "/settings", http.StatusTemporaryRedirect)
		return
	}
	params := newTemplateParams(r)
	params.Title = "Reset Password"
	switch {
	case r.Method == http.MethodGet:
		params.Flashes = session.Flashes()
		session.Save(r, w)
		getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
		return
	case r.Method == http.MethodPost:
		newPassword := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")
		newHash, err := auth.ValidatePasswordChange(u.Hash, newPassword, confirmPassword)
		if err != nil {
			Flash(w, r, "danger", err.Error())
			params.Flashes = session.Flashes()
			session.Save(r, w)
			w.WriteHeader(http.StatusBadRequest)
			getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
			return
		}
		u.PasswordChangeRequired = false
		u.Hash = newHash
		if err = models.PutUser(&u); err != nil {
			Flash(w, r, "danger", err.Error())
			params.Flashes = session.Flashes()
			session.Save(r, w)
			w.WriteHeader(http.StatusInternalServerError)
			getTemplate(w, "reset_password").ExecuteTemplate(w, "base", params)
			return
		}
		// TODO: We probably want to flash a message here that the password was
		// changed successfully. The problem is that when the user resets their
		// password on first use, they will see two flashes on the dashboard-
		// one for their password reset, and one for the "no campaigns created".
		//
		// The solution to this is to revamp the empty page to be more useful,
		// like a wizard or something.
		as.nextOrIndex(w, r)
	}
}
