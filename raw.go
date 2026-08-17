package main

import (
	"log"
	"net/http"
	"os/exec"
	"strings"
)

// splitRefPath separates "ref/sub/path" where ref may itself contain slashes.
// The longest matching branch name wins; otherwise the first segment is
// treated as the ref (sha, tag, etc.).
func splitRefPath(repo *Repo, rest string) (string, string) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return repo.DefaultBranch, ""
	}
	segs := strings.Split(rest, "/")
	branches := listBranches(repo.DiskPath())
	for i := len(segs); i >= 1; i-- {
		cand := strings.Join(segs[:i], "/")
		for _, b := range branches {
			if b.Name == cand {
				return cand, strings.Join(segs[i:], "/")
			}
		}
	}
	return segs[0], strings.Join(segs[1:], "/")
}

// serveArchive streams `git archive` output for a ref, GitHub-style.
// GET /{owner}/{repo}/archive/{ref}.zip | .tar.gz
// (pattern adapted from Gitea's modules/git/archive.go, MIT — see NOTICE)
func serveArchive(w http.ResponseWriter, r *http.Request, owner, name, sub string) {
	repo, err := getRepo(owner, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := currentUser(r)
	if u == nil {
		u = basicAuthUser(r)
	}
	if !canRead(u, repo) {
		http.NotFound(w, r)
		return
	}
	format, refPart := "", ""
	switch {
	case strings.HasSuffix(sub, ".tar.gz"):
		format, refPart = "tar.gz", strings.TrimSuffix(sub, ".tar.gz")
	case strings.HasSuffix(sub, ".zip"):
		format, refPart = "zip", strings.TrimSuffix(sub, ".zip")
	default:
		http.Error(w, "supported formats: .zip, .tar.gz", http.StatusBadRequest)
		return
	}
	ref, _ := splitRefPath(repo, refPart)
	if ref == "" || strings.HasPrefix(ref, "-") {
		http.NotFound(w, r)
		return
	}
	sha, err := resolveCommit(repo.DiskPath(), ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	safeRef := strings.ReplaceAll(ref, "/", "-")
	filename := repo.Name + "-" + safeRef + "." + format
	ct := "application/zip"
	if format == "tar.gz" {
		ct = "application/gzip"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	cmd := exec.Command("git", "archive", "--format="+format, "--prefix="+repo.Name+"-"+safeRef+"/", sha)
	cmd.Dir = repo.DiskPath()
	cmd.Stdout = w
	if err := cmd.Run(); err != nil {
		log.Printf("archive %s@%s: %v", repo.FullName(), ref, err)
	}
}

// serveRaw streams a file at a ref, for downloads and README images.
// GET /{owner}/{repo}/raw/{ref}/{path}
func serveRaw(w http.ResponseWriter, r *http.Request, owner, name, sub string) {
	repo, err := getRepo(owner, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := currentUser(r)
	if u == nil {
		u = basicAuthUser(r)
	}
	if !canRead(u, repo) {
		http.NotFound(w, r)
		return
	}
	ref, path := splitRefPath(repo, sub)
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	content, _, err := readBlob(dir, sha, path, 0)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	ct := http.DetectContentType(content)
	if strings.HasPrefix(ct, "text/html") {
		ct = "text/plain; charset=utf-8" // never serve HTML from raw
	}
	w.Header().Set("Content-Type", ct)
	w.Write(content)
}
