package main

// Notifications: an inbox of the things that happened to you.
//
// The hard part of a notification system is not storage, it is deciding who
// hears about what. GitGit answers that with three rules, in order of
// strength:
//
//   - you are mentioned by name          -> always, it was addressed to you
//   - you authored the thread            -> always, it is yours
//   - you already took part in it        -> yes, you are in the conversation
//
// and one rule that fires only on creation: whoever administers a repository
// hears about new issues and pull requests in it, since nobody has taken part
// yet and something has to reach a maintainer.
//
// An actor is never notified of their own action. A row is upserted per
// (user, subject), so ten comments on one issue leave one unread item rather
// than burying the rest of the inbox.

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Notification reasons, strongest first. "Why am I being told about this" is
// the whole value of an inbox, so the reason is the strongest one that ever
// applied — later chatter on a thread must not downgrade a mention to a
// generic comment.
const (
	reasonMention = "mention"
	reasonAuthor  = "author"
	reasonReview  = "review"
	reasonCI      = "ci"
	reasonComment = "comment"
	reasonRepo    = "repo"
)

var reasonRank = map[string]int{
	reasonMention: 0, reasonAuthor: 1, reasonReview: 2,
	reasonCI: 3, reasonComment: 4, reasonRepo: 5,
}

func rankOf(reason string) int {
	if r, ok := reasonRank[reason]; ok {
		return r
	}
	return 9
}

type Notification struct {
	ID        int64
	UserID    int64
	RepoID    int64
	RepoName  string
	Subject   string // "issue" | "pull"
	Number    int64
	Title     string
	Reason    string
	ActorID   int64
	ActorName string
	Read      bool
	UpdatedAt int64
}

// mentionRe matches @username as a word, not inside an email address or a
// path. Usernames allow dots, so a trailing one is trimmed as punctuation.
var mentionRe = regexp.MustCompile(`(^|[^\w@/.-])@([A-Za-z0-9][A-Za-z0-9._-]*)`)

func mentionedUsers(body string) []*User {
	seen := map[string]bool{}
	out := []*User{}
	for _, m := range mentionRe.FindAllStringSubmatch(body, -1) {
		name := strings.TrimRight(m[2], ".")
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		if u, err := getUserByName(name); err == nil && !u.IsOrg {
			out = append(out, u)
		}
	}
	return out
}

// notify records one item for one recipient. Self-notification is dropped
// here rather than at every call site, so a new caller cannot forget it.
func notify(userID int64, actor *User, repo *Repo, subject string, number int64, title, reason string) {
	if userID == 0 || repo == nil || (actor != nil && actor.ID == userID) {
		return
	}
	if u, err := getUserByID(userID); err != nil || u.IsOrg {
		return // an organization has no inbox
	}
	var actorID int64
	if actor != nil {
		actorID = actor.ID
	}
	// Log rather than discard: a silent failure here means the inbox quietly
	// stops working, which is indistinguishable from "nothing happened".
	_, err := db.Exec(`INSERT INTO notifications
			(user_id, repo_id, subject, number, title, reason, reason_rank, actor_id, read, updated_at)
		VALUES (?,?,?,?,?,?,?,?,0,?)
		ON CONFLICT(user_id, repo_id, subject, number) DO UPDATE SET
			title = excluded.title,
			-- keep the strongest reason that has ever applied
			reason      = CASE WHEN excluded.reason_rank < notifications.reason_rank
			                   THEN excluded.reason ELSE notifications.reason END,
			reason_rank = MIN(excluded.reason_rank, notifications.reason_rank),
			actor_id    = excluded.actor_id,
			read        = 0,
			updated_at  = excluded.updated_at`,
		userID, repo.ID, subject, number, title, reason, rankOf(reason), actorID, now())
	if err != nil {
		log.Printf("notify: %s#%d for user %d: %v", repo.FullName(), number, userID, err)
	}
}

// notifyThread fans one event out to everyone with a stake in a thread.
func notifyThread(actor *User, repo *Repo, subject string, number, authorID int64, title, body, reason string) {
	// mention wins: it was addressed to this person by name
	mentioned := map[int64]bool{}
	for _, u := range mentionedUsers(body) {
		mentioned[u.ID] = true
		notify(u.ID, actor, repo, subject, number, title, reasonMention)
	}
	if !mentioned[authorID] {
		notify(authorID, actor, repo, subject, number, title, reasonAuthor)
	}
	for _, id := range threadParticipants(subject, repo.ID, number) {
		if !mentioned[id] && id != authorID {
			notify(id, actor, repo, subject, number, title, reason)
		}
	}
}

