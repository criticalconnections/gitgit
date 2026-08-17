package main

import (
	"fmt"
	"sort"
	"strconv"
)

func nullNow() (n NullInt64) { n.Int64, n.Valid = now(), true; return }
func nullInt() (n NullInt64) { return }

func prURL(repo *Repo, pr *Pull) string {
	return "/" + repo.FullName() + "/pull/" + strconv.FormatInt(pr.Number, 10)
}

func prPayload(repo *Repo, pr *Pull, action string) map[string]any {
	return map[string]any{
		"repository": repo.FullName(),
		"action":     action,
		"number":     pr.Number,
		"title":      pr.Title,
		"base":       pr.BaseBranch,
		"head":       pr.HeadBranch,
		"state":      pr.State,
	}
}

// ---------- stack computation ----------

type StackEntry struct {
	Pull    *Pull
	Depth   int
	Current bool
	CI      string
}

// stackForPull returns the whole stack containing pr, ordered bottom→top.
// Returns nil when the PR is not part of a stack.
func stackForPull(repo *Repo, pr *Pull) []*StackEntry {
	bottom := pr
	seen := map[int64]bool{pr.ID: true}
	for {
		parents := openPullsWithHead(repo.ID, bottom.BaseBranch)
		if len(parents) == 0 || seen[parents[0].ID] {
			break
		}
		bottom = parents[0]
		seen[bottom.ID] = true
	}
	var out []*StackEntry
	visited := map[int64]bool{}
	var walk func(p *Pull, depth int)
	walk = func(p *Pull, depth int) {
		if visited[p.ID] {
			return
		}
		visited[p.ID] = true
		out = append(out, &StackEntry{Pull: p, Depth: depth, Current: p.ID == pr.ID})
		for _, child := range openPullsWithBase(repo.ID, p.HeadBranch) {
			walk(child, depth+1)
		}
	}
	walk(bottom, 0)
	if len(out) <= 1 {
		return nil
	}
	dir := repo.DiskPath()
	for _, e := range out {
		if sha, err := resolveCommit(dir, "refs/heads/"+e.Pull.HeadBranch); err == nil {
			e.CI, _ = ciStatusForSHA(repo.ID, sha)
		}
	}
	return out
}

// ---------- merge state ----------

// MergeState captures everything that determines whether a PR can merge.
type MergeState struct {
	BranchesOK       bool
	BaseSHA, HeadSHA string
	Ahead, Behind    int
	Clean            bool
	Conflicts        []string
	CIStatus         string
	CIRun            *CIRun
	HasCIConfig      bool
	Approvals        int
	ChangesRequested int
}

func computeMergeState(repo *Repo, pr *Pull) *MergeState {
	dir := repo.DiskPath()
	st := &MergeState{}
	var baseErr, headErr error
	st.BaseSHA, baseErr = resolveCommit(dir, "refs/heads/"+pr.BaseBranch)
	st.HeadSHA, headErr = resolveCommit(dir, "refs/heads/"+pr.HeadBranch)
	st.BranchesOK = baseErr == nil && headErr == nil
	if st.BranchesOK && pr.State == "open" {
		st.Ahead, st.Behind = aheadBehind(dir, pr.BaseBranch, pr.HeadBranch)
		if mc, err := mergeTreeCheck(dir, st.BaseSHA, st.HeadSHA); err == nil {
			st.Clean = mc.Clean
			st.Conflicts = mc.Conflicts
		} else {
			st.Clean = false
			st.Conflicts = []string{err.Error()}
		}
		st.CIStatus, st.CIRun = ciStatusForSHA(repo.ID, st.HeadSHA)
		st.HasCIConfig = loadCIConfig(dir, st.HeadSHA) != nil
	}
	st.Approvals, st.ChangesRequested = approvalCount(pr.ID)
	return st
}

// mergeBlockersFor explains everything preventing a merge right now.
func mergeBlockersFor(repo *Repo, pr *Pull, u *User, st *MergeState) []string {
	if pr.State != "open" {
		return []string{"pull request is " + pr.State}
	}
	blockers := []string{}
	if !st.BranchesOK {
		return []string{"base or head branch no longer exists"}
	}
	if !canWrite(u, repo) {
		blockers = append(blockers, "you need write access to merge")
	}
	if !st.Clean {
		blockers = append(blockers, "merge conflicts must be resolved")
	}
	if repo.RequireCIPass && st.HasCIConfig && st.CIStatus != "success" {
		switch st.CIStatus {
		case "":
			blockers = append(blockers, "CI has not run for the head commit")
		case "queued", "running":
			blockers = append(blockers, "CI is still "+st.CIStatus)
		default:
			blockers = append(blockers, "CI status is "+st.CIStatus)
		}
	}
	if repo.RequireApprovals > 0 {
		if int64(st.Approvals) < repo.RequireApprovals {
			blockers = append(blockers, fmt.Sprintf("needs %d approval(s), has %d", repo.RequireApprovals, st.Approvals))
		}
		if st.ChangesRequested > 0 {
			blockers = append(blockers, "changes were requested by a reviewer")
		}
	}
	return blockers
}

type VerdictRow struct {
	User  *User
	State string
}

func verdictRows(pullID int64) []VerdictRow {
	var rows []VerdictRow
	for uid, state := range reviewVerdicts(pullID) {
		u, err := getUserByID(uid)
		if err != nil {
			continue
		}
		rows = append(rows, VerdictRow{User: u, State: state})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].User.Username < rows[j].User.Username })
	return rows
}
