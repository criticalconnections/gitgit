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

// hostSessionCookie is the hardened name used over HTTPS. The "__Host-"
// prefix makes browsers refuse the cookie unless it is Secure, Path=/, and
// carries NO Domain attribute — which also means a Preview Environment on a
// sibling subdomain cannot set or shadow it. That matters because previews
// are served from *.<preview-domain>, under the same registrable domain as
// the forge itself.
const hostSessionCookie = "__Host-" + sessionCookie

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// readSessionCookie prefers the hardened cookie and falls back to the plain
// one (HTTP development, and sessions issued before this was introduced).
func readSessionCookie(r *http.Request) string {
	if c, err := r.Cookie(hostSessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// withSession loads the session cookie (if any) into the request context.
func withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := readSessionCookie(r); tok != "" {
			if s := getSession(tok); s != nil {
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
	secure := isSecureRequest(r)
	name := sessionCookie
	if secure {
		name = hostSessionCookie // "__Host-" requires Secure; see the const doc
	}
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	// clear both names: the hardened one and any legacy cookie still held
	for _, n := range []string{hostSessionCookie, sessionCookie} {
		http.SetCookie(w, &http.Cookie{Name: n, Value: "", Path: "/", MaxAge: -1, Secure: n == hostSessionCookie})
	}
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
