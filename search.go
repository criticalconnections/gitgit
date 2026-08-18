package main

// Search across repositories, issues, pull requests, code and people.
//
// Everything here filters by what the caller may actually see. That is worth
// stating plainly, because search is the classic way private data leaks out of
// a forge: a title in a result list is a disclosure even when the link 404s.
// Every branch below either restricts by repository id from listVisibleRepos,
// or calls canRead.
//
// Code search shells out to `git grep` against the default branch of each
// visible repository. No index is maintained: git already has one, and a
// second one would be a second thing to keep correct.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	maxSearchResults  = 50
	maxCodeMatchRepos = 40 // stop grepping after this many repositories
)

type CodeMatch struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// searchRepos matches on name and description.
func searchRepos(u *User, q string) []map[string]any {
	needle := strings.ToLower(q)
	out := []map[string]any{}
	for _, rp := range listVisibleRepos(u) {
		if strings.Contains(strings.ToLower(rp.FullName()), needle) ||
			strings.Contains(strings.ToLower(rp.Description), needle) {
			out = append(out, repoJSON(rp, u))
			if len(out) >= maxSearchResults {
				break
			}
		}
	}
	return out
}

// searchUsers matches people and organizations by name.
func searchUsers(q string) []map[string]any {
	rows, err := db.Query("SELECT "+userCols+" FROM users WHERE username LIKE ? OR full_name LIKE ? ORDER BY username LIMIT ?",
		"%"+q+"%", "%"+q+"%", maxSearchResults)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		if u, err := scanUser(rows); err == nil {
			out = append(out, userJSON(u))
		}
	}
	return out
}

// visibleRepoIDs is the id set every content search is restricted to, so a
// private repository cannot surface through its issues or its code.
func visibleRepoIDs(u *User, only *Repo) ([]int64, map[int64]*Repo) {
	byID := map[int64]*Repo{}
	var ids []int64
	if only != nil {
		if !canRead(u, only) {
			return nil, byID
		}
		return []int64{only.ID}, map[int64]*Repo{only.ID: only}
	}
	for _, rp := range listVisibleRepos(u) {
		ids = append(ids, rp.ID)
		byID[rp.ID] = rp
	}
	return ids, byID
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// searchIssues covers issues and pull requests, which share a number space.
func searchIssues(u *User, q string, only *Repo, kind string) []map[string]any {
	ids, byID := visibleRepoIDs(u, only)
	if len(ids) == 0 {
		return []map[string]any{}
	}
	args := make([]any, 0, len(ids)+3)
	for _, id := range ids {
		args = append(args, id)
	}
	like := "%" + q + "%"

	out := []map[string]any{}
	add := func(query string, isPull bool) {
		rows, err := db.Query(query, append(append([]any{}, args...), like, like, maxSearchResults)...)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id, repoID, number, createdAt int64
			var title, state string
			if rows.Scan(&id, &repoID, &number, &title, &state, &createdAt) != nil {
				continue
			}
			rp := byID[repoID]
			if rp == nil {
				continue
			}
			noun := "issue"
			if isPull {
				noun = "pull"
			}
			out = append(out, map[string]any{
				"type": noun, "repo": rp.FullName(), "number": number,
				"title": title, "state": state, "created_at": createdAt,
				"url": fmt.Sprintf("/%s/%s/%d", rp.FullName(), noun, number),
			})
		}
	}

	in := placeholders(len(ids))
	if kind != "pulls" {
		add(`SELECT id, repo_id, number, title, state, created_at FROM issues
		     WHERE repo_id IN (`+in+`) AND (title LIKE ? OR body LIKE ?)
		     ORDER BY created_at DESC LIMIT ?`, false)
	}
	if kind != "issues" {
		add(`SELECT id, repo_id, number, title, state, created_at FROM pulls
		     WHERE repo_id IN (`+in+`) AND (title LIKE ? OR body LIKE ?)
		     ORDER BY created_at DESC LIMIT ?`, true)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["created_at"].(int64) > out[j]["created_at"].(int64)
	})
	if len(out) > maxSearchResults {
		out = out[:maxSearchResults]
	}
	return out
}

// searchCode greps the default branch of each visible repository. git grep
// treats the pattern as a regexp by default; --fixed-strings keeps a stray
// bracket in a query from becoming a syntax error the user has to debug.
func searchCode(u *User, q string, only *Repo) []CodeMatch {
	ids, byID := visibleRepoIDs(u, only)
	out := []CodeMatch{}
	searched := 0
	for _, id := range ids {
		rp := byID[id]
		if rp == nil {
			continue
		}
		if searched >= maxCodeMatchRepos || len(out) >= maxSearchResults {
			break
		}
		searched++
		ref := rp.DefaultBranch
		res, err := gitRun(rp.DiskPath(), "grep", "--fixed-strings", "--ignore-case",
			"--line-number", "--max-count", "5", "-I", q, ref)
		if err != nil || res == "" {
			continue // exit status 1 simply means no match
		}
		for _, line := range strings.Split(res, "\n") {
			if len(out) >= maxSearchResults {
				break
			}
			// ref:path/to/file:LINE:text
			rest, ok := strings.CutPrefix(line, ref+":")
			if !ok {
				continue
			}
			path, rest, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			numStr, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			n, err := strconv.Atoi(numStr)
			if err != nil {
				continue
			}
			if len(text) > 300 {
				text = text[:300] + "…"
			}
			out = append(out, CodeMatch{
				Repo: rp.FullName(), Ref: ref, Path: path, Line: n, Text: text,
			})
		}
	}
	return out
}

// ---------- API ----------

// handleAPISearch answers /api/v1/search?q=&type=&repo=
func handleAPISearch(c *apiCtx) {
	if c.r.Method != http.MethodGet {
		c.err(405, "method not allowed")
		return
	}
	q := strings.TrimSpace(c.r.URL.Query().Get("q"))
	if q == "" {
		c.out(200, map[string]any{"query": "", "repos": []any{}, "issues": []any{}, "code": []any{}, "users": []any{}})
		return
	}
	kind := c.r.URL.Query().Get("type") // "", repos, issues, pulls, code, users

	var only *Repo
	if name := strings.TrimSpace(c.r.URL.Query().Get("repo")); name != "" {
		owner, repoName, ok := strings.Cut(name, "/")
		if !ok {
			c.err(422, "repo must be owner/name")
			return
		}
		rp, err := getRepo(owner, repoName)
		if err != nil || !canRead(c.u, rp) {
			c.err(404, "repository not found")
			return
		}
		only = rp
	}

	body := map[string]any{"query": q}
	if kind == "" || kind == "repos" {
		body["repos"] = searchRepos(c.u, q)
	}
	if kind == "" || kind == "issues" || kind == "pulls" {
		body["issues"] = searchIssues(c.u, q, only, kind)
	}
	if kind == "" || kind == "code" {
		body["code"] = searchCode(c.u, q, only)
	}
	if kind == "" || kind == "users" {
		body["users"] = searchUsers(q)
	}
	c.out(200, body)
}
