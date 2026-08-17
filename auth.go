package main

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

const sessionCookie = "gitgit_session"

// withSession loads the session cookie (if any) into the request context.
func withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if s := getSession(c.Value); s != nil {
				if u, err := getUserByID(s.UserID); err == nil {
					ctx := context.WithValue(r.Context(), ctxUser, u)
					ctx = context.WithValue(ctx, ctxSession, s)
					r = r.WithContext(ctx)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// currentUser returns the logged-in user or nil.
func currentUser(r *http.Request) *User {
	if u, ok := r.Context().Value(ctxUser).(*User); ok {
		return u
	}
	return nil
}

func currentSession(r *http.Request) *Session {
	if s, ok := r.Context().Value(ctxSession).(*Session); ok {
		return s
	}
	return nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}

// basicAuthUser authenticates a git/API client via HTTP Basic or token header.
// Password may be the account password or a personal access token.
func basicAuthUser(r *http.Request) *User {
	// Authorization: token <tok> / Bearer <tok> (API style)
	if ah := r.Header.Get("Authorization"); ah != "" {
		low := strings.ToLower(ah)
		if strings.HasPrefix(low, "token ") || strings.HasPrefix(low, "bearer ") {
			tok := strings.TrimSpace(ah[strings.IndexByte(ah, ' ')+1:])
			return userByToken(tok)
		}
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil
	}
	// token can be used as the password with any username, or as the username
	if u := userByToken(password); u != nil {
		return u
	}
	if u := userByToken(username); u != nil {
		return u
	}
	u, err := getUserByName(username)
	if err != nil {
		return nil
	}
	if !checkPassword(u, password) {
		return nil
	}
	return u
}

func requireBasicAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="gitgit"`)
	http.Error(w, "authentication required", http.StatusUnauthorized)
}
