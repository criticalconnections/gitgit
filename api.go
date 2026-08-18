package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// ---------- JSON envelope helpers ----------

type apiCtx struct {
	w http.ResponseWriter
	r *http.Request
	u *User // may be nil
}

func (c *apiCtx) out(code int, v any) {
	c.w.Header().Set("Content-Type", "application/json")
	c.w.WriteHeader(code)
	json.NewEncoder(c.w).Encode(v)
}

func (c *apiCtx) err(code int, msg string) {
	c.out(code, map[string]string{"error": msg})
}

func (c *apiCtx) decode(v any) bool {
	if err := json.NewDecoder(c.r.Body).Decode(v); err != nil {
		c.err(http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (c *apiCtx) requireUser() bool {
	if c.u == nil {
		c.err(http.StatusUnauthorized, "authentication required")
		return false
	}
	return true
}

// sameOriginOK guards cookie-authenticated mutations against CSRF: browsers
// always send Sec-Fetch-Site / Origin on cross-site requests. Token-based
// clients (curl, CI) send neither header and are exempt because token auth
// cannot be triggered cross-site.
func sameOriginOK(r *http.Request) bool {
	if basicAuthUser(r) != nil {
		return true // explicit credentials, not ambient cookies
	}
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" {
		return sfs == "same-origin" || sfs == "none"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client using session? not possible cross-site
	}
	return strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://") == r.Host
}

// apiUser resolves the caller: token/Basic first, then session cookie.
func apiUser(r *http.Request) *User {
	if u := basicAuthUser(r); u != nil {
		return u
	}
	return currentUser(r)
}

// handleAPI routes everything under /api/v1/.
func handleAPI(w http.ResponseWriter, r *http.Request) {
	c := &apiCtx{w: w, r: r, u: apiUser(r)}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOriginOK(r) {
		c.err(http.StatusForbidden, "cross-origin request rejected")
		return
	}

	switch {
	case len(parts) == 0:
		c.out(200, map[string]string{"name": "gitgit", "version": "1.0"})
	case parts[0] == "qr" && len(parts) == 1:
		handleAPIQR(c)
	case parts[0] == "auth":
		handleAPIAuth(c, parts[1:])
	case parts[0] == "user":
		handleAPIUser(c, parts[1:])
	case parts[0] == "users" && len(parts) == 2:
		handleAPIUserProfile(c, parts[1])
	case parts[0] == "orgs":
		handleAPIOrgs(c, parts[1:])
	case parts[0] == "repos" && len(parts) == 1:
		handleAPIRepoIndex(c)
	case parts[0] == "import":
		handleAPIImport(c, parts[1:])
	case parts[0] == "repos" && len(parts) >= 3:
		repo, err := getRepo(parts[1], parts[2])
		if err != nil || !canRead(c.u, repo) {
			c.err(404, "repository not found")
			return
		}
		handleAPIRepo(c, repo, parts[3:])
	default:
		c.err(404, "unknown endpoint")
	}
}

// handleAPIQR renders a QR code PNG for a short text (preview URLs).
func handleAPIQR(c *apiCtx) {
	if !c.requireUser() {
		return
	}
	text := c.r.URL.Query().Get("text")
	if text == "" || len(text) > 512 {
		c.err(422, "text required (max 512 chars)")
		return
	}
	png, err := qrcode.Encode(text, qrcode.Medium, 512)
	if err != nil {
		c.err(500, err.Error())
		return
	}
	c.w.Header().Set("Content-Type", "image/png")
	c.w.Header().Set("Cache-Control", "private, max-age=300")
	c.w.Write(png)
}

// ---------- auth ----------

func handleAPIAuth(c *apiCtx, rest []string) {
	if c.r.Method != http.MethodPost || len(rest) != 1 {
		c.err(404, "unknown endpoint")
		return
	}
	switch rest[0] {
	case "register":
		var req struct{ Username, Email, Password string }
		if !c.decode(&req) {
			return
		}
		// Closed registration still permits the very first account, so a fresh
		// internet-facing instance can bootstrap its admin.
		if !openReg && userCount() > 0 {
			c.err(403, "registration is closed on this instance")
			return
		}
		u, err := createUser(strings.TrimSpace(req.Username), strings.TrimSpace(req.Email), req.Password)
		if err != nil {
			c.err(422, err.Error())
			return
		}
		s, err := createSession(u.ID)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		setSessionCookie(c.w, c.r, s.Token)
		c.out(201, userJSON(u))
	case "login":
		var req struct{ Username, Password string }
		if !c.decode(&req) {
			return
		}
		u, err := getUserByName(strings.TrimSpace(req.Username))
		if err != nil || !checkPassword(u, req.Password) {
			c.err(401, "invalid username or password")
			return
		}
		s, err := createSession(u.ID)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		setSessionCookie(c.w, c.r, s.Token)
		c.out(200, userJSON(u))
	case "logout":
		if tok := readSessionCookie(c.r); tok != "" {
			deleteSession(tok)
		}
		clearSessionCookie(c.w)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(404, "unknown endpoint")
	}
}

// ---------- current user ----------

func handleAPIUser(c *apiCtx, rest []string) {
	if !c.requireUser() {
		return
	}
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		c.out(200, userJSON(c.u))
	case len(rest) == 0 && c.r.Method == http.MethodPatch:
		var req struct{ Email, FullName string }
		if !c.decode(&req) {
			return
		}
		if err := updateProfile(c.u.ID, strings.TrimSpace(req.Email), strings.TrimSpace(req.FullName)); err != nil {
			c.err(500, err.Error())
			return
		}
		u, _ := getUserByID(c.u.ID)
		c.out(200, userJSON(u))
	case len(rest) == 1 && rest[0] == "password" && c.r.Method == http.MethodPost:
		var req struct{ Current, Password string }
		if !c.decode(&req) {
			return
		}
		if !checkPassword(c.u, req.Current) {
			c.err(403, "current password is incorrect")
			return
		}
		if err := setPassword(c.u.ID, req.Password); err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(200, map[string]bool{"ok": true})
	case len(rest) == 1 && rest[0] == "tokens" && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, t := range listAccessTokens(c.u.ID) {
			row := map[string]any{"id": t.ID, "name": t.Name, "created_at": t.CreatedAt}
			if t.LastUsedAt.Valid {
				row["last_used_at"] = t.LastUsedAt.Int64
			}
			out = append(out, row)
		}
		c.out(200, out)
	case len(rest) == 1 && rest[0] == "tokens" && c.r.Method == http.MethodPost:
		var req struct{ Name string }
		if !c.decode(&req) {
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = "token"
		}
		plain, err := createAccessToken(c.u.ID, strings.TrimSpace(req.Name))
		if err != nil {
			c.err(500, err.Error())
			return
		}
		c.out(201, map[string]string{"token": plain, "name": req.Name})
	case len(rest) == 2 && rest[0] == "tokens" && c.r.Method == http.MethodDelete:
		id, _ := strconv.ParseInt(rest[1], 10, 64)
		deleteAccessToken(id, c.u.ID)
		c.out(200, map[string]bool{"ok": true})
	default:
		c.err(404, "unknown endpoint")
	}
}

// ---------- public user profiles ----------

func handleAPIUserProfile(c *apiCtx, name string) {
	u, err := getUserByName(name)
	if err != nil {
		c.err(404, "user not found")
		return
	}
	// Ask canRead rather than reimplementing visibility here: it is the one
	// place that knows about collaborators, organization membership and site
	// admins, and a second copy of that logic would drift.
	out := []map[string]any{}
	for _, rp := range listReposForOwner(u.ID, true) {
		if canRead(c.u, rp) {
			out = append(out, repoJSON(rp, c.u))
		}
	}
	body := map[string]any{"user": userJSON(u), "repos": out}
	if u.IsOrg {
		body["can_admin"] = c.u != nil && (c.u.IsAdmin || isOrgOwner(u.ID, c.u.ID))
		body["role"] = ""
		if c.u != nil {
			body["role"] = orgRole(u.ID, c.u.ID)
		}
	}
	c.out(200, body)
}

// ---------- repo index ----------

func handleAPIRepoIndex(c *apiCtx) {
	switch c.r.Method {
	case http.MethodGet:
		q := strings.ToLower(strings.TrimSpace(c.r.URL.Query().Get("q")))
		out := []map[string]any{}
		for _, rp := range listVisibleRepos(c.u) {
			if q != "" && !strings.Contains(strings.ToLower(rp.FullName()), q) &&
				!strings.Contains(strings.ToLower(rp.Description), q) {
				continue
			}
			out = append(out, repoJSON(rp, c.u))
		}
		c.out(200, out)
	case http.MethodPost:
		if !c.requireUser() {
			return
		}
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Private     bool   `json:"private"`
			AutoInit    bool   `json:"auto_init"`
			Owner       string `json:"owner"` // an organization, or empty for yourself
		}
		if !c.decode(&req) {
			return
		}
		ownerID := c.u.ID
		if org := strings.TrimSpace(req.Owner); org != "" && !strings.EqualFold(org, c.u.Username) {
			target, err := getUserByName(org)
			if err != nil || !target.IsOrg {
				c.err(422, "no such organization")
				return
			}
			if !isOrgOwner(target.ID, c.u.ID) && !c.u.IsAdmin {
				c.err(403, "you need to be an owner of "+target.Username)
				return
			}
			ownerID = target.ID
		}
		repo, err := insertRepo(ownerID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Private)
		if err != nil {
			c.err(422, err.Error())
			return
		}
		if err := initBareRepo(repo.DiskPath(), repo.DefaultBranch); err != nil {
			deleteRepoRows(repo.ID)
			c.err(500, err.Error())
			return
		}
		if req.AutoInit {
			seedInitialCommit(repo.DiskPath(), repo.DefaultBranch, c.u, repo.Name, req.Description)
		}
		c.out(201, repoJSON(repo, c.u))
	default:
		c.err(405, "method not allowed")
	}
}

