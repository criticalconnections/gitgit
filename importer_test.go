package main

import "testing"

func TestParseImportSource(t *testing.T) {
	cases := []struct{ in, owner, name, host, kind string }{
		{"octocat/Hello-World", "octocat", "Hello-World", "github.com", "github"},
		{"https://github.com/octocat/Hello-World", "octocat", "Hello-World", "github.com", "github"},
		{"https://gitlab.com/gitlab-org/gitlab", "gitlab-org", "gitlab", "gitlab.com", "gitlab"},
		// GitLab groups nest, and browser URLs carry a /-/ marker
		{"https://gitlab.com/group/subgroup/project", "group/subgroup", "project", "gitlab.com", "gitlab"},
		{"https://gitlab.com/group/sub/proj/-/tree/main", "group/sub", "proj", "gitlab.com", "gitlab"},
		{"git@gitlab.example.org:team/app.git", "team", "app", "gitlab.example.org", "gitlab"},
		{"https://git.corp.internal/team/app.git", "team", "app", "git.corp.internal", "github"},
	}
	for _, tc := range cases {
		got, err := parseImportSource(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got.Owner != tc.owner || got.Name != tc.name || got.Host != tc.host || got.Kind != tc.kind {
			t.Errorf("%s -> %+v, want owner=%s name=%s host=%s kind=%s", tc.in, got, tc.owner, tc.name, tc.host, tc.kind)
		}
	}
}
