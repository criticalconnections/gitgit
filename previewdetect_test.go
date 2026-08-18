package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// fakeTree stands in for a commit: path -> content ("" for a directory).
type fakeTree map[string]string

func (f fakeTree) file(p string) []byte {
	if c, ok := f[p]; ok && c != "" {
		return []byte(c)
	}
	return nil
}
func (f fakeTree) has(p string) bool { _, ok := f[p]; return ok }

func TestDetectPreview(t *testing.T) {
	cases := []struct {
		name   string
		tree   fakeTree
		want   string // detected framework, "" for no proposal
		build  string // first build step, to check package-manager choice
		run    string
		static string
	}{
		{
			name:   "vite with a package-lock uses npm ci",
			tree:   fakeTree{"package.json": `{"devDependencies":{"vite":"^5"}}`, "package-lock.json": "{}", "index.html": "<html>"},
			want:   "Vite",
			build:  "npm ci",
			static: "dist",
		},
		{
			name:   "vite with a pnpm lockfile uses pnpm",
			tree:   fakeTree{"package.json": `{"devDependencies":{"vite":"^5"}}`, "pnpm-lock.yaml": "-"},
			want:   "Vite",
			build:  "pnpm install --frozen-lockfile",
			static: "dist",
		},
		{
			name:  "next wins over its react dependency",
			tree:  fakeTree{"package.json": `{"dependencies":{"next":"14","react":"18","vite":"5"}}`, "yarn.lock": "-"},
			want:  "Next.js",
			build: "yarn install --frozen-lockfile",
			run:   "yarn run start",
		},
		{
			name:   "sveltekit without an adapter builds to a directory",
			tree:   fakeTree{"package.json": `{"devDependencies":{"@sveltejs/kit":"2"}}`},
			want:   "SvelteKit",
			static: "build",
		},
		{
			name: "sveltekit with adapter-node runs a server",
			tree: fakeTree{"package.json": `{"devDependencies":{"@sveltejs/kit":"2","@sveltejs/adapter-node":"5"}}`},
			want: "SvelteKit",
			run:  "node build",
		},
		{
			name:   "create react app",
			tree:   fakeTree{"package.json": `{"dependencies":{"react-scripts":"5"}}`},
			want:   "Create React App",
			static: "build",
		},
		{
			name: "a plain start script is a node server",
			tree: fakeTree{"package.json": `{"scripts":{"start":"node server.js"}}`},
			want: "Node",
			run:  "npm run start",
		},
		{
			name:   "hugo",
			tree:   fakeTree{"hugo.toml": "x", "content": ""},
			want:   "Hugo",
			static: "public",
		},
		{
			name: "go module with a root main.go",
			tree: fakeTree{"go.mod": "module x", "main.go": "package main"},
			want: "Go",
			run:  "./.gitgit-preview-app",
		},
		{
			name: "a go module without a root main package is not guessed at",
			tree: fakeTree{"go.mod": "module x", "cmd": ""},
			want: "",
		},
		{
			name: "a plain static site needs no build",
			tree: fakeTree{"index.html": "<html>", "style.css": "body{}"},
			want: "",
		},
		{
			name: "unparseable package.json is not guessed at",
			tree: fakeTree{"package.json": "{not json"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectPreview(tc.tree)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected no proposal, got %q", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %s, got no proposal", tc.want)
			}
			if got.Name != tc.want {
				t.Fatalf("detected %q, want %q", got.Name, tc.want)
			}
			if tc.build != "" && (len(got.Cfg.Build) == 0 || got.Cfg.Build[0] != tc.build) {
				t.Errorf("build[0] = %v, want %q", got.Cfg.Build, tc.build)
			}
			if got.Cfg.Run != tc.run {
				t.Errorf("run = %q, want %q", got.Cfg.Run, tc.run)
			}
			if got.Cfg.Static != tc.static {
				t.Errorf("static = %q, want %q", got.Cfg.Static, tc.static)
			}
			if !got.Cfg.needsEnv() {
				t.Errorf("a proposal must need an environment, or it will never be built")
			}
		})
	}
}

// A detected config must never be servable straight from the tree, and a
// committed one that only sets a TTL must not spin up a pointless workspace.
func TestNeedsEnv(t *testing.T) {
	if (&PreviewConfig{TTLMinutes: 30}).needsEnv() {
		t.Error("a config with no build, run, or static should not need an environment")
	}
	if !(&PreviewConfig{Static: "dist"}).needsEnv() {
		t.Error("static output needs a workspace to be built in")
	}
	if !(&PreviewConfig{Build: []string{"make"}}).needsEnv() {
		t.Error("a build step needs a workspace")
	}
}

func TestDetectedYAMLRoundTrips(t *testing.T) {
	d := detectPreview(fakeTree{"package.json": `{"devDependencies":{"vite":"^5"}}`, "package-lock.json": "{}"})
	if d == nil {
		t.Fatal("expected a Vite proposal")
	}
	// the snippet we tell people to commit must actually parse back
	cfg := &PreviewConfig{}
	if err := yaml.Unmarshal([]byte(d.yamlText()), cfg); err != nil {
		t.Fatalf("proposed yaml does not parse: %v", err)
	}
	if cfg.Static != d.Cfg.Static || len(cfg.Build) != len(d.Cfg.Build) {
		t.Fatalf("round trip lost fields: %+v vs %+v", cfg, d.Cfg)
	}
}
