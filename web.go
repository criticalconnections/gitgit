package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// The compiled React app (web/dist) is embedded into the binary.
//
//go:embed all:web/dist
var spaEmbed embed.FS

var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// renderMarkdown converts markdown to sanitized HTML (goldmark escapes raw
// HTML by default).
func renderMarkdown(src string) template.HTML {
	var buf strings.Builder
	if err := markdown.Convert([]byte(src), &buf); err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(src) + "</pre>")
	}
	return template.HTML(buf.String())
}

// serverBase returns the external base URL for clone instructions.
func serverBase(r *http.Request) string {
	if baseURL != "" {
		return strings.TrimSuffix(baseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func spaFS() fs.FS {
	sub, err := fs.Sub(spaEmbed, "web/dist")
	if err != nil {
		log.Fatalf("spa dist missing from embed: %v", err)
	}
	return sub
}

// serveSPA serves a static asset from the built frontend, falling back to
// index.html so client-side routing works on deep links.
func serveSPA(w http.ResponseWriter, r *http.Request) {
	var dist fs.FS = spaFS()
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := dist.Open(p); err == nil {
		f.Close()
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, dist, p)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, dist, "index.html")
}

// appSecurityHeaders hardens GitGit's own UI/API responses (not previews,
// which get their sandbox CSP instead): deny framing to block clickjacking.
func appSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/p/") && previewTokenFromHost(r.Host) == "" {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func buildHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/", handleAPI)
	// Unauthenticated on purpose: a load balancer and a monitor have no
	// credentials, and neither endpoint reveals anything a visitor could not
	// already count from the UI.
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /metrics", handleMetrics)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Preview Environments own their entire subdomain, so this check comes
		// before any of GitGit's own routes.
		if token := previewTokenFromHost(r.Host); token != "" {
			servePreviewHost(w, r, token)
			return
		}

		// branch previews: /p/{token}/{path...}
		if strings.HasPrefix(r.URL.Path, "/p/") {
			rest := strings.TrimPrefix(r.URL.Path, "/p/")
			token, path, _ := strings.Cut(rest, "/")
			servePreview(w, r, token, path)
			return
		}

		segs := strings.SplitN(strings.Trim(r.URL.Path, "/"), "/", 4)

		// git smart HTTP + raw files are served by the backend directly
		if len(segs) >= 3 {
			rest := strings.Join(segs[2:], "/")
			if rest == "info/refs" || rest == "git-upload-pack" || rest == "git-receive-pack" {
				if repo, err := getRepo(segs[0], segs[1]); err == nil {
					handleGitSmartHTTP(w, r, repo, rest)
				} else {
					http.NotFound(w, r)
				}
				return
			}
			if segs[2] == "raw" {
				sub := ""
				if len(segs) == 4 {
					sub = segs[3]
				}
				serveRaw(w, r, segs[0], segs[1], sub)
				return
			}
			if segs[2] == "archive" && len(segs) == 4 {
				serveArchive(w, r, segs[0], segs[1], segs[3])
				return
			}
		}
		// a bare /owner/repo.git → canonical SPA route
		if len(segs) == 2 && strings.HasSuffix(segs[1], ".git") {
			http.Redirect(w, r, "/"+segs[0]+"/"+strings.TrimSuffix(segs[1], ".git"), http.StatusSeeOther)
			return
		}
		serveSPA(w, r)
	})

	return withSession(appSecurityHeaders(logRequests(mux)))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			log.Printf("%s %s %s", r.Method, redactLogPath(r.URL.Path), time.Since(start).Round(time.Millisecond))
		}
	})
}

// redactLogPath elides preview capability tokens so logs never contain a
// working credential to a preview (which needs no other auth).
func redactLogPath(p string) string {
	if !strings.HasPrefix(p, "/p/") {
		return p
	}
	rest := strings.TrimPrefix(p, "/p/")
	token, tail, _ := strings.Cut(rest, "/")
	_ = token
	if tail != "" {
		return "/p/{token}/" + tail
	}
	return "/p/{token}/"
}