// threadParticipants are the people who have already spoken in a thread.
func threadParticipants(subject string, repoID, number int64) []int64 {
	var target string
	var targetID int64
	switch subject {
	case "pull":
		p, err := getPull(repoID, number)
		if err != nil {
			return nil
		}
		target, targetID = "pull", p.ID
	default:
		i, err := getIssue(repoID, number)
		if err != nil {
			return nil
		}
		target, targetID = "issue", i.ID
	}
	rows, err := db.Query("SELECT DISTINCT author_id FROM comments WHERE target = ? AND target_id = ? AND system = 0",
		target, targetID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	if subject == "pull" {
		// reviewers are participants even if they left no plain comment
		r2, err := db.Query("SELECT DISTINCT reviewer_id FROM reviews WHERE pull_id = ?", targetID)
		if err == nil {
			defer r2.Close()
			for r2.Next() {
				var id int64
				if r2.Scan(&id) == nil {
					out = append(out, id)
				}
			}
		}
	}
	return out
}

// notifyRepoAdmins reaches maintainers when a thread is new and therefore has
// no participants yet.
func notifyRepoAdmins(actor *User, repo *Repo, subject string, number int64, title string) {
	seen := map[int64]bool{}
	add := func(id int64) {
		if !seen[id] {
			seen[id] = true
			notify(id, actor, repo, subject, number, title, reasonRepo)
		}
	}
	owner, err := getUserByID(repo.OwnerID)
	if err == nil && owner.IsOrg {
		for _, m := range listOrgMembers(repo.OwnerID) {
			if m.Role == roleOrgOwner {
				add(m.User.ID)
			}
		}
	} else {
		add(repo.OwnerID)
	}
	for _, c := range listCollaborators(repo.ID) {
		if c.Role == "admin" || c.Role == "write" {
			add(c.User.ID)
		}
	}
}

// ---------- reading ----------

func listNotifications(userID int64, includeRead bool, limit int) []*Notification {
	q := `SELECT n.id, n.user_id, n.repo_id, n.subject, n.number, n.title, n.reason,
	             n.actor_id, n.read, n.updated_at
	      FROM notifications n WHERE n.user_id = ?`
	if !includeRead {
		q += " AND n.read = 0"
	}
	q += " ORDER BY n.updated_at DESC LIMIT ?"
	rows, err := db.Query(q, userID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*Notification{}
	for rows.Next() {
		n := &Notification{}
		if rows.Scan(&n.ID, &n.UserID, &n.RepoID, &n.Subject, &n.Number, &n.Title, &n.Reason,
			&n.ActorID, &n.Read, &n.UpdatedAt) != nil {
			continue
		}
		if repo, err := getRepoByID(n.RepoID); err == nil {
			n.RepoName = repo.FullName()
		}
		if a, err := getUserByID(n.ActorID); err == nil {
			n.ActorName = a.Username
		}
		out = append(out, n)
	}
	return out
}

func unreadNotificationCount(userID int64) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read = 0", userID).Scan(&n)
	return n
}

func markNotificationRead(userID, id int64) {
	db.Exec("UPDATE notifications SET read = 1 WHERE id = ? AND user_id = ?", id, userID)
}

func markAllNotificationsRead(userID int64) {
	db.Exec("UPDATE notifications SET read = 1 WHERE user_id = ? AND read = 0", userID)
}

func notificationJSON(n *Notification) map[string]any {
	return map[string]any{
		"id": n.ID, "repo": n.RepoName, "subject": n.Subject, "number": n.Number,
		"title": n.Title, "reason": n.Reason, "actor": n.ActorName,
		"read": n.Read, "updated_at": n.UpdatedAt,
		"url": fmt.Sprintf("/%s/%s/%d", n.RepoName, map[string]string{"pull": "pull", "issue": "issue"}[n.Subject], n.Number),
	}
}

// ---------- API ----------

func handleAPINotifications(c *apiCtx, rest []string) {
	if !c.requireUser() {
		return
	}
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		all := c.r.URL.Query().Get("all") == "1"
		out := []map[string]any{}
		for _, n := range listNotifications(c.u.ID, all, 100) {
			out = append(out, notificationJSON(n))
		}
		c.out(200, map[string]any{"notifications": out, "unread": unreadNotificationCount(c.u.ID)})

	case len(rest) == 1 && rest[0] == "count" && c.r.Method == http.MethodGet:
		c.out(200, map[string]any{"unread": unreadNotificationCount(c.u.ID)})

	case len(rest) == 1 && rest[0] == "read" && c.r.Method == http.MethodPost:
		markAllNotificationsRead(c.u.ID)
		c.out(200, map[string]any{"unread": 0})

	case len(rest) == 2 && rest[1] == "read" && c.r.Method == http.MethodPost:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		markNotificationRead(c.u.ID, id)
		c.out(200, map[string]any{"unread": unreadNotificationCount(c.u.ID)})

	default:
		c.err(404, "unknown endpoint")
	}
}