// ---------- shared JSON shapes ----------

func userJSON(u *User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"id": u.ID, "username": u.Username, "email": u.Email,
		"full_name": u.FullName, "is_admin": u.IsAdmin, "is_org": u.IsOrg,
		"created_at": u.CreatedAt,
	}
}

func userRefJSON(id int64) map[string]any {
	u, err := getUserByID(id)
	if err != nil {
		return map[string]any{"id": id, "username": "ghost"}
	}
	return map[string]any{"id": u.ID, "username": u.Username, "full_name": u.FullName}
}

func repoJSON(r *Repo, viewer *User) map[string]any {
	return map[string]any{
		"id": r.ID, "owner": r.OwnerName, "name": r.Name, "full_name": r.FullName(),
		"description": r.Description, "default_branch": r.DefaultBranch,
		"private": r.IsPrivate, "created_at": r.CreatedAt,
		"can_write": canWrite(viewer, r), "can_admin": canAdmin(viewer, r),
		"stars": starCount(r.ID), "starred": viewer != nil && isStarred(viewer.ID, r.ID),
		"open_pulls": countPulls(r.ID, "open"), "open_issues": countIssues(r.ID, "open"),
		"empty":                  isEmptyRepo(r.DiskPath()),
		"allow_merge":            r.AllowMerge,
		"allow_squash":           r.AllowSquash,
		"allow_rebase":           r.AllowRebase,
		"delete_branch_on_merge": r.DeleteBranchOnMerge,
		"require_ci_pass":        r.RequireCIPass,
		"require_approvals":      r.RequireApprovals,
	}
}

