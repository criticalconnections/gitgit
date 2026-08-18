package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// handleGitSmartHTTP serves the git smart HTTP protocol:
//
//	GET  /{owner}/{repo}.git/info/refs?service=git-upload-pack|git-receive-pack
//	POST /{owner}/{repo}.git/git-upload-pack
//	POST /{owner}/{repo}.git/git-receive-pack
func handleGitSmartHTTP(w http.ResponseWriter, r *http.Request, repo *Repo, rest string) {
	switch {
	case r.Method == http.MethodGet && rest == "info/refs":
		service := r.URL.Query().Get("service")
		if service != "git-upload-pack" && service != "git-receive-pack" {
			http.Error(w, "smart HTTP only", http.StatusForbidden)
			return
		}
		if !authorizeGit(w, r, repo, service) {
			return
		}
		advertiseRefs(w, r, repo, service)
	case r.Method == http.MethodPost && (rest == "git-upload-pack" || rest == "git-receive-pack"):
		service := rest
		if !authorizeGit(w, r, repo, service) {
			return
		}
		serviceRPC(w, r, repo, service)
	default:
		http.NotFound(w, r)
	}
}

// authorizeGit enforces read access for fetches and write access for pushes.
func authorizeGit(w http.ResponseWriter, r *http.Request, repo *Repo, service string) bool {
	u := basicAuthUser(r)
	if service == "git-receive-pack" {
		if u == nil {
			requireBasicAuth(w)
			return false
		}
		if !canWrite(u, repo) {
			http.Error(w, "write access denied", http.StatusForbidden)
			return false
		}
		return true
	}
	if repo.IsPrivate {
		if u == nil {
			requireBasicAuth(w)
			return false
		}
		if !canRead(u, repo) {
			http.Error(w, "access denied", http.StatusForbidden)
			return false
		}
	}
	return true
}

func pktLine(s string) string {
	return fmt.Sprintf("%04x%s", len(s)+4, s)
}

func advertiseRefs(w http.ResponseWriter, r *http.Request, repo *Repo, service string) {
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, pktLine("# service="+service+"\n"))
	fmt.Fprint(w, "0000")

	cmd := exec.Command("git", strings.TrimPrefix(service, "git-"), "--stateless-rpc", "--advertise-refs", ".")
	cmd.Dir = repo.DiskPath()
	cmd.Env = gitProtocolEnv(r)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("git %s advertise-refs %s: %v", service, repo.FullName(), err)
	}
}

func serviceRPC(w http.ResponseWriter, r *http.Request, repo *Repo, service string) {
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "bad gzip body", http.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}

	dir := repo.DiskPath()
	isPush := service == "git-receive-pack"
	var before map[string]string
	if isPush {
		before = refsSnapshot(dir)
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-result", service))
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.Command("git", strings.TrimPrefix(service, "git-"), "--stateless-rpc", ".")
	cmd.Dir = dir
	cmd.Env = gitProtocolEnv(r)
	cmd.Stdin = body
	cmd.Stdout = &flushWriter{w: w}
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("git %s %s: %v", service, repo.FullName(), err)
		return
	}

	if isPush {
		go processPush(repo, basicAuthUser(r), before, refsSnapshot(dir))
	}
}

func gitProtocolEnv(r *http.Request) []string {
	env := os.Environ()
	if p := r.Header.Get("Git-Protocol"); p != "" {
		env = append(env, "GIT_PROTOCOL="+p)
	}
	return env
}

type flushWriter struct{ w http.ResponseWriter }

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

// RefUpdate describes one branch change from a push.
type RefUpdate struct {
	Branch  string
	OldSHA  string
	NewSHA  string
	Deleted bool
	Created bool
}

// processPush reacts to a completed push: updates default branch on first
// push, bumps affected PRs, enqueues CI, and fires webhooks.
func processPush(repo *Repo, pusher *User, before, after map[string]string) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("processPush panic: %v", v)
		}
	}()
	var updates []RefUpdate
	for ref, sha := range after {
		branch := strings.TrimPrefix(ref, "refs/heads/")
		if old, ok := before[ref]; !ok {
			updates = append(updates, RefUpdate{Branch: branch, OldSHA: "", NewSHA: sha, Created: true})
		} else if old != sha {
			updates = append(updates, RefUpdate{Branch: branch, OldSHA: old, NewSHA: sha})
		}
	}
	for ref, sha := range before {
		if _, ok := after[ref]; !ok {
			updates = append(updates, RefUpdate{Branch: strings.TrimPrefix(ref, "refs/heads/"), OldSHA: sha, Deleted: true})
		}
	}
	if len(updates) == 0 {
		return
	}

	// First push to an empty repo: adopt the pushed branch as default.
	if len(before) == 0 {
		adopt := repo.DefaultBranch
		if _, ok := after["refs/heads/"+adopt]; !ok {
			// prefer main, then master, then the first pushed branch
			for _, cand := range []string{"main", "master"} {
				if _, ok := after["refs/heads/"+cand]; ok {
					adopt = cand
					break
				}
			}
			if _, ok := after["refs/heads/"+adopt]; !ok {
				for _, u := range updates {
					if !u.Deleted {
						adopt = u.Branch
						break
					}
				}
			}
			repo.DefaultBranch = adopt
			updateRepoMeta(repo)
		}
		setHEADBranch(repo.DiskPath(), repo.DefaultBranch)
	}

	pusherName := "someone"
	if pusher != nil {
		pusherName = pusher.Username
	}

	for _, up := range updates {
		log.Printf("push %s: %s %s -> %s", repo.FullName(), up.Branch, short(up.OldSHA), short(up.NewSHA))
		if up.Deleted {
			stopEnvsForRef(repo.ID, up.Branch)
			continue
		}
		// Bump any open PR whose head branch moved, and note the update.
		for _, pr := range openPullsWithHead(repo.ID, up.Branch) {
			touchPull(pr.ID)
			if pusher != nil && !up.Created {
				addComment("pull", pr.ID, pusher.ID,
					fmt.Sprintf("%s pushed new commits to `%s` (now `%s`)", pusherName, up.Branch, short(up.NewSHA)), true)
			}
		}
		// CI for the new tip (push event, or pull_request if the branch has an open PR).
		event := "push"
		if len(openPullsWithHead(repo.ID, up.Branch)) > 0 {
			event = "pull_request"
		}
		enqueueCI(repo, up.NewSHA, up.Branch, event)

		// Environments configured to follow this branch ship the new commit.
		autoDeployOnPush(repo, pusher, up.Branch, up.NewSHA)

		// A Preview Environment pinned to the old commit is now stale; drop it
		// so the next visit rebuilds at the new tip.
		if p := previewByRepoRef(repo.ID, up.Branch); p != nil {
			if e := envByPreview(p.ID); e != nil && e.CommitSHA != up.NewSHA {
				stopPreviewEnv(e.ID, "superseded by a new push")
			}
		}
	}

	fireWebhooks(repo, "push", map[string]any{
		"repository": repo.FullName(),
		"pusher":     pusherName,
		"pushed_at":  time.Now().UTC().Format(time.RFC3339),
		"updates":    updates,
	})
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	if sha == "" {
		return "∅"
	}
	return sha
}
