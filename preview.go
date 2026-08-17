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
const previewSandboxCSP = "sandbox allow-scripts allow-forms allow-modals allow-popups allow-pointer-lock"

// servePreview handles GET /p/{token}/{path...}.
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
	p := previewByToken(token)
	if p == nil {
		http.Error(w, "preview not found or expired", http.StatusNotFound)
		return
	}
	repo, err := getRepoByID(p.RepoID)
	if err != nil {
		http.Error(w, "preview not found or expired", http.StatusNotFound)
		return
	}
	// A preview is only alive while its creator still has write access — so a
	// de-provisioned author's previews stop serving immediately, and a repo
	// turned private (which purges previews on transition) can't be re-exposed
	// by a stale link belonging to someone who lost access.
	if creator, err := getUserByID(p.CreatedBy); err != nil || !canWrite(creator, repo) {
		http.Error(w, "preview no longer available", http.StatusNotFound)
		return
	}
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, "refs/heads/"+p.Ref)
	if err != nil {
		if sha, err = resolveCommit(dir, p.Ref); err != nil { // pinned sha / tag
			http.Error(w, "preview ref no longer exists", http.StatusNotFound)
			return
		}
	}

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
		writePreviewFile(w, cand, content, sha)
		return
	}

	// SPA fallback: extensionless navigation goes to the root index.html
	if clean != "" && !strings.Contains(path.Base(clean), ".") &&
		strings.Contains(r.Header.Get("Accept"), "text/html") {
		if content, _, err := readBlob(dir, sha, "index.html", 0); err == nil {
			writePreviewFile(w, "index.html", content, sha)
			return
		}
	}

	// no index: escaped directory listing so any repo is still browseable
	if clean == "" || pathKind(dir, sha, clean) == "tree" {
		writePreviewListing(w, repo, p, dir, sha, clean)
		return
	}
	previewHeaders(w, "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("<!doctype html><meta charset=utf-8><title>404</title><p style='font-family:system-ui;padding:2em'>404 — not found in this preview.</p>"))
}

func previewHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", previewSandboxCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func writePreviewFile(w http.ResponseWriter, name string, content []byte, sha string) {
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(content)
	}
	if strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "charset") {
		ct += "; charset=utf-8"
	}
	previewHeaders(w, ct)
	w.Header().Set("X-Preview-Commit", sha)
	w.Write(content)
}

func writePreviewListing(w http.ResponseWriter, repo *Repo, p *Preview, dir, sha, treePath string) {
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
	previewHeaders(w, "text/html; charset=utf-8")
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
