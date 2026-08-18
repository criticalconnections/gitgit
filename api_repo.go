package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// handleAPIRepo routes /api/v1/repos/{owner}/{repo}/... after read access is
// established.
func handleAPIRepo(c *apiCtx, repo *Repo, rest []string) {
	get := c.r.Method == http.MethodGet
	post := c.r.Method == http.MethodPost

	switch {
	case len(rest) == 0:
		handleAPIRepoRoot(c, repo)
	case rest[0] == "star" && post:
		if !c.requireUser() {
			return
		}
		var req struct{ On bool }
		if !c.decode(&req) {
			return
		}
		starRepo(c.u.ID, repo.ID, req.On)
		c.out(200, map[string]any{"starred": req.On, "stars": starCount(repo.ID)})
	case rest[0] == "tree" && get:
		apiTree(c, repo)
	case rest[0] == "blob" && get:
		apiBlob(c, repo)
	case rest[0] == "commits" && get:
		apiCommits(c, repo)
	case rest[0] == "commit" && len(rest) == 2 && get:
		apiCommit(c, repo, rest[1])
	case rest[0] == "branches" && len(rest) == 1 && get:
		apiBranches(c, repo)
	case rest[0] == "tags" && len(rest) == 1 && get:
		rows := []map[string]any{}
		for _, t := range listTags(repo.DiskPath()) {
			rows = append(rows, map[string]any{"name": t.Name, "sha": t.SHA, "when": t.When, "subject": t.Subject})
		}
		c.out(200, rows)
	case rest[0] == "branches" && len(rest) >= 2 && c.r.Method == http.MethodDelete:
		apiDeleteBranch(c, repo, strings.Join(rest[1:], "/"))
	case rest[0] == "compare" && get:
		apiCompare(c, repo)
	case rest[0] == "pulls":
		handleAPIPulls(c, repo, rest[1:])
	case rest[0] == "stacks" && get:
		apiStacks(c, repo)
	case rest[0] == "issues":
		handleAPIIssues(c, repo, rest[1:])
	case rest[0] == "labels":
		apiLabels(c, repo, rest[1:])
	case rest[0] == "collaborators":
		apiCollaborators(c, repo, rest[1:])
	case rest[0] == "webhooks":
		apiWebhooks(c, repo, rest[1:])
	case rest[0] == "ci":
		apiCI(c, repo, rest[1:])
	case rest[0] == "previews":
		apiPreviews(c, repo, rest[1:])
	default:
		c.err(404, "unknown endpoint")
	}
}

// ---------- import from GitHub ----------

func handleAPIImport(c *apiCtx, rest []string) {
	if !c.requireUser() {
		return
	}
	switch {
	case len(rest) == 1 && rest[0] == "zip" && c.r.Method == http.MethodPost:
		handleAPIImportZip(c)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		var req struct {
			Source       string `json:"source"`
			Name         string `json:"name"`
			Private      bool   `json:"private"`
			Token        string `json:"token"`
			ImportIssues bool   `json:"import_issues"`
		}
		if !c.decode(&req) {
			return
		}
		job, err := startImport(c.u, ImportRequest{
			Source: req.Source, TargetName: req.Name, Private: req.Private,
			Token: req.Token, ImportIssues: req.ImportIssues,
		})
		if err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(202, importJobJSON(job))
	case len(rest) == 1 && c.r.Method == http.MethodGet:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		job := getImportJob(id)
		if job == nil || job.UserID != c.u.ID {
			c.err(404, "import not found")
			return
		}
		c.out(200, importJobJSON(job))
	default:
		c.err(404, "unknown endpoint")
	}
}