func labelJSON(l *Label) map[string]any {
	return map[string]any{"id": l.ID, "name": l.Name, "color": l.Color}
}

func commitJSON(ci *CommitInfo) map[string]any {
	return map[string]any{
		"sha": ci.SHA, "short_sha": ci.ShortSHA, "author_name": ci.AuthorName,
		"author_email": ci.AuthorEmail, "when": ci.When, "subject": ci.Subject,
		"body": ci.Body, "parents": ci.Parents,
	}
}

func diffFileJSON(f *DiffFile) map[string]any {
	hunks := []map[string]any{}
	for _, h := range f.Hunks {
		lines := []map[string]any{}
		for _, l := range h.Lines {
			lines = append(lines, map[string]any{
				"op": string(l.Op), "old": l.OldNum, "new": l.NewNum, "text": l.Text,
			})
		}
		hunks = append(hunks, map[string]any{"header": h.Header, "lines": lines})
	}
	return map[string]any{
		"old_path": f.OldPath, "new_path": f.NewPath, "path": f.DisplayPath(),
		"status": f.Status, "binary": f.IsBinary, "additions": f.Additions,
		"deletions": f.Deletions, "truncated": f.Truncated, "hunks": hunks,
	}
}

func diffJSON(files []*DiffFile) map[string]any {
	st := diffStats(files)
	out := []map[string]any{}
	for _, f := range files {
		out = append(out, diffFileJSON(f))
	}
	return map[string]any{
		"files": out,
		"stat":  map[string]any{"files": st.Files, "additions": st.Additions, "deletions": st.Deletions},
	}
}

