package main

// Zero-config preview detection.
//
// Most repositories need a build before there is anything worth looking at: a
// Vite app's tree holds sources and an index.html that references
// /src/main.tsx, which no browser can run. Rather than require every
// repository to commit a .gitgit/preview.yml before its first preview works,
// GitGit reads the tree and proposes how the branch builds and runs.
//
// A proposal is only ever a proposal. Building one executes repository code on
// this host, and unlike a committed .gitgit/preview.yml — which only somebody
// with push access can add — a detected config carries no such consent. So
// detection never starts anything by itself; see previewPlan.

import (
	"encoding/json"
	"strings"
)

// DetectedPreview is a proposed configuration plus the evidence behind it, so
// the guess can be shown to a human, adopted, or corrected.
type DetectedPreview struct {
	Name string // "Vite", "Next.js" — named in the UI
	Why  string // what in the tree gave it away
	Cfg  *PreviewConfig
}

// previewTree is the slice of a commit detection needs. An interface rather
// than a (dir, sha) pair so the rules can be tested without a git repository.
type previewTree interface {
	file(path string) []byte // nil when absent
	has(path string) bool
}

type repoTree struct{ dir, sha string }

func (t repoTree) file(p string) []byte { return fileAtCommit(t.dir, t.sha, p) }
func (t repoTree) has(p string) bool    { return pathKind(t.dir, t.sha, p) != "" }

// detectPreview guesses how a commit previews, or returns nil when it cannot
// tell — a repository whose root index.html is already servable needs no
// build, and so needs no proposal.
func detectPreview(t previewTree) *DetectedPreview {
	if d := detectNode(t); d != nil {
		return d
	}
	if d := detectSiteGenerator(t); d != nil {
		return d
	}
	return detectGo(t)
}

// ---------- node ----------

type nodePkg struct {
	Scripts map[string]string `json:"scripts"`
	Deps    map[string]string `json:"dependencies"`
	DevDeps map[string]string `json:"devDependencies"`
}

func (p *nodePkg) has(deps ...string) bool {
	for _, d := range deps {
		if _, ok := p.Deps[d]; ok {
			return true
		}
		if _, ok := p.DevDeps[d]; ok {
			return true
		}
	}
	return false
}

func (p *nodePkg) script(name string) bool { return strings.TrimSpace(p.Scripts[name]) != "" }

// nodeTooling picks the package manager from the lockfile: `npm ci` fails
// outright in a pnpm repository, and installing with the wrong one silently
// resolves different versions than CI did.
func nodeTooling(t previewTree) (install, run string) {
	switch {
	case t.has("pnpm-lock.yaml"):
		return "pnpm install --frozen-lockfile", "pnpm run"
	case t.has("yarn.lock"):
		return "yarn install --frozen-lockfile", "yarn run"
	case t.has("bun.lockb"), t.has("bun.lock"):
		return "bun install", "bun run"
	case t.has("package-lock.json"):
		return "npm ci", "npm run"
	}
	return "npm install", "npm run"
}