// handleAPIImportZip receives a multipart upload and turns it into a
// repository. The archive is spooled to disk rather than held in memory.
func handleAPIImportZip(c *apiCtx) {
	c.r.Body = http.MaxBytesReader(c.w, c.r.Body, maxZipUpload)
	if err := c.r.ParseMultipartForm(32 << 20); err != nil {
		c.err(413, fmt.Sprintf("upload too large or malformed (limit %d MB)", maxZipUpload>>20))
		return
	}
	defer c.r.MultipartForm.RemoveAll()

	file, header, err := c.r.FormFile("file")
	if err != nil {
		c.err(422, "a .zip file is required")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		c.err(422, "only .zip archives are supported")
		return
	}

	// zip.NewReader needs io.ReaderAt; multipart gives that for spooled files
	// but not for small in-memory ones, so normalize through a temp file.
	ra, ok := file.(io.ReaderAt)
	size := header.Size
	if !ok {
		tmp, terr := os.CreateTemp(dataDir, "upload-*.zip")
		if terr != nil {
			c.err(500, terr.Error())
			return
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		n, cerr := io.Copy(tmp, file)
		if cerr != nil {
			c.err(500, cerr.Error())
			return
		}
		ra, size = tmp, n
	}

	job, repo, err := startZipImport(c.u, ra, size, ZipImportRequest{
		Name:     c.r.FormValue("name"),
		Private:  c.r.FormValue("private") == "true",
		Filename: header.Filename,
	})
	if err != nil {
		c.err(422, err.Error())
		return
	}
	m := importJobJSON(job)
	if repo != nil {
		m["repo"] = repo.FullName()
	}
	c.out(201, m)
}

func importJobJSON(j *ImportJob) map[string]any {
	m := map[string]any{
		"id": j.ID, "source": j.Source, "status": j.Status,
		"message": j.Message, "log": j.Log, "created_at": j.CreatedAt,
	}
	if j.RepoID != 0 {
		if r, err := getRepoByID(j.RepoID); err == nil {
			m["repo"] = r.FullName()
		}
	}
	return m
}

// ---------- branch previews ----------

func previewJSON(c *apiCtx, repo *Repo, p *Preview) map[string]any {
	m := map[string]any{
		"id": p.ID, "ref": p.Ref, "token": p.Token,
		"path":       "/p/" + p.Token + "/",
		"created_at": p.CreatedAt, "expires_at": p.ExpiresAt,
		"hosts": previewHosts(c.r),
	}
	sha := ""
	if s, err := resolveCommit(repo.DiskPath(), "refs/heads/"+p.Ref); err == nil {
		sha = s
		m["sha"] = s
	}
	// Preview Environment: its own origin when a preview domain is configured
	if host := previewHostFor(p); host != "" {
		m["host"] = host
		m["url"] = previewOrigin(p)
	}
	if sha != "" {
		cfg, det := previewPlan(repo.DiskPath(), sha)
		m["runnable"] = cfg != nil
		if det != nil {
			// a guess, not a commitment: the UI shows it for approval
			m["detected"] = map[string]any{
				"name": det.Name, "why": det.Why, "yaml": det.yamlText(),
				"build": det.Cfg.Build, "run": det.Cfg.Run, "static": det.Cfg.Static,
				"approved": p.EnvOK,
			}
		}
	}
	if e := envByPreview(p.ID); e != nil {
		m["env"] = envJSON(e)
	}
	return m
}

func envJSON(e *PreviewEnv) map[string]any {
	return map[string]any{
		"id": e.ID, "status": e.Status, "commit": e.CommitSHA, "ref": e.Ref,
		"message": e.Message, "created_at": e.CreatedAt, "started_at": e.StartedAt,
		"last_used_at": e.LastUsedAt, "expires_at": e.ExpiresAt,
	}
}

func apiPreviews(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, p := range listPreviews(repo.ID) {
			out = append(out, previewJSON(c, repo, p))
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		var req struct{ Ref string }
		if !c.decode(&req) {
			return
		}
		if !branchExists(repo.DiskPath(), req.Ref) {
			c.err(422, "branch does not exist")
			return
		}
		p, err := createPreview(repo.ID, c.u.ID, req.Ref)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		c.out(201, previewJSON(c, repo, p))
	case len(rest) == 1 && c.r.Method == http.MethodDelete:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		if e := envByPreview(id); e != nil {
			stopPreviewEnv(e.ID, "preview revoked")
		}
		deletePreview(repo.ID, id)
		c.out(200, map[string]bool{"ok": true})

	// ---- Preview Environment control ----
	case len(rest) == 2 && rest[1] == "env" && c.r.Method == http.MethodGet:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		e := envByPreview(id)
		if e == nil {
			c.out(200, map[string]any{"status": "none"})
			return
		}
		m := envJSON(e)
		m["log"] = envLogTail(e.ID, 20000)
		c.out(200, m)
	case len(rest) == 2 && rest[1] == "env" && c.r.Method == http.MethodDelete:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		if e := envByPreview(id); e != nil {
			stopPreviewEnv(e.ID, "stopped by "+c.u.Username)
		}
		c.out(200, map[string]bool{"ok": true})
	case len(rest) == 3 && rest[1] == "env" && rest[2] == "restart" && c.r.Method == http.MethodPost:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		p := previewByID(id)
		if p == nil || p.RepoID != repo.ID {
			c.err(404, "preview not found")
			return
		}
		if e := envByPreview(id); e != nil {
			stopPreviewEnv(e.ID, "restarting")
			db.Exec("DELETE FROM preview_envs WHERE id = ?", e.ID)
		}
		sha, err := resolveCommit(repo.DiskPath(), "refs/heads/"+p.Ref)
		if err != nil {
			c.err(422, "branch no longer exists")
			return
		}
		cfg, det := previewPlan(repo.DiskPath(), sha)
		if cfg == nil {
			c.err(422, "nothing to build for this branch — add .gitgit/preview.yml to say how it builds and runs")
			return
		}
		// Pressing this button IS the approval for a detected configuration:
		// only write access reaches here. Remember it so later commits on the
		// branch rebuild without asking again.
		if !p.EnvOK {
			db.Exec("UPDATE previews SET env_ok = 1 WHERE id = ?", p.ID)
			p.EnvOK = true
		}
		e := startPreviewEnv(repo, p, sha, cfg, det != nil)
		if e == nil {
			c.err(503, "no capacity for another preview environment right now")
			return
		}
		c.out(201, envJSON(e))
	default:
		c.err(404, "unknown endpoint")
	}
}

func handleAPIRepoRoot(c *apiCtx, repo *Repo) {
	switch c.r.Method {
	case http.MethodGet:
		m := repoJSON(repo, c.u)
		m["clone_url"] = serverBase(c.r) + "/" + repo.FullName() + ".git"
		branches := []map[string]any{}
		for _, b := range listBranches(repo.DiskPath()) {
			branches = append(branches, map[string]any{"name": b.Name, "sha": b.SHA})
		}
		m["branches"] = branches
		c.out(200, m)
	case http.MethodPatch:
		if c.u == nil || !canAdmin(c.u, repo) {
			c.err(403, "admin access required")
			return
		}
		var req struct {
			Description         *string `json:"description"`
			DefaultBranch       *string `json:"default_branch"`
			Private             *bool   `json:"private"`
			AllowMerge          *bool   `json:"allow_merge"`
			AllowSquash         *bool   `json:"allow_squash"`
			AllowRebase         *bool   `json:"allow_rebase"`
			DeleteBranchOnMerge *bool   `json:"delete_branch_on_merge"`
			RequireCIPass       *bool   `json:"require_ci_pass"`
			RequireApprovals    *int64  `json:"require_approvals"`
		}
		if !c.decode(&req) {
			return
		}
		if req.Description != nil {
			repo.Description = strings.TrimSpace(*req.Description)
		}
		if req.DefaultBranch != nil && *req.DefaultBranch != "" {
			if !branchExists(repo.DiskPath(), *req.DefaultBranch) {
				c.err(422, "branch does not exist")
				return
			}
			repo.DefaultBranch = *req.DefaultBranch
			setHEADBranch(repo.DiskPath(), repo.DefaultBranch)
		}
		if req.Private != nil {
			wasPublic := !repo.IsPrivate
			repo.IsPrivate = *req.Private
			// turning a repo private revokes existing preview links, which are
			// unauthenticated capability URLs that would otherwise keep serving
			if *req.Private && wasPublic {
				db.Exec("DELETE FROM previews WHERE repo_id = ?", repo.ID)
			}
		}
		if req.AllowMerge != nil {
			repo.AllowMerge = *req.AllowMerge
		}
		if req.AllowSquash != nil {
			repo.AllowSquash = *req.AllowSquash
		}
		if req.AllowRebase != nil {
			repo.AllowRebase = *req.AllowRebase
		}
		if !repo.AllowMerge && !repo.AllowSquash && !repo.AllowRebase {
			repo.AllowMerge = true
		}
		if req.DeleteBranchOnMerge != nil {
			repo.DeleteBranchOnMerge = *req.DeleteBranchOnMerge
		}
		if req.RequireCIPass != nil {
			repo.RequireCIPass = *req.RequireCIPass
		}
		if req.RequireApprovals != nil && *req.RequireApprovals >= 0 {
			repo.RequireApprovals = *req.RequireApprovals
		}
		if err := updateRepoMeta(repo); err != nil {
			c.err(500, err.Error())
			return
		}
		c.out(200, repoJSON(repo, c.u))
	case http.MethodDelete:
		if c.u == nil || !canAdmin(c.u, repo) {
			c.err(403, "admin access required")
			return
		}
		diskPath := repo.DiskPath()
		closeRepoBatch(diskPath)
		deleteRepoRows(repo.ID)
		os.RemoveAll(diskPath)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(405, "method not allowed")
	}
}

// ---------- code browsing ----------

func refPathParams(c *apiCtx, repo *Repo) (ref, path string) {
	ref = c.r.URL.Query().Get("ref")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	path = strings.Trim(c.r.URL.Query().Get("path"), "/")
	return
}

func apiTree(c *apiCtx, repo *Repo) {
	dir := repo.DiskPath()
	if isEmptyRepo(dir) {
		c.out(200, map[string]any{"empty": true, "entries": []any{}})
		return
	}
	ref, path := refPathParams(c, repo)
	sha, err := resolveCommit(dir, ref)
	if err != nil {
		c.err(404, "ref not found: "+ref)
		return
	}
	kind := pathKind(dir, sha, path)
	if kind != "tree" {
		c.err(404, "not a directory: "+path)
		return
	}
	entries, err := lsTree(dir, sha, path)
	if err != nil {
		c.err(500, err.Error())
		return
	}
	rows := []map[string]any{}
	for _, e := range entries {
		rows = append(rows, map[string]any{
			"name": e.Name, "path": e.Path, "type": e.Type, "size": e.Size, "mode": e.Mode,
		})
	}
	resp := map[string]any{"ref": ref, "sha": sha, "path": path, "entries": rows, "empty": false}
	if commits, err := listCommits(dir, sha, path, 1, 0); err == nil && len(commits) > 0 {
		resp["latest"] = commitJSON(commits[0])
		if s, run := ciStatusForSHA(repo.ID, commits[0].SHA); s != "" {
			resp["ci_status"] = s
			resp["ci_run"] = run.Number
		}
	}
	if count, err := gitRun(dir, "rev-list", "--count", sha); err == nil {
		resp["commit_count"], _ = strconv.Atoi(count)
	}
	for _, e := range entries {
		if e.Type == "blob" && strings.EqualFold(e.Name, "README.md") {
			if content, _, err := readBlob(dir, sha, e.Path, 1<<20); err == nil {
				resp["readme"] = string(renderMarkdown(string(content)))
				resp["readme_path"] = e.Path
			}
			break
		}
	}
	c.out(200, resp)
}

func apiBlob(c *apiCtx, repo *Repo) {
	dir := repo.DiskPath()
	ref, path := refPathParams(c, repo)
	sha, err := resolveCommit(dir, ref)
	if err != nil {
		c.err(404, "ref not found: "+ref)
		return
	}
	content, truncated, err := readBlob(dir, sha, path, 1<<20)
	if err != nil {
		c.err(404, "file not found: "+path)
		return
	}
	isBinary := !utf8.Valid(content) || strings.ContainsRune(string(content[:min(len(content), 8000)]), 0)
	resp := map[string]any{
		"ref": ref, "sha": sha, "path": path, "size": len(content),
		"binary": isBinary, "truncated": truncated,
		"raw_url": "/" + repo.FullName() + "/raw/" + ref + "/" + path,
	}
	if !isBinary {
		resp["content"] = string(content)
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			resp["rendered"] = string(renderMarkdown(string(content)))
		}
	}
	c.out(200, resp)
}