func runJSON(run *CIRun, withJobs bool) map[string]any {
	m := map[string]any{
		"id": run.ID, "number": run.Number, "commit": run.CommitSHA, "ref": run.Ref,
		"event": run.Event, "status": run.Status, "created_at": run.CreatedAt,
	}
	if run.StartedAt.Valid {
		m["started_at"] = run.StartedAt.Int64
	}
	if run.FinishedAt.Valid {
		m["finished_at"] = run.FinishedAt.Int64
	}
	if withJobs {
		jobs := []map[string]any{}
		for _, j := range runJobs(run.ID) {
			jm := map[string]any{
				"id": j.ID, "name": j.Name, "status": j.Status, "exit_code": j.ExitCode, "log": j.Log,
			}
			if j.StartedAt.Valid {
				jm["started_at"] = j.StartedAt.Int64
			}
			if j.FinishedAt.Valid {
				jm["finished_at"] = j.FinishedAt.Int64
			}
			jobs = append(jobs, jm)
		}
		m["jobs"] = jobs
	}
	return m
}

func pullJSON(repo *Repo, p *Pull) map[string]any {
	m := map[string]any{
		"id": p.ID, "number": p.Number, "title": p.Title, "body": p.Body,
		"body_html": string(renderMarkdown(p.Body)),
		"state":     p.State, "base": p.BaseBranch, "head": p.HeadBranch,
		"merge_commit": p.MergeCommit, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
		"author": userRefJSON(p.AuthorID),
	}
	if p.MergedAt.Valid {
		m["merged_at"] = p.MergedAt.Int64
	}
	if p.MergedBy.Valid {
		m["merged_by"] = userRefJSON(p.MergedBy.Int64)
	}
	if sha, err := resolveCommit(repo.DiskPath(), "refs/heads/"+p.HeadBranch); err == nil {
		if s, run := ciStatusForSHA(repo.ID, sha); s != "" {
			m["ci_status"] = s
			m["ci_run"] = run.Number
		}
	}
	m["comments"] = countComments("pull", p.ID)
	return m
}

func issueJSON(is *Issue) map[string]any {
	labels := []map[string]any{}
	for _, l := range itemLabels("issue", is.ID) {
		labels = append(labels, labelJSON(l))
	}
	m := map[string]any{
		"id": is.ID, "number": is.Number, "title": is.Title, "body": is.Body,
		"body_html": string(renderMarkdown(is.Body)),
		"state":     is.State, "created_at": is.CreatedAt, "updated_at": is.UpdatedAt,
		"author": userRefJSON(is.AuthorID), "labels": labels,
		"comments": countComments("issue", is.ID),
	}
	return m
}

func commentJSON(cm *Comment) map[string]any {
	return map[string]any{
		"id": cm.ID, "body": cm.Body, "body_html": string(renderMarkdown(cm.Body)),
		"system": cm.System, "created_at": cm.CreatedAt, "author": userRefJSON(cm.AuthorID),
	}
}
