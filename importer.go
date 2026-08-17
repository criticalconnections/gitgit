package main

// Import repositories from GitHub (or any other git host).
//
// Git data is mirrored with `git clone --mirror`, which brings every branch
// and tag across in one pass. Issues are optional and come from GitHub's REST
// API; pull requests are imported as issues, because their branches may not
// exist on the source and a half-real PR is worse than an honest record.
//
// Imports run in the background because a large mirror takes minutes; the UI
// polls the job for progress.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ImportJob struct {
	ID        int64
	UserID    int64
	Source    string
	RepoID    int64
	Status    string // running | done | failed
	Message   string
	Log       string
	CreatedAt int64
}

var (
	importMu   sync.Mutex
	importSeq  int64
	importJobs = map[int64]*ImportJob{}
)

func newImportJob(userID int64, source string) *ImportJob {
	importMu.Lock()
	defer importMu.Unlock()
	importSeq++
	j := &ImportJob{ID: importSeq, UserID: userID, Source: source, Status: "running", CreatedAt: now()}
	importJobs[j.ID] = j
	return j
}

func getImportJob(id int64) *ImportJob {
	importMu.Lock()
	defer importMu.Unlock()
	if j, ok := importJobs[id]; ok {
		cp := *j
		return &cp
	}
	return nil
}

func (j *ImportJob) log(format string, a ...any) {
	importMu.Lock()
	defer importMu.Unlock()
	if live, ok := importJobs[j.ID]; ok {
		live.Log += fmt.Sprintf(format, a...) + "\n"
	}
}

func (j *ImportJob) finish(status, message string, repoID int64) {
	importMu.Lock()
	defer importMu.Unlock()
	if live, ok := importJobs[j.ID]; ok {
		live.Status, live.Message, live.RepoID = status, message, repoID
	}
}

// ---------- source parsing ----------

type GitHubSource struct {
	Owner string
	Name  string
	Host  string // github.com, or a GitHub Enterprise host
}

func (s GitHubSource) CloneURL() string {
	return fmt.Sprintf("https://%s/%s/%s.git", s.Host, s.Owner, s.Name)
}

func (s GitHubSource) APIBase() string {
	if s.Host == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + s.Host + "/api/v3"
}

var ghShorthand = regexp.MustCompile(`^([\w.-]+)/([\w.-]+)$`)

// parseGitHubSource accepts "owner/repo", a browser URL, or a clone URL.
func parseGitHubSource(input string) (GitHubSource, error) {
	in := strings.TrimSpace(input)
	in = strings.TrimSuffix(in, "/")
	if in == "" {
		return GitHubSource{}, fmt.Errorf("a repository is required")
	}
	if m := ghShorthand.FindStringSubmatch(in); m != nil {
		return GitHubSource{Owner: m[1], Name: strings.TrimSuffix(m[2], ".git"), Host: "github.com"}, nil
	}
	// git@host:owner/repo.git
	if strings.HasPrefix(in, "git@") {
		rest := strings.TrimPrefix(in, "git@")
		host, path, ok := strings.Cut(rest, ":")
		if !ok {
			return GitHubSource{}, fmt.Errorf("could not parse %q", input)
		}
		parts := strings.Split(strings.TrimSuffix(path, ".git"), "/")
		if len(parts) != 2 {
			return GitHubSource{}, fmt.Errorf("expected owner/repo in %q", input)
		}
		return GitHubSource{Owner: parts[0], Name: parts[1], Host: host}, nil
	}
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil {
		return GitHubSource{}, fmt.Errorf("could not parse %q", input)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return GitHubSource{}, fmt.Errorf("expected a URL like https://github.com/owner/repo")
	}
	return GitHubSource{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git"), Host: u.Host}, nil
}

// ---------- import ----------

type ImportRequest struct {
	Source       string // owner/repo or URL
	TargetName   string // optional rename
	Private      bool
	Token        string // GitHub PAT, for private repos and issue import
	ImportIssues bool
}

// startImport validates the request, creates the repository, and mirrors it
// in the background.
func startImport(u *User, req ImportRequest) (*ImportJob, error) {
	src, err := parseGitHubSource(req.Source)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.TargetName)
	if name == "" {
		name = src.Name
	}
	if !validSlug(name) {
		return nil, fmt.Errorf("%q is not a valid repository name", name)
	}
	if existing, err := getRepo(u.Username, name); err == nil && existing != nil {
		return nil, fmt.Errorf("you already have a repository named %q", name)
	}

	repo, err := insertRepo(u.ID, name, fmt.Sprintf("Imported from %s/%s", src.Owner, src.Name), req.Private)
	if err != nil {
		return nil, err
	}

	job := newImportJob(u.ID, src.Owner+"/"+src.Name)
	go runImport(job, u, repo, src, req)
	return job, nil
}