func apiCommits(c *apiCtx, repo *Repo) {
	dir := repo.DiskPath()
	ref, path := refPathParams(c, repo)
	page, _ := strconv.Atoi(c.r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const perPage = 30
	commits, err := listCommits(dir, ref, path, perPage+1, (page-1)*perPage)
	if err != nil {
		c.err(404, "cannot list commits for "+ref)
		return
	}
	hasNext := len(commits) > perPage
	if hasNext {
		commits = commits[:perPage]
	}
	rows := []map[string]any{}
	for _, ci := range commits {
		m := commitJSON(ci)
		if s, run := ciStatusForSHA(repo.ID, ci.SHA); s != "" {
			m["ci_status"] = s
			m["ci_run"] = run.Number
		}
		rows = append(rows, m)
	}
	c.out(200, map[string]any{"ref": ref, "path": path, "page": page, "has_next": hasNext, "commits": rows})
}

func apiCommit(c *apiCtx, repo *Repo, shaArg string) {
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, shaArg)
	if err != nil {
		c.err(404, "commit not found")
		return
	}
	commit, err := getCommit(dir, sha)
	if err != nil {
		c.err(404, "commit not found")
		return
	}
	patch, err := commitPatch(dir, sha)
	if err != nil {
		c.err(500, err.Error())
		return
	}
	resp := map[string]any{"commit": commitJSON(commit), "diff": diffJSON(parseUnifiedDiff(patch))}
	if s, run := ciStatusForSHA(repo.ID, sha); s != "" {
		resp["ci_status"] = s
		resp["ci_run"] = run.Number
	}
	c.out(200, resp)
}

