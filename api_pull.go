package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// handleAPIPulls routes /api/v1/repos/{o}/{r}/pulls/...
func handleAPIPulls(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		state := c.r.URL.Query().Get("state")
		if state == "" {
			state = "open"
		}
		out := []map[string]any{}
		for _, pr := range listPulls(repo.ID, state) {
			out = append(out, pullJSON(repo, pr))
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		apiCreatePull(c, repo)
	case len(rest) >= 1:
		num, err := strconv.ParseInt(rest[0], 10, 64)
		if err != nil {
			c.err(400, "bad pull number")
			return
		}
		pr, err := getPull(repo.ID, num)
		if err != nil {
			c.err(404, "pull request not found")
			return
		}
		apiPullActions(c, repo, pr, rest[1:])
	default:
		c.err(404, "unknown endpoint")
	}
}

func apiCreatePull(c *apiCtx, repo *Repo) {
	if !c.requireUser() {
		return
	}
	var req struct{ Title, Body, Base, Head string }
	if !c.decode(&req) {
		return
	}
	dir := repo.DiskPath()
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		c.err(422, "title is required")
		return
	}
	if req.Base == req.Head {
		c.err(422, "base and head must differ")
		return
	}
	if !branchExists(dir, req.Base) || !branchExists(dir, req.Head) {
		c.err(422, "both branches must exist")
		return
	}
	for _, pr := range openPullsWithHead(repo.ID, req.Head) {
		if pr.BaseBranch == req.Base {
			c.err(409, fmt.Sprintf("an open pull request for this branch pair already exists (#%d)", pr.Number))
			return
		}
	}
	ahead, _ := aheadBehind(dir, req.Base, req.Head)
	if ahead == 0 {
		c.err(422, fmt.Sprintf("%s has no commits that are not already in %s", req.Head, req.Base))
		return
	}
	pr, err := createPull(repo.ID, c.u.ID, req.Title, strings.TrimSpace(req.Body), req.Base, req.Head)
	if err != nil {
		c.err(500, err.Error())
		return
	}
	if sha, err := resolveCommit(dir, "refs/heads/"+req.Head); err == nil && latestRunForSHA(repo.ID, sha) == nil {
		enqueueCI(repo, sha, req.Head, "pull_request")
	}
	fireWebhooks(repo, "pull_request", prPayload(repo, pr, "opened"))
	c.out(201, pullJSON(repo, pr))
}

// apiPullDetail assembles the full PR page payload: merge state, stack,
// timeline, verdicts.
func apiPullDetail(c *apiCtx, repo *Repo, pr *Pull) {
	st := computeMergeState(repo, pr)
	m := pullJSON(repo, pr)
	m["merge_state"] = map[string]any{
		"branches_ok": st.BranchesOK, "base_sha": st.BaseSHA, "head_sha": st.HeadSHA,
		"ahead": st.Ahead, "behind": st.Behind, "clean": st.Clean, "conflicts": st.Conflicts,
		"ci_status": st.CIStatus, "has_ci_config": st.HasCIConfig,
		"approvals": st.Approvals, "changes_requested": st.ChangesRequested,
		"blockers": mergeBlockersFor(repo, pr, c.u, st),
	}
	if st.CIRun != nil {
		m["ci_run"] = st.CIRun.Number
		m["ci_status"] = st.CIStatus
	}
	if pr.State == "merged" && pr.MergeCommit != "" {
		if s, run := ciStatusForSHA(repo.ID, pr.MergeCommit); s != "" {
			m["ci_status"] = s
			m["ci_run"] = run.Number
		}
	}

	// stack
	stack := []map[string]any{}
	for _, e := range stackForPull(repo, pr) {
		stack = append(stack, map[string]any{
			"number": e.Pull.Number, "title": e.Pull.Title, "base": e.Pull.BaseBranch,
			"head": e.Pull.HeadBranch, "depth": e.Depth, "current": e.Current, "ci_status": e.CI,
		})
	}
	m["stack"] = stack

	// timeline: comments + reviews interleaved
	type item struct {
		when int64
		data map[string]any
	}
	var items []item
	for _, cm := range listComments("pull", pr.ID) {
		d := commentJSON(cm)
		d["type"] = "comment"
		items = append(items, item{cm.CreatedAt, d})
	}
	for _, rv := range listReviews(pr.ID) {
		d := map[string]any{
			"type": "review", "id": rv.ID, "state": rv.State, "body": rv.Body,
			"body_html": string(renderMarkdown(rv.Body)), "commit_sha": rv.CommitSHA,
			"created_at": rv.CreatedAt, "author": userRefJSON(rv.ReviewerID),
		}
		items = append(items, item{rv.CreatedAt, d})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].when < items[j].when })
	timeline := []map[string]any{}
	for _, it := range items {
		timeline = append(timeline, it.data)
	}
	m["timeline"] = timeline

	// latest verdicts per reviewer
	verdicts := []map[string]any{}
	for _, row := range verdictRows(pr.ID) {
		verdicts = append(verdicts, map[string]any{"user": userJSON(row.User), "state": row.State})
	}
	m["verdicts"] = verdicts
	m["review_comments"] = len(listReviewComments(pr.ID))
	c.out(200, m)
}