func runImport(job *ImportJob, u *User, repo *Repo, src GitHubSource, req ImportRequest) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("import panic: %v", v)
			job.finish("failed", fmt.Sprint(v), 0)
		}
	}()

	job.log("Importing %s from %s", src.Owner+"/"+src.Name, src.Host)

	// Credentials go in the URL only for the duration of the clone, and are
	// never written to the repository config or logged.
	cloneURL := src.CloneURL()
	if req.Token != "" {
		cloneURL = fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", req.Token, src.Host, src.Owner, src.Name)
	}

	dest := repo.DiskPath()
	os.RemoveAll(dest)
	job.log("Mirroring git data (all branches and tags)…")

	cmd := exec.Command("git", "clone", "--mirror", cloneURL, dest)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GCM_INTERACTIVE=never")
	out, err := cmd.CombinedOutput()
	if err != nil {
		deleteRepoRows(repo.ID)
		os.RemoveAll(dest)
		msg := scrubToken(string(out), req.Token)
		if strings.Contains(msg, "Authentication failed") || strings.Contains(msg, "could not read Username") {
			msg = "authentication failed — a token with repo access is required for private repositories"
		} else if strings.Contains(msg, "not found") || strings.Contains(msg, "Repository not found") {
			msg = "repository not found (or private, and no token was supplied)"
		}
		job.log("clone failed: %s", strings.TrimSpace(msg))
		job.finish("failed", msg, 0)
		return
	}

	// A mirror clone keeps the source as "origin"; drop it so nothing here
	// ever pushes back to GitHub, and disable the mirror refspec.
	gitRun(dest, "remote", "remove", "origin")
	gitRun(dest, "config", "--unset-all", "remote.origin.fetch")

	// Adopt the source's default branch when it exists locally.
	if head, err := gitRun(dest, "symbolic-ref", "--short", "HEAD"); err == nil && head != "" {
		if branchExists(dest, head) {
			repo.DefaultBranch = head
			updateRepoMeta(repo)
		}
	}

	branches := listBranches(dest)
	tags := listTags(dest)
	job.log("Imported %d branch(es) and %d tag(s)", len(branches), len(tags))

	// CI for the default branch, if the imported code defines any.
	if sha, err := resolveCommit(dest, "refs/heads/"+repo.DefaultBranch); err == nil {
		enqueueCI(repo, sha, repo.DefaultBranch, "push")
	}

	if req.ImportIssues {
		n, err := importIssues(job, u, repo, src, req.Token)
		if err != nil {
			// The git data is already safely in place, so a failure here
			// degrades rather than fails the whole import.
			job.log("issue import stopped: %v", err)
		} else {
			job.log("Imported %d issue(s)", n)
		}
	}

	job.log("Done.")
	job.finish("done", fmt.Sprintf("imported %d branch(es), %d tag(s)", len(branches), len(tags)), repo.ID)
	log.Printf("import: %s -> %s", src.Owner+"/"+src.Name, repo.FullName())
}

// scrubToken makes sure a credential never reaches a log or an error message.
func scrubToken(s, token string) string {
	if token != "" {
		s = strings.ReplaceAll(s, token, "***")
	}
	return s
}

// ---------- issues ----------

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type ghIssue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	User        ghUser    `json:"user"`
	Labels      []ghLabel `json:"labels"`
	CreatedAt   string    `json:"created_at"`
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
	Comments int `json:"comments"`
}

func ghGet(token, url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gitgit-importer")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return fmt.Errorf("GitHub API rate limit reached — supply a token to raise it")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return fmt.Errorf("GitHub API %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// importIssues copies issues (and, as issues, pull requests) into the repo.
// Authorship is preserved in the body text rather than by inventing accounts.
func importIssues(job *ImportJob, u *User, repo *Repo, src GitHubSource, token string) (int, error) {
	job.log("Importing issues…")
	imported := 0
	labelIDs := map[string]int64{}

	for page := 1; page <= 10; page++ { // cap at 1000 issues
		var batch []ghIssue
		endpoint := fmt.Sprintf("%s/repos/%s/%s/issues?state=all&per_page=100&page=%d&direction=asc",
			src.APIBase(), src.Owner, src.Name, page)
		if err := ghGet(token, endpoint, &batch); err != nil {
			return imported, err
		}
		if len(batch) == 0 {
			break
		}
		for _, gi := range batch {
			kind := "issue"
			if gi.PullRequest != nil {
				kind = "pull request"
			}
			header := fmt.Sprintf("_Imported from [%s/%s#%d](%s) — originally opened by **@%s**",
				src.Owner, src.Name, gi.Number, gi.HTMLURL, gi.User.Login)
			if kind == "pull request" {
				header += " as a pull request"
			}
			header += "._\n\n"

			is, err := createIssue(repo.ID, u.ID, gi.Title, header+gi.Body)
			if err != nil {
				continue
			}
			for _, gl := range gi.Labels {
				id, ok := labelIDs[gl.Name]
				if !ok {
					color := "#" + strings.TrimPrefix(gl.Color, "#")
					if len(color) != 7 {
						color = "#1f6feb"
					}
					createLabel(repo.ID, gl.Name, color)
					for _, l := range listLabels(repo.ID) {
						if l.Name == gl.Name {
							id = l.ID
							labelIDs[gl.Name] = l.ID
						}
					}
				}
				if id != 0 {
					setItemLabel("issue", is.ID, id, true)
				}
			}
			if gi.State == "closed" {
				is.State = "closed"
				is.ClosedAt = nullNow()
				saveIssue(is)
			}
			imported++
		}
		if len(batch) < 100 {
			break
		}
	}
	return imported, nil
}