func apiBranches(c *apiCtx, repo *Repo) {
	dir := repo.DiskPath()
	rows := []map[string]any{}
	for _, b := range listBranches(dir) {
		row := map[string]any{
			"name": b.Name, "sha": b.SHA, "when": b.When, "subject": b.Subject,
			"author_name": b.AuthorName, "default": b.Name == repo.DefaultBranch,
		}
		if b.Name != repo.DefaultBranch {
			ahead, behind := aheadBehind(dir, repo.DefaultBranch, b.Name)
			row["ahead"], row["behind"] = ahead, behind
			if prs := openPullsWithHead(repo.ID, b.Name); len(prs) > 0 {
				row["pull"] = prs[0].Number
			}
		}
		rows = append(rows, row)
	}
	c.out(200, rows)
}

func apiDeleteBranch(c *apiCtx, repo *Repo, name string) {
	if c.u == nil || !canWrite(c.u, repo) {
		c.err(403, "write access required")
		return
	}
	if name == repo.DefaultBranch {
		c.err(422, "cannot delete the default branch")
		return
	}
	if prs := openPullsWithHead(repo.ID, name); len(prs) > 0 {
		c.err(422, "branch has an open pull request")
		return
	}
	if err := deleteBranch(repo.DiskPath(), name); err != nil {
		c.err(422, err.Error())
		return
	}
	c.out(200, map[string]bool{"ok": true})
}

