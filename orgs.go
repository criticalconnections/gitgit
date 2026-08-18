package main

// Organizations: shared ownership of repositories.
//
// An organization is a row in `users` with is_org set, which is how Gitea and
// GitHub model it too. Everything that already takes an owner — repository
// paths, avatars, profile pages, `repos.owner_id` — keeps working unchanged,
// and the only new concept is membership.
//
// Membership carries a role. An owner administers the organization and every
// repository in it; a member can read them and is a candidate for
// per-repository collaboration. Repository-level collaborator roles still
// apply on top, so a member can be given write access to one repository
// without being made an owner of the organization.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	roleOrgOwner  = "owner"
	roleOrgMember = "member"
)

type OrgMember struct {
	User *User
	Role string
}

func createOrg(name, description string, creator *User) (*User, error) {
	if creator == nil {
		return nil, errors.New("sign in to create an organization")
	}
	if !validSlug(name) {
		return nil, errors.New("an organization name may only contain letters, digits, '-', '_', and '.'")
	}
	if reservedNames[strings.ToLower(name)] {
		return nil, errors.New("that name is reserved")
	}
	if _, err := getUserByName(name); err == nil {
		return nil, fmt.Errorf("%q is already taken", name)
	}
	// No password hash: an organization is never a login. Authentication
	// checks a hash that can never match, rather than relying on callers to
	// remember that organizations exist.
	res, err := db.Exec(`INSERT INTO users (username, email, full_name, password_hash, is_admin, is_org, created_at)
		VALUES (?, '', ?, '', 0, 1, ?)`, name, description, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := db.Exec("INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?,?,?,?)",
		id, creator.ID, roleOrgOwner, now()); err != nil {
		return nil, err
	}
	return getUserByID(id)
}

// orgRole reports a user's standing in an organization, or "" for an outsider.
func orgRole(orgID, userID int64) string {
	var role string
	if db.QueryRow("SELECT role FROM org_members WHERE org_id = ? AND user_id = ?", orgID, userID).Scan(&role) != nil {
		return ""
	}
	return role
}

func isOrgOwner(orgID, userID int64) bool { return orgRole(orgID, userID) == roleOrgOwner }

func listOrgMembers(orgID int64) []*OrgMember {
	rows, err := db.Query(`SELECT `+prefixed("u", userCols)+`, m.role FROM org_members m
		JOIN users u ON u.id = m.user_id WHERE m.org_id = ? ORDER BY m.role, u.username`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*OrgMember{}
	for rows.Next() {
		u := &User{}
		m := &OrgMember{}
		if rows.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.PasswordHash, &u.IsAdmin, &u.IsOrg,
			&u.CreatedAt, &m.Role) != nil {
			continue
		}
		u.PasswordHash = ""
		m.User = u
		out = append(out, m)
	}
	return out
}

// listUserOrgs returns the organizations a user belongs to.
func listUserOrgs(userID int64) []*User {
	rows, err := db.Query(`SELECT `+prefixed("u", userCols)+` FROM org_members m
		JOIN users u ON u.id = m.org_id WHERE m.user_id = ? AND u.is_org = 1 ORDER BY u.username`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err == nil {
			u.PasswordHash = ""
			out = append(out, u)
		}
	}
	return out
}

func setOrgMember(orgID, userID int64, role string) error {
	if role != roleOrgOwner && role != roleOrgMember {
		return errors.New("role must be owner or member")
	}
	_, err := db.Exec(`INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?,?,?,?)
		ON CONFLICT(org_id, user_id) DO UPDATE SET role = excluded.role`, orgID, userID, role, now())
	return err
}

// removeOrgMember refuses to remove the last owner: an organization with no
// owner cannot be administered by anyone, and its repositories would be
// stranded.
func removeOrgMember(orgID, userID int64) error {
	if isOrgOwner(orgID, userID) {
		var owners int
		db.QueryRow("SELECT COUNT(*) FROM org_members WHERE org_id = ? AND role = ?", orgID, roleOrgOwner).Scan(&owners)
		if owners <= 1 {
			return errors.New("an organization needs at least one owner")
		}
	}
	_, err := db.Exec("DELETE FROM org_members WHERE org_id = ? AND user_id = ?", orgID, userID)
	return err
}

// prefixed qualifies a column list for a join, so userCols can be reused in
// queries that touch more than one table.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, c := range parts {
		parts[i] = alias + "." + c
	}
	return strings.Join(parts, ", ")
}

// ---------- API ----------

func orgJSON(o *User, viewer *User) map[string]any {
	m := userJSON(o)
	m["description"] = o.FullName // organizations reuse full_name as their blurb
	if viewer != nil {
		m["role"] = orgRole(o.ID, viewer.ID)
		m["can_admin"] = viewer.IsAdmin || isOrgOwner(o.ID, viewer.ID)
	}
	return m
}

func handleAPIOrgs(c *apiCtx, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		if !c.requireUser() {
			return
		}
		out := []map[string]any{}
		for _, o := range listUserOrgs(c.u.ID) {
			out = append(out, orgJSON(o, c.u))
		}
		c.out(200, out)

	case len(rest) == 0 && c.r.Method == http.MethodPost:
		if !c.requireUser() {
			return
		}
		var req struct{ Name, Description string }
		if !c.decode(&req) {
			return
		}
		org, err := createOrg(strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), c.u)
		if err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(201, orgJSON(org, c.u))

	case len(rest) >= 1:
		org, err := getUserByName(rest[0])
		if err != nil || !org.IsOrg {
			c.err(404, "organization not found")
			return
		}
		handleAPIOrg(c, org, rest[1:])

	default:
		c.err(404, "unknown endpoint")
	}
}

func handleAPIOrg(c *apiCtx, org *User, rest []string) {
	admin := c.u != nil && (c.u.IsAdmin || isOrgOwner(org.ID, c.u.ID))
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		m := orgJSON(org, c.u)
		repos := []map[string]any{}
		for _, rp := range listReposForOwner(org.ID, true) {
			if canRead(c.u, rp) {
				repos = append(repos, repoJSON(rp, c.u))
			}
		}
		m["repos"] = repos
		c.out(200, m)

	case len(rest) == 1 && rest[0] == "members" && c.r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, m := range listOrgMembers(org.ID) {
			row := userJSON(m.User)
			row["role"] = m.Role
			out = append(out, row)
		}
		c.out(200, out)

	case len(rest) == 1 && rest[0] == "members" && c.r.Method == http.MethodPost:
		if !admin {
			c.err(403, "you need to be an owner of "+org.Username)
			return
		}
		var req struct{ Username, Role string }
		if !c.decode(&req) {
			return
		}
		u, err := getUserByName(strings.TrimSpace(req.Username))
		if err != nil || u.IsOrg {
			c.err(404, "no such user")
			return
		}
		role := strings.TrimSpace(req.Role)
		if role == "" {
			role = roleOrgMember
		}
		if err := setOrgMember(org.ID, u.ID, role); err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(201, map[string]any{"username": u.Username, "role": role})

	case len(rest) == 2 && rest[0] == "members" && c.r.Method == http.MethodDelete:
		if !admin {
			c.err(403, "you need to be an owner of "+org.Username)
			return
		}
		u, err := getUserByName(rest[1])
		if err != nil {
			c.err(404, "no such user")
			return
		}
		if err := removeOrgMember(org.ID, u.ID); err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(200, map[string]bool{"ok": true})

	default:
		c.err(404, "unknown endpoint")
	}
}
