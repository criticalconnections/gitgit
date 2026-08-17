package main

// Branch previews: capability-URL static serving of a repo tree, so a branch
// can be tested in a real browser (or on a phone, via QR) before merging.
//
// A preview is /p/{token}/… where the token is an unguessable capability.
// The preview follows its ref: push again and the same link serves the new
// tip. Responses carry a sandboxing CSP that puts previewed content in an
// opaque origin — its scripts cannot read GitGit cookies, and its requests
// fail GitGit's same-origin CSRF guard.

import (
	"html"
	"mime"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
)

const previewTTL = 24 * 3600 // seconds

type Preview struct {
	ID        int64
	RepoID    int64
	Ref       string
	Token     string
	CreatedBy int64
	CreatedAt int64
	ExpiresAt int64
}

func prunePreviews() {
	db.Exec("DELETE FROM previews WHERE expires_at < ?", now())
}

// createPreview returns an existing live preview for repo+ref or makes one.
func createPreview(repoID, userID int64, ref string) (*Preview, error) {
	prunePreviews()
	if p := previewByRepoRef(repoID, ref); p != nil {
		// refresh expiry so an active testing loop keeps its QR working
		db.Exec("UPDATE previews SET expires_at = ? WHERE id = ?", now()+previewTTL, p.ID)
		p.ExpiresAt = now() + previewTTL
		return p, nil
	}
	p := &Preview{
		RepoID: repoID, Ref: ref, Token: randHex(16),
		CreatedBy: userID, CreatedAt: now(), ExpiresAt: now() + previewTTL,
	}
	res, err := db.Exec("INSERT INTO previews (repo_id, ref, token, created_by, created_at, expires_at) VALUES (?,?,?,?,?,?)",
		p.RepoID, p.Ref, p.Token, p.CreatedBy, p.CreatedAt, p.ExpiresAt)
	if err != nil {
		return nil, err
	}
	p.ID, _ = res.LastInsertId()
	return p, nil
}

func scanPreview(row interface{ Scan(...any) error }) *Preview {
	p := &Preview{}
	if row.Scan(&p.ID, &p.RepoID, &p.Ref, &p.Token, &p.CreatedBy, &p.CreatedAt, &p.ExpiresAt) != nil {
		return nil
	}
	return p
}

const previewCols = "id, repo_id, ref, token, created_by, created_at, expires_at"

func previewByRepoRef(repoID int64, ref string) *Preview {
	return scanPreview(db.QueryRow("SELECT "+previewCols+" FROM previews WHERE repo_id = ? AND ref = ? AND expires_at > ?", repoID, ref, now()))
}

func previewByToken(token string) *Preview {
	return scanPreview(db.QueryRow("SELECT "+previewCols+" FROM previews WHERE token = ? AND expires_at > ?", token, now()))
}