func apiCompare(c *apiCtx, repo *Repo) {
	dir := repo.DiskPath()
	base := c.r.URL.Query().Get("base")
	head := c.r.URL.Query().Get("head")
	if base == "" {
		base = repo.DefaultBranch
	}
	resp := map[string]any{"base": base, "head": head}
	if head == "" {
		c.out(200, resp)
		return
	}
	baseSHA, err1 := resolveCommit(dir, base)
	headSHA, err2 := resolveCommit(dir, head)
	if err1 != nil || err2 != nil {
		c.err(404, "unknown branch in comparison")
		return
	}
	commits, _ := commitRange(dir, baseSHA, headSHA)
	raw, _ := diffRange(dir, base, head, true)
	ahead, behind := aheadBehind(dir, base, head)
	rows := []map[string]any{}
	for _, ci := range commits {
		rows = append(rows, commitJSON(ci))
	}
	resp["commits"] = rows
	resp["diff"] = diffJSON(parseUnifiedDiff(raw))
	resp["ahead"], resp["behind"] = ahead, behind
	for _, pr := range openPullsWithHead(repo.ID, head) {
		if pr.BaseBranch == base {
			resp["existing_pull"] = pr.Number
			break
		}
	}
	c.out(200, resp)
}

// ---------- labels / collaborators / webhooks ----------