func detectNode(t previewTree) *DetectedPreview {
	raw := t.file("package.json")
	if raw == nil {
		return nil
	}
	pkg := &nodePkg{}
	if json.Unmarshal(raw, pkg) != nil {
		return nil
	}
	install, run := nodeTooling(t)

	// a framework that builds to a directory of files we can serve ourselves
	site := func(name, why, script, out string) *DetectedPreview {
		return &DetectedPreview{Name: name, Why: why, Cfg: &PreviewConfig{
			Build: []string{install, run + " " + script}, Static: out,
		}}
	}
	// a framework that needs its own server process listening on $PORT
	server := func(name, why, script, start string) *DetectedPreview {
		build := []string{install}
		if script != "" {
			build = append(build, run+" "+script)
		}
		return &DetectedPreview{Name: name, Why: why, Cfg: &PreviewConfig{Build: build, Run: start}}
	}
	dep := func(names ...string) string {
		return "package.json depends on " + strings.Join(names, " + ")
	}

	// Ordered most specific first: a Next.js app also depends on react, and an
	// Astro site may well depend on vite.
	switch {
	case pkg.has("next"):
		return server("Next.js", dep("next"), "build", run+" start")
	case pkg.has("nuxt"):
		return server("Nuxt", dep("nuxt"), "build", "node .output/server/index.mjs")
	case pkg.has("@sveltejs/kit"):
		if pkg.has("@sveltejs/adapter-node") {
			return server("SvelteKit", dep("@sveltejs/kit", "@sveltejs/adapter-node"), "build", "node build")
		}
		return site("SvelteKit", dep("@sveltejs/kit"), "build", "build")
	case pkg.has("@remix-run/serve"):
		return server("Remix", dep("@remix-run/serve"), "build", run+" start")
	case pkg.has("@react-router/serve"):
		return server("React Router", dep("@react-router/serve"), "build", run+" start")
	case pkg.has("astro"):
		if pkg.has("@astrojs/node") {
			return server("Astro", dep("astro", "@astrojs/node"), "build", "node ./dist/server/entry.mjs")
		}
		return site("Astro", dep("astro"), "build", "dist")
	case pkg.has("@docusaurus/core"):
		return site("Docusaurus", dep("@docusaurus/core"), "build", "build")
	case pkg.has("vitepress"):
		if pkg.script("docs:build") {
			return site("VitePress", dep("vitepress"), "docs:build", "docs/.vitepress/dist")
		}
		return site("VitePress", dep("vitepress"), "build", "docs/.vitepress/dist")
	case pkg.has("gatsby"):
		return site("Gatsby", dep("gatsby"), "build", "public")
	case pkg.has("@11ty/eleventy"):
		return site("Eleventy", dep("@11ty/eleventy"), "build", "_site")
	case pkg.has("react-scripts"):
		return site("Create React App", dep("react-scripts"), "build", "build")
	case pkg.has("@vue/cli-service"):
		return site("Vue CLI", dep("@vue/cli-service"), "build", "dist")
	case pkg.has("@angular/cli", "@angular-devkit/build-angular"):
		// Angular nests output under dist/<project>[/browser]; the static
		// server descends into a lone wrapper directory to find index.html.
		return site("Angular", dep("the Angular CLI"), "build", "dist")
	case pkg.has("@tanstack/react-start", "@tanstack/solid-start", "@tanstack/start", "nitropack", "vinxi"):
		// Nitro builds to .output, not dist — and it depends on vite, so this
		// has to be decided before the generic Vite case below. Which command
		// serves it depends on the preset the app was built with.
		name := "Nitro"
		if pkg.has("@tanstack/react-start", "@tanstack/solid-start", "@tanstack/start") {
			name = "TanStack Start"
		}
		if pkg.has("wrangler") || t.has("wrangler.jsonc") || t.has("wrangler.toml") || t.has("wrangler.json") {
			// The Cloudflare preset emits a worker module, which plain node
			// cannot run; wrangler runs it locally against workerd.
			return server(name+" (Cloudflare)", "a wrangler config alongside "+name, "build",
				"npx -y wrangler dev --config .output/server/wrangler.json --port $PORT")
		}
		return server(name, dep(name), "build", "node .output/server/index.mjs")
	case pkg.has("parcel"):
		return site("Parcel", dep("parcel"), "build", "dist")
	case pkg.has("vite"):
		return site("Vite", dep("vite"), "build", "dist")
	case pkg.script("start"):
		script := ""
		if pkg.script("build") {
			script = "build"
		}
		return server("Node", "package.json has a start script", script, run+" start")
	}
	return nil
}

// ---------- static site generators ----------

func detectSiteGenerator(t previewTree) *DetectedPreview {
	switch {
	case t.has("hugo.toml"), t.has("hugo.yaml"), t.has("hugo.json"),
		t.has("config.toml") && t.has("content"):
		return &DetectedPreview{Name: "Hugo", Why: "a Hugo site configuration", Cfg: &PreviewConfig{
			Build: []string{"hugo --minify -d public"}, Static: "public",
		}}
	case t.has("_config.yml") && t.has("Gemfile"):
		return &DetectedPreview{Name: "Jekyll", Why: "_config.yml and a Gemfile", Cfg: &PreviewConfig{
			Build: []string{"bundle install", "bundle exec jekyll build"}, Static: "_site",
		}}
	}
	return nil
}

// ---------- go ----------

func detectGo(t previewTree) *DetectedPreview {
	// Only a main package at the root can be built without guessing which of
	// several cmd/ binaries is the web server.
	if !t.has("go.mod") || !t.has("main.go") {
		return nil
	}
	return &DetectedPreview{Name: "Go", Why: "go.mod and a main.go at the root", Cfg: &PreviewConfig{
		Build: []string{"go build -o .gitgit-preview-app ."}, Run: "./.gitgit-preview-app",
	}}
}

// ---------- rendering ----------

// yamlText renders the proposal as the file a maintainer would commit, so a
// good guess can be adopted verbatim and a near-miss edited by hand.
func (d *DetectedPreview) yamlText() string {
	var b strings.Builder
	b.WriteString("# .gitgit/preview.yml — detected: " + d.Name + "\n")
	if len(d.Cfg.Build) > 0 {
		b.WriteString("build:\n")
		for _, step := range d.Cfg.Build {
			b.WriteString("  - " + step + "\n")
		}
	}
	if d.Cfg.Run != "" {
		b.WriteString("run: " + d.Cfg.Run + "        # must listen on $PORT\n")
	}
	if d.Cfg.Static != "" {
		b.WriteString("static: " + d.Cfg.Static + "\n")
	}
	return b.String()
}