func apiPullActions(c *apiCtx, repo *Repo, pr *Pull, rest []string) {
	get := c.r.Method == http.MethodGet
	post := c.r.Method == http.MethodPost

	// read endpoints
	switch {
	case len(rest) == 0 && get:
		apiPullDetail(c, repo, pr)
		return
	case len(rest) == 0 && c.r.Method == http.MethodPatch:
		if c.u == nil || (!canWrite(c.u, repo) && c.u.ID != pr.AuthorID) {
			c.err(403, "not allowed")
			return
		}
		var req struct{ Title, Body *string }
		if !c.decode(&req) {
			return
		}
		if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
			pr.Title = strings.TrimSpace(*req.Title)
		}
		if req.Body != nil {
			pr.Body = strings.TrimSpace(*req.Body)
		}
		savePull(pr)
		c.out(200, pullJSON(repo, pr))
		return
	case len(rest) == 1 && rest[0] == "files" && get:
		apiPullFiles(c, repo, pr)
		return
	case len(rest) == 1 && rest[0] == "commits" && get:
		dir := repo.DiskPath()
		rows := []map[string]any{}
		if branchExists(dir, pr.BaseBranch) && branchExists(dir, pr.HeadBranch) {
			commits, _ := commitRange(dir, "refs/heads/"+pr.BaseBranch, "refs/heads/"+pr.HeadBranch)
			for _, ci := range commits {
				m := commitJSON(ci)
				if s, run := ciStatusForSHA(repo.ID, ci.SHA); s != "" {
					m["ci_status"] = s
					m["ci_run"] = run.Number
				}
				rows = append(rows, m)
			}
		}
		c.out(200, rows)
		return
	}

	// mutations below need a user
	if !post || !c.requireUser() {
		if !post {
			c.err(404, "unknown endpoint")
		}
		return
	}
	action := rest[0]
	switch action {
	case "comment":
		var req struct{ Body string }
		if !c.decode(&req) {
			return
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.Body == "" {
			c.err(422, "comment body required")
			return
		}
		cm, err := addComment("pull", pr.ID, c.u.ID, req.Body, false)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		touchPull(pr.ID)
		c.out(201, commentJSON(cm))
	case "review":
		var req struct{ Verdict, Body string }
		if !c.decode(&req) {
			return
		}
		if req.Verdict != "approved" && req.Verdict != "changes_requested" {
			req.Verdict = "commented"
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.Verdict == "commented" && req.Body == "" {
			c.err(422, "review body required for comment-only reviews")
			return
		}
		if c.u.ID == pr.AuthorID && req.Verdict == "approved" {
			c.err(422, "you cannot approve your own pull request")
			return
		}
		sha := ""
		if s, err := resolveCommit(repo.DiskPath(), "refs/heads/"+pr.HeadBranch); err == nil {
			sha = s
		}
		addReview(pr.ID, c.u.ID, req.Verdict, req.Body, sha)
		touchPull(pr.ID)
		fireWebhooks(repo, "pull_request", prPayload(repo, pr, "reviewed"))
		c.out(201, map[string]bool{"ok": true})
	case "review-comment":
		var req struct {
			File string
			Line int64
			Side string
			Body string
		}
		if !c.decode(&req) {
			return
		}
		if req.Side != "old" {
			req.Side = "new"
		}
		req.Body = strings.TrimSpace(req.Body)
		if req.File == "" || req.Line <= 0 || req.Body == "" {
			c.err(422, "file, line, and body required")
			return
		}
		sha := ""
		if s, err := resolveCommit(repo.DiskPath(), "refs/heads/"+pr.HeadBranch); err == nil {
			sha = s
		}
		addReviewComment(pr.ID, c.u.ID, req.File, req.Line, req.Side, req.Body, sha)
		touchPull(pr.ID)
		c.out(201, map[string]bool{"ok": true})
	case "merge":
		apiMergePull(c, repo, pr)
	case "close":
		if pr.State == "open" && (canWrite(c.u, repo) || c.u.ID == pr.AuthorID) {
			pr.State = "closed"
			pr.ClosedAt = nullNow()
			savePull(pr)
			addComment("pull", pr.ID, c.u.ID, "closed this pull request", true)
			fireWebhooks(repo, "pull_request", prPayload(repo, pr, "closed"))
		}
		c.out(200, pullJSON(repo, pr))
	case "reopen":
		if pr.State == "closed" && (canWrite(c.u, repo) || c.u.ID == pr.AuthorID) {
			pr.State = "open"
			pr.ClosedAt = nullInt()
			savePull(pr)
			addComment("pull", pr.ID, c.u.ID, "reopened this pull request", true)
			fireWebhooks(repo, "pull_request", prPayload(repo, pr, "reopened"))
		}
		c.out(200, pullJSON(repo, pr))
	case "retarget":
		if !canWrite(c.u, repo) && c.u.ID != pr.AuthorID {
			c.err(403, "not allowed")
			return
		}
		var req struct{ Base string }
		if !c.decode(&req) {
			return
		}
		if req.Base == pr.BaseBranch || req.Base == pr.HeadBranch || !branchExists(repo.DiskPath(), req.Base) {
			c.err(422, "invalid base branch")
			return
		}
		old := pr.BaseBranch
		pr.BaseBranch = req.Base
		savePull(pr)
		addComment("pull", pr.ID, c.u.ID, fmt.Sprintf("changed the base branch from `%s` to `%s`", old, req.Base), true)
		fireWebhooks(repo, "pull_request", prPayload(repo, pr, "retargeted"))
		c.out(200, pullJSON(repo, pr))
	case "update-branch", "rebase-branch":
		apiUpdateBranch(c, repo, pr, action == "rebase-branch")
	default:
		c.err(404, "unknown endpoint")
	}
}

func apiPullFiles(c *apiCtx, repo *Repo, pr *Pull) {
	dir := repo.DiskPath()
	var files []*DiffFile
	fromMerge := false
	if pr.State == "merged" && pr.MergeCommit != "" && !branchExists(dir, pr.HeadBranch) {
		if patch, err := commitPatch(dir, pr.MergeCommit); err == nil {
			files = parseUnifiedDiff(patch)
			fromMerge = true
		}
	} else if branchExists(dir, pr.BaseBranch) && branchExists(dir, pr.HeadBranch) {
		if raw, err := diffRange(dir, pr.BaseBranch, pr.HeadBranch, true); err == nil {
			files = parseUnifiedDiff(raw)
		}
	}
	comments := []map[string]any{}
	for _, rc := range listReviewComments(pr.ID) {
		comments = append(comments, map[string]any{
			"id": rc.ID, "file": rc.File, "line": rc.Line, "side": rc.Side,
			"body": rc.Body, "body_html": string(renderMarkdown(rc.Body)),
			"commit_sha": rc.CommitSHA, "created_at": rc.CreatedAt, "author": userRefJSON(rc.AuthorID),
		})
	}
	resp := diffJSON(files)
	resp["from_merge_commit"] = fromMerge
	resp["comments"] = comments
	c.out(200, resp)
}

func apiMergePull(c *apiCtx, repo *Repo, pr *Pull) {
	var req struct {
		Strategy     string `json:"strategy"`
		Message      string `json:"message"`
		DeleteBranch *bool  `json:"delete_branch"`
	}
	if !c.decode(&req) {
		return
	}
	if req.Strategy == "" {
		req.Strategy = "merge"
	}
	if pr.State != "open" {
		c.err(409, "pull request is not open")
		return
	}
	if blockers := mergeBlockersFor(repo, pr, c.u, computeMergeState(repo, pr)); len(blockers) > 0 {
		c.err(409, "cannot merge: "+strings.Join(blockers, "; "))
		return
	}
	allowed := map[string]bool{"merge": repo.AllowMerge, "squash": repo.AllowSquash, "rebase": repo.AllowRebase}
	if !allowed[req.Strategy] {
		c.err(422, fmt.Sprintf("merge strategy %q is not allowed for this repository", req.Strategy))
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		if req.Strategy == "squash" {
			msg = fmt.Sprintf("%s (#%d)", pr.Title, pr.Number)
			if pr.Body != "" {
				msg += "\n\n" + pr.Body
			}
		} else {
			msg = fmt.Sprintf("Merge pull request #%d from %s\n\n%s", pr.Number, pr.HeadBranch, pr.Title)
		}
	}

	dir := repo.DiskPath()
	newTip, err := mergePR(dir, pr, req.Strategy, msg, c.u)
	if err != nil {
		c.err(409, err.Error())
		return
	}
	pr.State = "merged"
	pr.MergeCommit = newTip
	pr.MergedBy.Int64, pr.MergedBy.Valid = c.u.ID, true
	pr.MergedAt = nullNow()
	savePull(pr)
	addComment("pull", pr.ID, c.u.ID, fmt.Sprintf("merged commit `%s` into `%s` (%s)", short(newTip), pr.BaseBranch, req.Strategy), true)

	// stacked children follow the merged PR's base
	for _, child := range openPullsWithBase(repo.ID, pr.HeadBranch) {
		old := child.BaseBranch
		child.BaseBranch = pr.BaseBranch
		savePull(child)
		addComment("pull", child.ID, c.u.ID,
			fmt.Sprintf("base automatically changed from `%s` to `%s` because #%d was merged", old, child.BaseBranch, pr.Number), true)
	}

	deleteBranchAfter := repo.DeleteBranchOnMerge
	if req.DeleteBranch != nil {
		deleteBranchAfter = *req.DeleteBranch
	}
	if deleteBranchAfter && pr.HeadBranch != repo.DefaultBranch && len(openPullsWithHead(repo.ID, pr.HeadBranch)) == 0 {
		if err := deleteBranch(dir, pr.HeadBranch); err == nil {
			addComment("pull", pr.ID, c.u.ID, fmt.Sprintf("deleted branch `%s`", pr.HeadBranch), true)
		}
	}
	enqueueCI(repo, newTip, pr.BaseBranch, "push")
	fireWebhooks(repo, "pull_request", prPayload(repo, pr, "merged"))
	c.out(200, pullJSON(repo, pr))
}

func apiUpdateBranch(c *apiCtx, repo *Repo, pr *Pull, rebase bool) {
	if !canWrite(c.u, repo) {
		c.err(403, "write access required")
		return
	}
	dir := repo.DiskPath()
	baseSHA, err1 := resolveCommit(dir, "refs/heads/"+pr.BaseBranch)
	headSHA, err2 := resolveCommit(dir, "refs/heads/"+pr.HeadBranch)
	if err1 != nil || err2 != nil {
		c.err(422, "branches missing")
		return
	}
	if rebase {
		target, err := rebaseCommits(dir, baseSHA, headSHA, c.u)
		if err != nil {
			c.err(409, err.Error())
			return
		}
		if err := updateRefCAS(dir, "refs/heads/"+pr.HeadBranch, target, headSHA); err != nil {
			c.err(409, err.Error())
			return
		}
		addComment("pull", pr.ID, c.u.ID, fmt.Sprintf("rebased `%s` onto `%s`", pr.HeadBranch, pr.BaseBranch), true)
		enqueueCI(repo, target, pr.HeadBranch, "pull_request")
	} else {
		mc, err := mergeTreeCheck(dir, headSHA, baseSHA)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		if !mc.Clean {
			c.err(409, "cannot update: conflicts in "+strings.Join(mc.Conflicts, ", "))
			return
		}
		msg := fmt.Sprintf("Merge branch '%s' into %s", pr.BaseBranch, pr.HeadBranch)
		newTip, err := commitTree(dir, mc.TreeSHA, []string{headSHA, baseSHA}, msg, c.u)
		if err != nil {
			c.err(500, err.Error())
			return
		}
		if err := updateRefCAS(dir, "refs/heads/"+pr.HeadBranch, newTip, headSHA); err != nil {
			c.err(409, err.Error())
			return
		}
		addComment("pull", pr.ID, c.u.ID, fmt.Sprintf("merged `%s` into `%s` to update the branch", pr.BaseBranch, pr.HeadBranch), true)
		enqueueCI(repo, newTip, pr.HeadBranch, "pull_request")
	}
	touchPull(pr.ID)
	c.out(200, pullJSON(repo, pr))
}

// ---------- stacks ----------

func apiStacks(c *apiCtx, repo *Repo) {
	open := listPulls(repo.ID, "open")
	headOf := map[string]*Pull{}
	for _, p := range open {
		headOf[p.HeadBranch] = p
	}
	var entries []*StackEntry
	visited := map[int64]bool{}
	var walk func(p *Pull, depth int)
	walk = func(p *Pull, depth int) {
		if visited[p.ID] {
			return
		}
		visited[p.ID] = true
		entries = append(entries, &StackEntry{Pull: p, Depth: depth})
		for _, child := range openPullsWithBase(repo.ID, p.HeadBranch) {
			walk(child, depth+1)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })
	for _, p := range open {
		if _, stacked := headOf[p.BaseBranch]; !stacked {
			walk(p, 0)
		}
	}
	for _, p := range open {
		walk(p, 0)
	}
	dir := repo.DiskPath()
	out := []map[string]any{}
	for _, e := range entries {
		row := map[string]any{
			"number": e.Pull.Number, "title": e.Pull.Title, "base": e.Pull.BaseBranch,
			"head": e.Pull.HeadBranch, "depth": e.Depth, "author": userRefJSON(e.Pull.AuthorID),
		}
		if sha, err := resolveCommit(dir, "refs/heads/"+e.Pull.HeadBranch); err == nil {
			if s, _ := ciStatusForSHA(repo.ID, sha); s != "" {
				row["ci_status"] = s
			}
		}
		out = append(out, row)
	}
	c.out(200, out)
}

// ---------- issues ----------

func handleAPIIssues(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		state := c.r.URL.Query().Get("state")
		if state == "" {
			state = "open"
		}
		out := []map[string]any{}
		for _, is := range listIssues(repo.ID, state) {
			out = append(out, issueJSON(is))
		}
		c.out(200, out)
	case len(rest) == 0 && c.r.Method == http.MethodPost:
		if !c.requireUser() {
			return
		}
		var req struct{ Title, Body string }
		if !c.decode(&req) {
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		if req.Title == "" {
			c.err(422, "title is required")
			return
		}
		is, err := createIssue(repo.ID, c.u.ID, req.Title, strings.TrimSpace(req.Body))
		if err != nil {
			c.err(500, err.Error())
			return
		}
		fireWebhooks(repo, "issues", map[string]any{
			"repository": repo.FullName(), "action": "opened", "number": is.Number, "title": is.Title,
		})
		c.out(201, issueJSON(is))
	case len(rest) >= 1:
		num, err := strconv.ParseInt(rest[0], 10, 64)
		if err != nil {
			c.err(400, "bad issue number")
			return
		}
		is, err := getIssue(repo.ID, num)
		if err != nil {
			// shared number space: report if it's a PR
			if pr, err2 := getPull(repo.ID, num); err2 == nil {
				c.out(200, map[string]any{"is_pull": true, "number": pr.Number})
				return
			}
			c.err(404, "issue not found")
			return
		}
		apiIssueActions(c, repo, is, rest[1:])
	default:
		c.err(404, "unknown endpoint")
	}
}

func apiIssueActions(c *apiCtx, repo *Repo, is *Issue, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		m := issueJSON(is)
		comments := []map[string]any{}
		for _, cm := range listComments("issue", is.ID) {
			comments = append(comments, commentJSON(cm))
		}
		m["comment_list"] = comments
		all := []map[string]any{}
		for _, l := range listLabels(repo.ID) {
			all = append(all, labelJSON(l))
		}
		m["all_labels"] = all
		c.out(200, m)
	case len(rest) == 0 && c.r.Method == http.MethodPatch:
		if c.u == nil || (!canWrite(c.u, repo) && c.u.ID != is.AuthorID) {
			c.err(403, "not allowed")
			return
		}
		var req struct{ Title, Body *string }
		if !c.decode(&req) {
			return
		}
		if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
			is.Title = strings.TrimSpace(*req.Title)
		}
		if req.Body != nil {
			is.Body = strings.TrimSpace(*req.Body)
		}
		saveIssue(is)
		c.out(200, issueJSON(is))
	case len(rest) == 1 && c.r.Method == http.MethodPost:
		if !c.requireUser() {
			return
		}
		switch rest[0] {
		case "comment":
			var req struct{ Body string }
			if !c.decode(&req) {
				return
			}
			req.Body = strings.TrimSpace(req.Body)
			if req.Body == "" {
				c.err(422, "comment body required")
				return
			}
			cm, err := addComment("issue", is.ID, c.u.ID, req.Body, false)
			if err != nil {
				c.err(500, err.Error())
				return
			}
			saveIssue(is)
			c.out(201, commentJSON(cm))
		case "close":
			if is.State == "open" && (canWrite(c.u, repo) || c.u.ID == is.AuthorID) {
				is.State = "closed"
				is.ClosedAt = nullNow()
				saveIssue(is)
				addComment("issue", is.ID, c.u.ID, "closed this issue", true)
				fireWebhooks(repo, "issues", map[string]any{
					"repository": repo.FullName(), "action": "closed", "number": is.Number, "title": is.Title,
				})
			}
			c.out(200, issueJSON(is))
		case "reopen":
			if is.State == "closed" && (canWrite(c.u, repo) || c.u.ID == is.AuthorID) {
				is.State = "open"
				is.ClosedAt = nullInt()
				saveIssue(is)
				addComment("issue", is.ID, c.u.ID, "reopened this issue", true)
			}
			c.out(200, issueJSON(is))
		default:
			c.err(404, "unknown endpoint")
		}
	case len(rest) == 1 && rest[0] == "labels" && c.r.Method == http.MethodPut:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		var req struct{ Labels []int64 }
		if !c.decode(&req) {
			return
		}
		selected := map[int64]bool{}
		for _, id := range req.Labels {
			selected[id] = true
		}
		for _, l := range listLabels(repo.ID) {
			setItemLabel("issue", is.ID, l.ID, selected[l.ID])
		}
		c.out(200, issueJSON(is))
	default:
		c.err(404, "unknown endpoint")
	}
}