func apiLabels(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, l := range listLabels(repo.ID) {
			out = append(out, labelJSON(l))
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		var req struct{ Name, Color string }
		if !c.decode(&req) {
			return
		}
		if err := createLabel(repo.ID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Color)); err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(201, map[string]bool{"ok": true})
	case len(rest) == 1 && c.r.Method == http.MethodDelete:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		deleteLabel(repo.ID, id)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(404, "unknown endpoint")
	}
}

func apiCollaborators(c *apiCtx, repo *Repo, rest []string) {
	if c.u == nil || !canAdmin(c.u, repo) {
		c.err(403, "admin access required")
		return
	}
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, col := range listCollaborators(repo.ID) {
			out = append(out, map[string]any{"user": userJSON(col.User), "role": col.Role})
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		var req struct{ Username, Role string }
		if !c.decode(&req) {
			return
		}
		u, err := getUserByName(strings.TrimSpace(req.Username))
		if err != nil {
			c.err(404, "no such user")
			return
		}
		if u.ID == repo.OwnerID {
			c.err(422, "user owns this repository")
			return
		}
		addCollaborator(repo.ID, u.ID, req.Role)
		c.out(201, map[string]bool{"ok": true})
	case len(rest) == 1 && c.r.Method == http.MethodDelete:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		removeCollaborator(repo.ID, id)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(404, "unknown endpoint")
	}
}

func apiWebhooks(c *apiCtx, repo *Repo, rest []string) {
	if c.u == nil || !canAdmin(c.u, repo) {
		c.err(403, "admin access required")
		return
	}
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, h := range listWebhooks(repo.ID) {
			out = append(out, map[string]any{
				"id": h.ID, "url": h.URL, "events": h.Events, "active": h.Active,
				"has_secret": h.Secret != "", "created_at": h.CreatedAt,
			})
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		var req struct{ URL, Secret, Events string }
		if !c.decode(&req) {
			return
		}
		if err := createWebhook(repo.ID, strings.TrimSpace(req.URL), req.Secret, strings.TrimSpace(req.Events)); err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(201, map[string]bool{"ok": true})
	case len(rest) == 1 && c.r.Method == http.MethodDelete:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		deleteWebhook(repo.ID, id)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(404, "unknown endpoint")
	}
}

// ---------- CI ----------

func apiCI(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 1 && rest[0] == "runs" && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, run := range listRuns(repo.ID, 100) {
			out = append(out, runJSON(run, false))
		}
		c.out(200, out)
	case len(rest) == 2 && rest[0] == "runs" && c.r.Method == http.MethodGet:
		num, _ := strconv.ParseInt(rest[1], 10, 64)
		run, err := getRun(repo.ID, num)
		if err != nil {
			c.err(404, "run not found")
			return
		}
		m := runJSON(run, true)
		if commit, err := getCommit(repo.DiskPath(), run.CommitSHA); err == nil {
			m["commit_info"] = commitJSON(commit)
		}
		c.out(200, m)
	case len(rest) == 3 && rest[0] == "runs" && rest[2] == "rerun" && c.r.Method == http.MethodPost:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		num, _ := strconv.ParseInt(rest[1], 10, 64)
		run, err := getRun(repo.ID, num)
		if err != nil {
			c.err(404, "run not found")
			return
		}
		newRun := enqueueCI(repo, run.CommitSHA, run.Ref, run.Event)
		if newRun == nil {
			c.err(422, "no CI config at that commit")
			return
		}
		c.out(201, runJSON(newRun, false))
	default:
		c.err(404, "unknown endpoint")
	}
}