func listPreviews(repoID int64) []*Preview {
	prunePreviews()
	rows, err := db.Query("SELECT "+previewCols+" FROM previews WHERE repo_id = ? ORDER BY id DESC", repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Preview
	for rows.Next() {
		if p := scanPreview(rows); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func deletePreview(repoID, id int64) {
	db.Exec("DELETE FROM previews WHERE id = ? AND repo_id = ?", id, repoID)
}

// previewSandboxCSP isolates previewed content in an opaque origin: scripts
// run, but GitGit cookies are unreadable and mutating API calls fail the
// same-origin guard (Origin: null).
//
// This applies to PATH-served previews (/p/{token}/…), which share GitGit's
// origin. Previews served on their own subdomain get real origin separation
// instead, and skip the sandbox so that localStorage, cookies, and
// same-origin fetches behave the way they will in production.
const previewSandboxCSP = "sandbox allow-scripts allow-forms allow-modals allow-popups allow-pointer-lock"

func previewByID(id int64) *Preview {
	return scanPreview(db.QueryRow("SELECT "+previewCols+" FROM previews WHERE id = ?", id))
}

// previewHostFor returns the dedicated hostname for a preview, e.g.
// "a1b2c3….preview.example.com", or "" when no preview domain is configured.
func previewHostFor(p *Preview) string {
	if previewDomain == "" || p == nil {
		return ""
	}
	return p.Token + "." + previewDomain
}

// previewOrigin returns the full external URL of a preview: its own subdomain
// when one is configured, otherwise the path form under the main host.
func previewOrigin(p *Preview) string {
	if p == nil {
		return ""
	}
	if h := previewHostFor(p); h != "" {
		scheme := "https"
		if previewInsecure {
			scheme = "http"
		}
		return scheme + "://" + h
	}
	if baseURL != "" {
		return strings.TrimSuffix(baseURL, "/") + "/p/" + p.Token + "/"
	}
	return "/p/" + p.Token + "/"
}

// previewTokenFromHost extracts the token from a request Host when it belongs
// to the preview domain: "<token>.<previewDomain>" (port ignored).
func previewTokenFromHost(host string) string {
	if previewDomain == "" || host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	suffix := "." + strings.ToLower(previewDomain)
	if !strings.HasSuffix(host, suffix) {
		return ""
	}
	label := strings.TrimSuffix(host, suffix)
	// exactly one label, and it must look like a token
	if label == "" || strings.Contains(label, ".") {
		return ""
	}
	return label
}

// servePreviewHost handles a request that arrived on a preview subdomain.
func servePreviewHost(w http.ResponseWriter, r *http.Request, token string) {
	p, repo, sha, ok := resolvePreview(w, token)
	if !ok {
		return
	}
	// A running environment takes over the whole host, including non-GET
	// methods, so real applications (forms, APIs) work.
	if e := ensurePreviewEnv(repo, p, sha); e != nil {
		if e.Status == "running" && e.Port > 0 {
			proxyToEnv(w, r, e)
			return
		}
		writeEnvStatusPage(w, e, "")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serveStaticPreview(w, r, repo, p, sha, strings.TrimPrefix(r.URL.Path, "/"), false)
}

// resolvePreview validates a token and the access still backing it.
func resolvePreview(w http.ResponseWriter, token string) (*Preview, *Repo, string, bool) {
	p := previewByToken(token)
	if p == nil {
		http.Error(w, "preview not found or expired", http.StatusNotFound)
		return nil, nil, "", false
	}
	repo, err := getRepoByID(p.RepoID)
	if err != nil {
		http.Error(w, "preview not found or expired", http.StatusNotFound)
		return nil, nil, "", false
	}
	if creator, err := getUserByID(p.CreatedBy); err != nil || !canWrite(creator, repo) {
		http.Error(w, "preview no longer available", http.StatusNotFound)
		return nil, nil, "", false
	}
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, "refs/heads/"+p.Ref)
	if err != nil {
		if sha, err = resolveCommit(dir, p.Ref); err != nil {
			http.Error(w, "preview ref no longer exists", http.StatusNotFound)
			return nil, nil, "", false
		}
	}
	return p, repo, sha, true
}

// servePreview handles GET /p/{token}/{path...} — the path-served form, used
// when no preview domain is configured (local development) and as a fallback.
// Content here shares GitGit's origin, so it keeps the sandbox CSP.
func servePreview(w http.ResponseWriter, r *http.Request, token, reqPath string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// reject control bytes (NUL etc.) so they can't reach the git object spec
	if strings.ContainsAny(reqPath, "\x00\n\r") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p, repo, sha, ok := resolvePreview(w, token)
	if !ok {
		return
	}
	// If this branch defines a runnable environment, the path form cannot
	// serve it safely (absolute asset paths would escape the prefix); point
	// the visitor at the environment's own origin instead.
	if previewDomain != "" {
		if cfg := loadPreviewConfig(repo.DiskPath(), sha); cfg != nil && strings.TrimSpace(cfg.Run) != "" {
			http.Redirect(w, r, previewOrigin(p), http.StatusTemporaryRedirect)
			return
		}
	}
	serveStaticPreview(w, r, repo, p, sha, reqPath, true)
}

// serveStaticPreview serves files straight out of the commit tree. `sandbox`
// selects the opaque-origin CSP, which is required when the content shares
// GitGit's origin and unnecessary when it has a subdomain of its own.
func serveStaticPreview(w http.ResponseWriter, r *http.Request, repo *Repo, p *Preview, sha, reqPath string, sandbox bool) {
	if strings.ContainsAny(reqPath, "\x00\n\r") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dir := repo.DiskPath()
	clean := path.Clean("/" + reqPath)
	clean = strings.TrimPrefix(clean, "/")
	if strings.HasPrefix(clean, "..") || clean == "." {
		clean = ""
	}

	// trailing slash normalization keeps relative asset URLs working
	if clean != "" && pathKind(dir, sha, clean) == "tree" && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	candidates := []string{}
	switch {
	case clean == "":
		candidates = append(candidates, "index.html", "index.htm")
	case pathKind(dir, sha, clean) == "tree":
		candidates = append(candidates, clean+"/index.html", clean+"/index.htm")
	default:
		candidates = append(candidates, clean)
	}

	for _, cand := range candidates {
		content, _, err := readBlob(dir, sha, cand, 0)
		if err != nil {
			continue
		}
		writePreviewFile(w, cand, content, sha, sandbox)
		return
	}

	// SPA fallback: extensionless navigation goes to the root index.html
	if clean != "" && !strings.Contains(path.Base(clean), ".") &&
		strings.Contains(r.Header.Get("Accept"), "text/html") {
		if content, _, err := readBlob(dir, sha, "index.html", 0); err == nil {
			writePreviewFile(w, "index.html", content, sha, sandbox)
			return
		}
	}

	// no index: escaped directory listing so any repo is still browseable
	if clean == "" || pathKind(dir, sha, clean) == "tree" {
		writePreviewListing(w, repo, p, dir, sha, clean, sandbox)
		return
	}
	previewHeaders(w, "text/html; charset=utf-8", sandbox)
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("<!doctype html><meta charset=utf-8><title>404</title><p style='font-family:system-ui;padding:2em'>404 — not found in this preview.</p>"))
}

func previewHeaders(w http.ResponseWriter, contentType string, sandbox bool) {
	w.Header().Set("Content-Type", contentType)
	// Only path-served previews need the opaque-origin sandbox; subdomain
	// previews are already a separate origin and must behave like the real app.
	if sandbox {
		w.Header().Set("Content-Security-Policy", previewSandboxCSP)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func writePreviewFile(w http.ResponseWriter, name string, content []byte, sha string, sandbox bool) {
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(content)
	}
	if strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "charset") {
		ct += "; charset=utf-8"
	}
	previewHeaders(w, ct, sandbox)
	w.Header().Set("X-Preview-Commit", sha)
	w.Write(content)
}

func writePreviewListing(w http.ResponseWriter, repo *Repo, p *Preview, dir, sha, treePath string, sandbox bool) {
	entries, err := lsTree(dir, sha, treePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><meta name=viewport content='width=device-width,initial-scale=1'>")
	b.WriteString("<title>Preview · " + html.EscapeString(repo.FullName()) + "</title>")
	b.WriteString("<body style='font-family:system-ui;max-width:640px;margin:2rem auto;padding:0 1rem;color:#232b36'>")
	b.WriteString("<h2 style='font-weight:600'>" + html.EscapeString(repo.FullName()) + " <span style='color:#888'>@ " + html.EscapeString(p.Ref) + "</span></h2>")
	b.WriteString("<p style='color:#888;font-size:14px'>No <code>index.html</code> here — directory listing (" + html.EscapeString(sha[:10]) + ")</p><ul style='line-height:1.9;list-style:none;padding:0'>")
	if treePath != "" {
		b.WriteString("<li><a href='../'>..</a></li>")
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Type == "tree" && entries[j].Type != "tree" })
	for _, e := range entries {
		n := html.EscapeString(e.Name)
		if e.Type == "tree" {
			b.WriteString("<li>📁 <a href='" + n + "/'>" + n + "/</a></li>")
		} else {
			b.WriteString("<li>📄 <a href='" + n + "'>" + n + "</a></li>")
		}
	}
	b.WriteString("</ul>")
	previewHeaders(w, "text/html; charset=utf-8", sandbox)
	w.Write([]byte(b.String()))
}

// previewHosts proposes absolute base URLs a phone could reach: the request
// host first, then detected private LAN addresses (for QR codes scanned off
// a dev machine where "localhost" means nothing).
func previewHosts(r *http.Request) []string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	port := ""
	if _, pt, err := net.SplitHostPort(r.Host); err == nil {
		port = pt
	}
	hosts := []string{}
	if baseURL != "" {
		hosts = append(hosts, strings.TrimSuffix(baseURL, "/"))
	}
	hosts = append(hosts, scheme+"://"+r.Host)
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
			continue
		}
		if !ipnet.IP.IsPrivate() {
			continue
		}
		h := ipnet.IP.String()
		if port != "" {
			h = net.JoinHostPort(h, port)
		}
		cand := scheme + "://" + h
		dup := false
		for _, e := range hosts {
			if e == cand {
				dup = true
			}
		}
		if !dup {
			hosts = append(hosts, cand)
		}
	}
	return hosts
}
